// Package daemon provides an IPC server that exposes a RESTful API
// over a Unix domain socket for CLI ↔ daemon communication.
//
// Security model (see ADR-003):
//   - Instance ownership via flock on devtether.lock (primary authority).
//   - PID file (devtether.pid) for diagnostics and SIGTERM fallback.
//   - Socket is placed in $XDG_RUNTIME_DIR/devtether/ (per-user, tmpfs-backed).
//   - Secure fallback to /tmp/devtether-<uid>/ when XDG_RUNTIME_DIR is unset.
//   - Directory validated via Lstat + UID ownership check (symlink protection).
//   - Socket permissions are 0600 (owner-only read/write).
//   - Socket directory permissions are 0700.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/ivin-titus/devtether/internal/router"
)

// CheckRunning returns an error if another daemon instance is already active.
func CheckRunning(ctx context.Context) error {
	socketPath := SocketPath()
	if _, statErr := os.Stat(socketPath); statErr == nil {
		dialer := net.Dialer{Timeout: 1 * time.Second}
		conn, dialErr := dialer.DialContext(ctx, "unix", socketPath)
		if dialErr == nil {
			_ = conn.Close()
			return fmt.Errorf("daemon: already running on %s", socketPath)
		}
		// Permission denied = socket owned by another user. Don't mask it.
		if netutil.IsPermissionError(dialErr) {
			return fmt.Errorf("daemon: socket permission denied: %w", dialErr)
		}
		// Connection refused = stale socket from a dead process. Safe to proceed.
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("daemon: failed to pre-flight socket: %w", statErr)
	}
	return nil
}

// SocketPath returns the platform-appropriate path for the IPC socket.
// Uses the same directory resolution as RuntimeDir (see lifecycle.go).
func SocketPath() string {
	return filepath.Join(runtimeDirPath(), "devtether.sock")
}

// Server runs an HTTP API over a Unix domain socket for IPC communication.
type Server struct {
	engine     *router.Engine
	httpServer *http.Server
	socketPath string
	startTime  time.Time
	cancelFunc context.CancelFunc
}

// NewServer initializes the IPC daemon.
func NewServer(engine *router.Engine, cancel context.CancelFunc) *Server {
	return &Server{
		engine:     engine,
		cancelFunc: cancel,
		socketPath: SocketPath(),
	}
}

// Start begins listening on the Unix domain socket. It blocks until the
// context is cancelled or a fatal error occurs.
//
// Startup sequence (ADR-003):
//
//	RuntimeDir validation → flock(LOCK_EX|LOCK_NB) → PID write → stale socket cleanup → bind → serve
func (s *Server) Start(ctx context.Context) error {
	// 1. Validate runtime directory (creates if needed, checks ownership).
	if _, dirErr := RuntimeDir(); dirErr != nil {
		return dirErr
	}

	// 2. Acquire exclusive instance lock (non-blocking).
	lockFile, lockErr := AcquireLock()
	if lockErr != nil {
		return lockErr
	}
	// Keep the lock file open for the daemon's entire lifetime.
	// The kernel releases the flock when the file is closed or the process exits.
	defer func() { _ = lockFile.Close() }()

	// 3. Write PID file (atomic: temp → fsync → rename).
	if pidErr := WritePID(); pidErr != nil {
		return pidErr
	}
	defer RemovePID()

	// 4. Clean up stale socket from a previous crash.
	// Under flock protection, this is safe — no other instance can race us.
	if _, statErr := os.Stat(s.socketPath); statErr == nil {
		if rmErr := os.Remove(s.socketPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return fmt.Errorf("daemon: failed to clear stale socket: %w", rmErr)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("daemon: failed to stat socket: %w", statErr)
	}

	// 5. Bind the Unix socket.
	var lc net.ListenConfig

	// ADR-003, ADR-009 §3: Atomic 0600 socket creation via umask.
	// The umask is process-global. This is safe because DNS and Proxy listeners
	// are bound synchronously in up.go *before* the errgroup launches Start().
	// If the startup order changes, replace with post-creation os.Chmod.
	oldUmask := syscall.Umask(0177)
	listener, err := lc.Listen(ctx, "unix", s.socketPath)
	syscall.Umask(oldUmask)

	if err != nil {
		return fmt.Errorf("daemon: failed to bind unix socket: %w", err)
	}

	s.startTime = time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/routes", s.handleRoutes)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/shutdown", s.handleShutdown)

	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second, // Accommodates the shutdown connection-hold pattern.
		MaxHeaderBytes:    1 << 16,          // 64KB — generous for IPC (ADR-003).
	}

	// Graceful shutdown when context is cancelled.
	//nolint:gosec // Background server goroutine does not need request context
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}()

	logger.New("daemon").Debug(fmt.Sprintf("ipc listening on %s", s.socketPath))
	if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("daemon: server error: %w", err)
	}
	return nil
}

// RouteResponse is the JSON representation of a single route.
type RouteResponse struct {
	Domain      string `json:"domain"`
	ServiceName string `json:"serviceName"`
	Port        int    `json:"port"`
	Type        string `json:"type"`
}

// handleRoutes serves the GET /routes endpoint.
func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	routes := s.engine.GetAllRoutes()
	response := make([]RouteResponse, 0, len(routes))

	for _, target := range routes {
		response = append(response, RouteResponse{
			Domain:      target.Domain,
			ServiceName: target.ServiceName,
			Port:        target.Port,
			Type:        string(target.Type),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// StatusResponse is the JSON representation of daemon status.
type StatusResponse struct {
	PID      int     `json:"pid"`
	Uptime   string  `json:"uptime"`
	Routes   int     `json:"routes"`
	MemAlloc uint64  `json:"mem_alloc"`
}

// handleStatus serves the GET /status endpoint.
// It returns a JSON object containing the daemon's process ID (pid),
// the duration it has been running (uptime), the number of active
// proxy rules (routes), and the current heap allocation in bytes (mem_alloc).
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	resp := StatusResponse{
		PID:      os.Getpid(),
		Uptime:   time.Since(s.startTime).Truncate(time.Second).String(),
		Routes:   len(s.engine.GetAllRoutes()),
		MemAlloc: m.Alloc,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleShutdown serves the POST /shutdown endpoint.
//
// Design: The handler flushes the response immediately, triggers shutdown,
// then blocks until the server's own shutdown sequence closes this handler's
// request context. This keeps the HTTP connection alive during the entire
// graceful shutdown (including the proxy's 5s drain), allowing the client
// to detect completion via EOF on the response body. No polling needed.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "shutting down"})

	// Flush the response to the client immediately.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Trigger graceful shutdown of all daemon subsystems.
	s.cancelFunc()

	// Block until the server's shutdown sequence closes our request context.
	// This keeps the HTTP connection alive so the client can detect completion
	// via EOF when the connection closes.
	<-r.Context().Done()
}
