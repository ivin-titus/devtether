// Package netutil provides shared network error classification utilities
// used by both the DNS and proxy servers for port binding fallback logic.
package netutil

import (
	"errors"
	"net"
	"os"
	"syscall"
)

// IsPermissionError returns true if err is caused by EACCES or EPERM.
// Uses typed error unwrapping via errors.As — never string matching.
func IsPermissionError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return errors.Is(sysErr.Err, syscall.EACCES) || errors.Is(sysErr.Err, syscall.EPERM)
		}
	}
	return errors.Is(err, os.ErrPermission)
}

// IsAddrInUse returns true if err is caused by EADDRINUSE.
func IsAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return errors.Is(sysErr.Err, syscall.EADDRINUSE)
		}
	}
	return false
}

// IsRecoverable returns true for errors that should trigger a port fallback
// (permission denied or address already in use).
func IsRecoverable(err error) bool {
	return IsPermissionError(err) || IsAddrInUse(err)
}

// IsDialFailure returns true if err occurred while establishing a connection
// (TCP dial/connect stage) rather than after the connection was established.
// It covers connection refusal, dial timeouts, and unreachable backends, but
// never post-connect failures such as mid-stream resets, response-header
// timeouts, or client-side cancellation.
func IsDialFailure(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Op == "dial"
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}
