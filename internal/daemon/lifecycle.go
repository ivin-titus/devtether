// Package daemon lifecycle management.
//
// This file implements the instance ownership, crash recovery, and directory
// security mechanisms required by ADR-003. The design follows the PostgreSQL
// model: flock is the primary authority for instance ownership, PID file is
// secondary (diagnostics and SIGTERM fallback), and all state files live in
// a validated, user-owned directory.
//
// Startup sequence (see ADR-003, ADR-009):
//
//	flock(LOCK_EX|LOCK_NB) → atomic PID write → Lstat directory validation → stale socket cleanup → bind
//
// The flock is held for the daemon's entire lifetime. The kernel releases it
// on any exit (including SIGKILL), providing crash-safe instance ownership
// without polling or PID-reuse races.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// RuntimeDir returns the validated, user-owned directory for daemon state
// files (lock, PID, socket). It creates the directory if needed and verifies
// ownership to prevent symlink attacks (ADR-003).
func RuntimeDir() (string, error) {
	dir := runtimeDirPath()

	// Create the directory if it does not exist.
	//nolint:gosec // ADR-003: socket directory uses 0700 (owner-only)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("daemon: failed to create runtime directory %s: %w", dir, err)
	}

	// ADR-003: Symlink protection — verify the directory is real and owned by us.
	if err := validateDirOwnership(dir); err != nil {
		return "", err
	}

	return dir, nil
}

// runtimeDirPath returns the raw directory path without validation.
func runtimeDirPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "devtether")
	}
	// Secure fallback: per-user isolated directory instead of bare /tmp/devtether.sock.
	// This prevents fake-daemon hijacking and multi-user conflicts (ADR-003).
	return filepath.Join(os.TempDir(), fmt.Sprintf("devtether-%d", os.Getuid()))
}

// validateDirOwnership uses Lstat (no symlink follow) to verify that dir is
// a real directory owned by the current user. Fails closed on any mismatch.
func validateDirOwnership(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("daemon: failed to stat runtime directory: %w", err)
	}

	// Must be a directory, not a symlink.
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("daemon: runtime directory %s is a symlink (possible attack)", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("daemon: %s exists but is not a directory", dir)
	}

	// Verify ownership matches the current user.
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("daemon: unable to read directory ownership (unsupported platform)")
	}
	//nolint:gosec // G115: Unix UIDs fit in uint32 safely
	if stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("daemon: runtime directory %s is owned by uid %d, not %d (possible attack)", dir, stat.Uid, os.Getuid())
	}

	return nil
}

// LockPath returns the path to the daemon lock file.
func LockPath() string {
	return filepath.Join(runtimeDirPath(), "devtether.lock")
}

// PIDPath returns the path to the daemon PID file.
func PIDPath() string {
	return filepath.Join(runtimeDirPath(), "devtether.pid")
}

// AcquireLock attempts to acquire an exclusive, non-blocking flock on the
// lock file. On success, the returned file must be kept open for the daemon's
// entire lifetime — the kernel releases the lock when the file is closed or
// the process exits (including SIGKILL).
//
// Returns os.ErrExist if another instance holds the lock.
func AcquireLock() (*os.File, error) {
	lockPath := LockPath()

	//nolint:gosec // Lock file path is derived from validated runtime directory
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("daemon: failed to open lock file %s: %w", lockPath, err)
	}

	// LOCK_EX = exclusive, LOCK_NB = non-blocking (fail immediately if held).
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("daemon: already running (lock held on %s): %w", lockPath, os.ErrExist)
		}
		return nil, fmt.Errorf("daemon: failed to acquire lock on %s: %w", lockPath, err)
	}

	return f, nil
}

// WritePID writes the current process ID to the PID file atomically
// (write to temp file → fsync → rename). The PID file is secondary to flock —
// it exists for diagnostics and as a SIGTERM fallback, not for instance
// ownership.
func WritePID() error {
	pidPath := PIDPath()
	tmpPath := pidPath + ".tmp"

	pid := os.Getpid()

	//nolint:gosec // PID file uses 0644 — readable by other tools for diagnostics
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("daemon: failed to create temp PID file: %w", err)
	}

	if _, writeErr := fmt.Fprintf(f, "%d\n", pid); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("daemon: failed to write PID: %w", writeErr)
	}

	if syncErr := f.Sync(); syncErr != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("daemon: failed to sync PID file: %w", syncErr)
	}

	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("daemon: failed to close PID file: %w", closeErr)
	}

	if renameErr := os.Rename(tmpPath, pidPath); renameErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("daemon: failed to rename PID file: %w", renameErr)
	}

	return nil
}

// RemovePID removes the PID file. Called during graceful shutdown.
func RemovePID() {
	_ = os.Remove(PIDPath())
}

// ReadPID reads the PID from the PID file. Returns 0 if the file does not
// exist or cannot be parsed.
func ReadPID() int {
	data, err := os.ReadFile(PIDPath())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(string(data[:len(data)-1])) // Strip trailing newline.
	if err != nil {
		return 0
	}
	return pid
}

// WaitForExit blocks until the daemon releases its flock (i.e., the daemon
// process exits). This is event-driven — the goroutine blocks in the kernel
// on flock(LOCK_EX), which returns the instant the daemon's file descriptor
// is closed (on any exit, including SIGKILL).
//
// Returns nil when the daemon exits, or ctx.Err() if the context is cancelled.
func WaitForExit(ctx context.Context) error {
	lockPath := LockPath()

	//nolint:gosec // Lock file path is derived from validated runtime directory
	f, err := os.Open(lockPath)
	if err != nil {
		// Lock file doesn't exist → daemon is not running.
		return nil
	}
	defer func() { _ = f.Close() }()

	// Run the blocking flock in a goroutine so we can respect context cancellation.
	done := make(chan error, 1)
	go func() {
		// LOCK_EX (blocking) — waits until the daemon releases the lock.
		flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if flockErr == nil {
			// Acquired the lock → daemon is dead. Release immediately.
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		}
		done <- flockErr
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Close the file to unblock the flock goroutine.
		_ = f.Close()
		return ctx.Err()
	}
}
