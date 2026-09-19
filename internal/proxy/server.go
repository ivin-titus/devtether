// Package proxy provides an HTTP reverse proxy that routes incoming
// requests to local backend services based on the Host header.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/ivin-titus/devtether/internal/router"
)

// Server wraps an http.Server configured as a reverse proxy with
// graceful shutdown support.
type Server struct {
	httpServer *http.Server
	port       int
	fallback   int
	// bindErr records the failure that forced a fallback from the configured
	// port. It is nil when the configured port bound successfully.
	bindErr error
}

// BindError returns the error that forced the proxy onto its fallback port,
// or nil when the configured port bound successfully. Callers use this to
// explain the fallback truthfully (privilege vs occupied) instead of guessing.
func (s *Server) BindError() error {
	return s.bindErr
}

// NewServer creates a proxy server with the given configuration.
// The resolver is used for Host header → backend target lookups.
func NewServer(cfg config.ProxyConfig, resolver router.Resolver) *Server {
	idleTimeout := config.ParseDuration(cfg.Timeouts.Idle, config.DefaultIdleTimeout)

	handler := NewHandler(resolver)

	return &Server{
		httpServer: &http.Server{
			Handler:           LoggingMiddleware(handler),
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       idleTimeout,
		},
		port:     cfg.Port,
		fallback: 8080,
	}
}

// Listen binds the proxy to the configured port and returns the listener and bound address.
// Port resolution order:
//  1. Configured port (default: 80)
//  2. If EACCES/EPERM or EADDRINUSE → fallback port (8080)
//
// If neither port can be bound the error is fatal; there is no OS-assigned
// port tier.
func (s *Server) Listen(ctx context.Context) (net.Listener, string, error) {
	return s.bind(ctx)
}

// Serve begins serving on the provided listener.
// It blocks until the context is cancelled.
func (s *Server) Serve(ctx context.Context, listener net.Listener, addr string) error {
	shutdownComplete := make(chan struct{})

	// Graceful shutdown when context is cancelled.
	//nolint:gosec // Background server goroutine does not need request context
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			logger.New("proxy").Error("shutdown error", err)
		}
		close(shutdownComplete)
	}()

	logger.New("proxy").Debug(fmt.Sprintf("listening on %s", addr))
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownComplete
		return nil
	}
	return fmt.Errorf("proxy: server error: %w", err)
}

// bind attempts to bind to the configured port, with automatic fallback.
// Both EACCES (permission denied) and EADDRINUSE (port occupied) trigger
// the next fallback step. Only truly unexpected errors are fatal.
func (s *Server) bind(ctx context.Context) (net.Listener, string, error) {
	var lc net.ListenConfig

	// Step 1: Try configured port.
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	listener, err := lc.Listen(ctx, "tcp", addr)
	if err == nil {
		return listener, addr, nil
	}

	// Record the configured-port failure so callers can report the true reason.
	s.bindErr = err

	if netutil.IsPermissionError(err) {
		logger.New("proxy").Debug(fmt.Sprintf("permission denied on port %d — falling back to port %d", s.port, s.fallback))
	} else if netutil.IsAddrInUse(err) {
		logger.New("proxy").Debug(fmt.Sprintf("port %d already in use — falling back to port %d", s.port, s.fallback))
	} else {
		return nil, "", fmt.Errorf("proxy: failed to bind to port %d: %w", s.port, err)
	}

	// Step 2: Try fallback port.
	addr = fmt.Sprintf("127.0.0.1:%d", s.fallback)
	listener, err = lc.Listen(ctx, "tcp", addr)
	if err == nil {
		return listener, addr, nil
	}

	if !netutil.IsRecoverable(err) {
		return nil, "", fmt.Errorf("proxy: failed to bind to fallback port %d: %w", s.fallback, err)
	}
	return nil, "", fmt.Errorf("proxy: configured port %d and fallback port %d are unavailable: %w", s.port, s.fallback, err)
}
