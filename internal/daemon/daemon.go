// Package daemon provides an IPC server that exposes a RESTful API
// over a Unix domain socket for CLI ↔ daemon communication.
//
// Security model (see ADR-003):
//   - Socket is placed in $XDG_RUNTIME_DIR/devtether/ (per-user, tmpfs-backed).
//   - Socket permissions are 0600 (owner-only read/write).
//   - Socket directory permissions are 0700.
//   - Fallback to /tmp/devtether.sock if XDG_RUNTIME_DIR is not set.
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
func SocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "devtether", "devtether.sock")
	}
	return "/tmp/devtether.sock"
}

// Server runs an HTTP API over a Unix domain socket for IPC communication.
type Server struct {
	engine     *router.Engine
	httpServer *http.Server
	socketPath string
}

// NewServer initializes the IPC daemon.
func NewServer(engine *router.Engine) *Server {
	return &Server{
		engine:     engine,
		socketPath: SocketPath(),
	}
}

// Start begins listening on the Unix domain socket. It blocks until the
// context is cancelled or a fatal error occurs.
func (s *Server) Start(ctx context.Context) error {
	// Ensure socket directory exists with restrictive permissions.
	socketDir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return fmt.Errorf("daemon: failed to create socket directory: %w", err)
	}

	// Clean up dead socket from a previous run.
	if _, statErr := os.Stat(s.socketPath); statErr == nil {
		// Socket file exists. Check if it's stale.
		dialer := net.Dialer{Timeout: 1 * time.Second}
		conn, dialErr := dialer.DialContext(ctx, "unix", s.socketPath)
		if dialErr == nil {
			_ = conn.Close()
			return fmt.Errorf("daemon: already running on %s", s.socketPath)
		}
		// Connection failed, assume stale socket and remove it.
		if rmErr := os.Remove(s.socketPath); rmErr != nil {
			return fmt.Errorf("daemon: failed to clear stale socket: %w", rmErr)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("daemon: failed to stat socket: %w", statErr)
	}

	var lc net.ListenConfig
	
	// ADR-003: Atomic 0600 socket creation via umask — eliminates TOCTOU window.
	oldUmask := syscall.Umask(0177)
	listener, err := lc.Listen(ctx, "unix", s.socketPath)
	syscall.Umask(oldUmask)
	
	if err != nil {
		return fmt.Errorf("daemon: failed to bind unix socket: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/routes", s.handleRoutes)

	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown when context is cancelled.
	//nolint:gosec // Background server goroutine does not need request context
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		// Socket cleanup handled by net.UnixListener.Close() and
		// the pre-flight stale socket probe in Start().
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
