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
			return sysErr.Err == syscall.EACCES || sysErr.Err == syscall.EPERM
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
			return sysErr.Err == syscall.EADDRINUSE
		}
	}
	return false
}

// IsRecoverable returns true for errors that should trigger a port fallback
// (permission denied or address already in use).
func IsRecoverable(err error) bool {
	return IsPermissionError(err) || IsAddrInUse(err)
}
