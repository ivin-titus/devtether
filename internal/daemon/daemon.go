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
			return fmt.Errorf("daemon: already running on %s — run 'devtether status' to inspect it or 'devtether down' to stop it", socketPath)
		}
		// Permission denied = socket owned by another user. Don't mask it.
		if netutil.IsPermissionError(dialErr) {
			return fmt.Errorf("daemon: socket permission denied on %s (a root-owned daemon may be running; rerun with sudo): %w", socketPath, dialErr)
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
	configPath string
	startTime  time.Time
	cancelFunc context.CancelFunc
	lockFile   *os.File
	nonce      string
}

// NewServer initializes the IPC daemon. configPath is the config file the
// daemon loaded; it is reported by /status so a client can identify which
// project instance it is talking to.
func NewServer(engine *router.Engine, cancel context.CancelFunc, configPath string) *Server {
	return &Server{
		engine:     engine,
		cancelFunc: cancel,
		socketPath: SocketPath(),
		configPath: configPath,
	}
}

// Start begins listening on the Unix domain socket. It blocks until the
// context is cancelled or a fatal error occurs.
//
// Startup sequence (ADR-003):
//
//	RuntimeDir validation → flock(LOCK_EX|LOCK_NB) → PID write → stale socket cleanup → bind → serve
func (s *Server) Start(ctx context.Context) error {
	listener, err := s.Listen(ctx)
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

// Listen acquires daemon ownership and binds the IPC socket before any
// concurrent server work begins. The returned listener remains owned by s.
func (s *Server) Listen(ctx context.Context) (net.Listener, error) {
	// 1. Validate runtime directory (creates if needed, checks ownership).
	if _, dirErr := RuntimeDir(); dirErr != nil {
		return nil, dirErr
	}

	// 2. Acquire exclusive instance lock (non-blocking).
	lockFile, lockErr := AcquireLock()
	if lockErr != nil {
		return nil, lockErr
	}
	s.lockFile = lockFile

	// 3. Write PID file (atomic: temp → fsync → rename).
	if pidErr := WritePID(); pidErr != nil {
		_ = s.lockFile.Close()
		s.lockFile = nil
		return nil, pidErr
	}
	nonce, nonceErr := writeNonce()
	if nonceErr != nil {
		RemovePID()
		_ = s.lockFile.Close()
		s.lockFile = nil
		return nil, nonceErr
	}
	s.nonce = nonce

	// 4. Clean up stale socket from a previous crash.
	// Under flock protection, this is safe — no other instance can race us.
	// Lstat (no symlink follow) is required: a *dangling* symlink at the socket
	// path looks absent to os.Stat, so bind would fail later with a misleading
	// "address already in use" instead of an explicit refusal (ADR-003).
	if linfo, statErr := os.Lstat(s.socketPath); statErr == nil {
		if linfo.Mode()&os.ModeSymlink != 0 {
			RemovePID()
			_ = s.lockFile.Close()
			s.lockFile = nil
			return nil, fmt.Errorf("daemon: socket path %s is a symlink (possible attack)", s.socketPath)
		}
		if linfo.Mode()&os.ModeSocket == 0 {
			RemovePID()
			_ = s.lockFile.Close()
			s.lockFile = nil
			return nil, fmt.Errorf("daemon: socket path %s exists but is not a unix socket", s.socketPath)
		}
		if rmErr := os.Remove(s.socketPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			RemovePID()
			_ = s.lockFile.Close()
			s.lockFile = nil
			return nil, fmt.Errorf("daemon: failed to clear stale socket: %w", rmErr)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		RemovePID()
		_ = s.lockFile.Close()
		s.lockFile = nil
		return nil, fmt.Errorf("daemon: failed to stat socket: %w", statErr)
	}

	// 5. Bind the Unix socket.
	var lc net.ListenConfig

	// ADR-003, ADR-009 §3: Atomic 0600 socket creation via umask.
	// The umask is process-global. This is safe because DNS and Proxy listeners
	// are bound synchronously in up.go *before* the errgroup launches Start().
	// If the startup order changes, replace with post-creation os.Chmod.
	oldUmask := syscall.Umask(0177)
	defer syscall.Umask(oldUmask)
	listener, err := lc.Listen(ctx, "unix", s.socketPath)

	if err != nil {
		RemovePID()
		_ = s.lockFile.Close()
		s.lockFile = nil
		return nil, fmt.Errorf("daemon: failed to bind unix socket: %w", err)
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
	return listener, nil
}

// Serve accepts IPC connections on listener until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	defer RemovePID()
	defer removeNonce()
	defer func() {
		if s.lockFile != nil {
			_ = s.lockFile.Close()
			s.lockFile = nil
		}
	}()
	// Unlink the socket while the instance lock is still held, so a concurrent
	// start cannot bind a fresh socket that this deferred cleanup then removes.
	if s.socketPath != "" {
		defer func() { _ = os.Remove(s.socketPath) }()
	}

	shutdownComplete := make(chan struct{})

	// Graceful shutdown when context is cancelled.
	//nolint:gosec // Background server goroutine does not need request context
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		close(shutdownComplete)
	}()

	logger.New("daemon").Debug(fmt.Sprintf("ipc listening on %s", s.socketPath))
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownComplete
		return nil
	}
	return fmt.Errorf("daemon: server error: %w", err)
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
	PID        int    `json:"pid"`
	Uptime     string `json:"uptime"`
	Routes     int    `json:"routes"`
	MemAlloc   uint64 `json:"mem_alloc"`
	ConfigPath string `json:"config_path"`
}

// handleStatus serves the GET /status endpoint.
// It returns a JSON object containing the daemon's process ID (pid),
// the duration it has been running (uptime), the number of active
// proxy rules (routes), the current heap allocation in bytes (mem_alloc),
// and the config file it loaded (config_path).
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	resp := StatusResponse{
		PID:        os.Getpid(),
		Uptime:     time.Since(s.startTime).Truncate(time.Second).String(),
		Routes:     len(s.engine.GetAllRoutes()),
		MemAlloc:   m.Alloc,
		ConfigPath: s.configPath,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleShutdown serves the POST /shutdown endpoint.
//
// Design: The handler flushes the response immediately and triggers shutdown.
// The HTTP connection closes as soon as this handler returns. The client
// must then poll the daemon's lockfile to determine when the process has
// completely exited.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Devtether-Nonce") != s.nonce {
		http.Error(w, "invalid IPC nonce", http.StatusForbidden)
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
}
