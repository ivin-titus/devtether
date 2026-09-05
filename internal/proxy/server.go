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
}

// NewServer creates a proxy server with the given configuration.
// The resolver is used for Host header → backend target lookups.
func NewServer(cfg config.ProxyConfig, resolver router.Resolver) *Server {
	readTimeout := config.ParseDuration(cfg.Timeouts.Read, config.DefaultReadTimeout)
	writeTimeout := config.ParseDuration(cfg.Timeouts.Write, config.DefaultWriteTimeout)
	idleTimeout := config.ParseDuration(cfg.Timeouts.Idle, config.DefaultIdleTimeout)

	handler := NewHandler(resolver)

	return &Server{
		httpServer: &http.Server{
			Handler:      LoggingMiddleware(handler),
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  idleTimeout,
		},
		port:     cfg.Port,
		fallback: 8080,
	}
}

// Listen binds the proxy to the configured port and returns the listener and bound address.
// Port resolution order:
//  1. Configured port (default: 80)
//  2. If EACCES/EPERM → fallback port (8080)
//  3. If fallback is also in use → OS-assigned port (port 0)
func (s *Server) Listen(ctx context.Context) (net.Listener, string, error) {
	return s.bind(ctx)
}

// Serve begins serving on the provided listener.
// It blocks until the context is cancelled.
func (s *Server) Serve(ctx context.Context, listener net.Listener, addr string) error {
	// Graceful shutdown when context is cancelled.
	//nolint:gosec // Background server goroutine does not need request context
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			logger.New("proxy").Error("shutdown error", err)
		}
	}()

	logger.New("proxy").Debug(fmt.Sprintf("listening on %s", addr))
	if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("proxy: server error: %w", err)
	}
	return nil
}

// bind attempts to bind to the configured port, with automatic fallback.
// Both EACCES (permission denied) and EADDRINUSE (port occupied) trigger
// the next fallback step. Only truly unexpected errors are fatal.
func (s *Server) bind(ctx context.Context) (net.Listener, string, error) {
	var lc net.ListenConfig

	// Step 1: Try configured port.
	addr := fmt.Sprintf(":%d", s.port)
	listener, err := lc.Listen(ctx, "tcp", addr)
	if err == nil {
		return listener, addr, nil
	}

	if netutil.IsPermissionError(err) {
		logger.New("proxy").Debug(fmt.Sprintf("permission denied on port %d — trying port %d", s.port, s.fallback))
	} else if netutil.IsAddrInUse(err) {
		logger.New("proxy").Debug(fmt.Sprintf("port %d already in use — trying port %d", s.port, s.fallback))
	} else {
		return nil, "", fmt.Errorf("proxy: failed to bind to port %d: %w", s.port, err)
	}

	// Step 2: Try fallback port.
	addr = fmt.Sprintf(":%d", s.fallback)
	listener, err = lc.Listen(ctx, "tcp", addr)
	if err == nil {
		return listener, addr, nil
	}

	if netutil.IsRecoverable(err) {
		logger.New("proxy").Info(fmt.Sprintf("port %d also unavailable — binding to OS-assigned port", s.fallback))
	} else {
		return nil, "", fmt.Errorf("proxy: failed to bind to fallback port %d: %w", s.fallback, err)
	}

	// Step 3: Last resort — OS-assigned port.
	listener, err = lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("proxy: failed to bind to any port: %w", err)
	}
	addr = listener.Addr().String()
	return listener, addr, nil
}
