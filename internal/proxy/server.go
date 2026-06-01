package proxy

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"syscall"
)

// Start binds the proxy server to port 80, or falls back to port 8080 if permissions evaluate to denied.
func (s *Server) Start() error {
	addr := ":80"

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Detect if this is a permission error indicating lack of root/setcap
		if isPermissionError(err) {
			log.Println("[Proxy] Permission denied on port 80. Falling back to port 8080.")
			addr = ":8080"
			listener, err = net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("failed to bind to fallback port 8080: %w", err)
			}
		} else {
			return fmt.Errorf("failed to bind to port 80: %w", err)
		}
	}

	log.Printf("[Proxy] HTTP Reverse Proxy listening on %s\n", addr)

	return http.Serve(listener, s)
}

func isPermissionError(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		var sysErr *os.SyscallError
		if errors.As(netErr.Err, &sysErr) {
			return sysErr.Err == syscall.EACCES || sysErr.Err == syscall.EPERM
		}
	}
	// Fallback check based on string if casting fails on non-unix structures sometimes
	if errors.Is(err, os.ErrPermission) || (err != nil && (err.Error() == "bind: permission denied" || err.Error() == "listen tcp :80: bind: permission denied")) {
		return true
	}
	return false
}
