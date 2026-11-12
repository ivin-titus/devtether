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
//	RuntimeDir validation → flock(LOCK_EX|LOCK_NB) → atomic PID write → stale socket cleanup → bind
//
// The flock is held for the daemon's entire lifetime. The kernel releases it
// on any exit (including SIGKILL), providing crash-safe instance ownership
// without polling or PID-reuse races.
package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// RuntimeDir returns the validated, user-owned directory for daemon state
// files (lock, PID, socket). It securely creates the directory if needed and
// verifies ownership to prevent symlink attacks (ADR-003). Resolution is
// delegated to resolveRuntimeDir, which enforces per-UID isolation and rejects
// unsafe inherited environments (XDG_RUNTIME_DIR/TMPDIR).
func RuntimeDir() (string, error) {
	return resolveRuntimeDir(true)
}

// LockPath returns the path to the daemon lock file.
func LockPath() (string, error) {
	return runtimeFilePath("devtether.lock")
}

// PIDPath returns the path to the daemon PID file.
func PIDPath() (string, error) {
	return runtimeFilePath("devtether.pid")
}

func noncePath() (string, error) { return runtimeFilePath("devtether.nonce") }

func writeNonce() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("daemon: generate IPC nonce: %w", err)
	}
	nonce := hex.EncodeToString(buf)
	path, err := noncePath()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(nonce), 0600); err != nil {
		return "", fmt.Errorf("daemon: write IPC nonce: %w", err)
	}
	return nonce, nil
}

func readNonce() (string, error) {
	path, err := noncePath()
	if err != nil {
		return "", err
	}
	//nolint:gosec // G304: path is derived from the validated runtime directory
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func removeNonce() {
	if path, err := noncePath(); err == nil {
		_ = os.Remove(path)
	}
}

// AcquireLock attempts to acquire an exclusive, non-blocking flock on the
// lock file. On success, the returned file must be kept open for the daemon's
// entire lifetime — the kernel releases the lock when the file is closed or
// the process exits (including SIGKILL).
//
// Returns os.ErrExist if another instance holds the lock.
func AcquireLock() (*os.File, error) {
	lockPath, err := LockPath()
	if err != nil {
		return nil, err
	}

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
	pidPath, err := PIDPath()
	if err != nil {
		return err
	}
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
	if path, err := PIDPath(); err == nil {
		_ = os.Remove(path)
	}
}

// ReadPID reads the PID from the PID file. Returns 0 if the file does not
// exist, cannot be parsed, or the runtime directory cannot be resolved.
func ReadPID() int {
	path, err := PIDPath()
	if err != nil {
		return 0
	}
	//nolint:gosec // G304: path is derived from the validated runtime directory
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// PIDAlive reports whether pid identifies a live process. A permission error
// counts as alive: the process exists but belongs to another user.
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// CleanStaleSocket removes an IPC socket and PID file orphaned by a daemon
// that is no longer running, so `status` and `down` recover on their own.
// Callers invoke it after a failed connection attempt (refused or missing
// socket); the PID guard prevents disturbing a live process whose socket
// briefly refused a connection.
func CleanStaleSocket() {
	if PIDAlive(ReadPID()) {
		return
	}
	socketPath, err := SocketPath()
	if err != nil {
		// No validated runtime directory means there is nothing to clean.
		return
	}
	if _, err := os.Lstat(socketPath); err == nil {
		_ = os.Remove(socketPath)
	}
	RemovePID()
}

// RootDaemonMayBeRunning reports whether a root-owned daemon socket may be
// present in a location this user cannot inspect. It is a best-effort
// diagnostic heuristic, NOT a security check: it never connects to a socket
// and only Lstats candidate paths. It may miss a root daemon that uses a
// custom XDG_RUNTIME_DIR; the authoritative signal remains the IPC permission
// error from an actual connect attempt.
func RootDaemonMayBeRunning() bool {
	if os.Getuid() == 0 {
		return false
	}
	candidates := []string{
		"/run/user/0/devtether/devtether.sock",
		filepath.Join(os.TempDir(), "devtether-0", "devtether.sock"),
		filepath.Join("/tmp", "devtether-0", "devtether.sock"),
	}
	for _, p := range candidates {
		info, err := os.Lstat(p)
		if err == nil {
			if info.Mode()&os.ModeSocket != 0 {
				return true
			}
			continue
		}
		if errors.Is(err, os.ErrPermission) {
			return true
		}
	}
	return false
}

// ErrNoDaemon reports that no daemon lock file exists for this user, so no
// daemon is running.
var ErrNoDaemon = errors.New("daemon: no lock file found")

// ErrLockUnreadable reports that the lock file exists but cannot be opened,
// typically because it belongs to another user's daemon.
var ErrLockUnreadable = errors.New("daemon: lock file is not readable")

// DaemonRunning probes the lock file for a live daemon without blocking.
//
// It returns (false, nil) when no daemon holds the lock, (true, nil) when a
// daemon does, and a distinguishable error when the lock file cannot be
// inspected at all (e.g. another user's daemon).
func DaemonRunning() (bool, error) {
	lockPath, err := LockPath()
	if err != nil {
		if errors.Is(err, ErrNoRuntimeDir) {
			return false, nil
		}
		return false, err
	}

	//nolint:gosec // Lock file path is derived from the validated runtime directory
	f, err := os.Open(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %s", ErrLockUnreadable, lockPath)
	}
	defer func() { _ = f.Close() }()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true, nil
		}
		return false, fmt.Errorf("daemon: failed to probe lock %s: %w", lockPath, err)
	}
	// Lock acquired → no daemon holds it. Release immediately.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false, nil
}

// flockPollInterval bounds how long WaitForExit sleeps between non-blocking
// lock probes. While an event-driven alternative exists (watching for lock file
// deletion via fsnotify), we opt for simple polling to avoid the complexity
// of file system watchers for a non-latency-critical exit check.
const flockPollInterval = 500 * time.Millisecond

// WaitForExit blocks until the daemon releases its flock (i.e., the daemon
// process exits). It probes the lock with non-blocking flock calls so context
// cancellation is honored promptly and no goroutine is left blocked on a
// descriptor the caller may close. The kernel releases the lock on any daemon
// exit, including SIGKILL.
//
// Returns nil when the daemon exits, ErrNoDaemon when no lock file exists,
// ErrLockUnreadable when the lock file cannot be opened, or ctx.Err() if the
// context is cancelled.
func WaitForExit(ctx context.Context) error {
	lockPath, err := LockPath()
	if err != nil {
		if errors.Is(err, ErrNoRuntimeDir) {
			return ErrNoDaemon
		}
		return err
	}

	//nolint:gosec // Lock file path is derived from validated runtime directory
	f, err := os.Open(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNoDaemon
		}
		return fmt.Errorf("%w: %s", ErrLockUnreadable, lockPath)
	}
	defer func() { _ = f.Close() }()

	fd := int(f.Fd())
	ticker := time.NewTicker(flockPollInterval)
	defer ticker.Stop()

	for {
		// LOCK_NB fails immediately while another process holds the lock.
		flockErr := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if flockErr == nil {
			// Acquired the lock → daemon is dead. Release immediately.
			_ = syscall.Flock(fd, syscall.LOCK_UN)
			return nil
		}
		if !errors.Is(flockErr, syscall.EWOULDBLOCK) {
			return flockErr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
