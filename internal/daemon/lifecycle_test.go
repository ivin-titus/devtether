package daemon

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// isolateRuntime points the daemon state directory at a fresh temp dir and
// creates it, mirroring the directory RuntimeDir() would validate at startup.
// Daemon state must never touch the real user runtime directory in tests.
func isolateRuntime(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", root)
	if err := os.MkdirAll(filepath.Join(root, "devtether"), 0700); err != nil {
		t.Fatalf("failed to create runtime directory: %v", err)
	}
	return root
}

// writeSocket creates a regular file standing in for a bound Unix socket.
// CleanStaleSocket only cares about filesystem presence, not socket-ness.
func writeSocket(t *testing.T) {
	t.Helper()
	//nolint:gosec // Test stand-in for a socket file
	if err := os.WriteFile(SocketPath(), nil, 0600); err != nil {
		t.Fatalf("failed to create socket stand-in: %v", err)
	}
}

func TestReadPIDHandlesEmptyFile(t *testing.T) {
	isolateRuntime(t)
	if err := os.MkdirAll(filepath.Dir(PIDPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PIDPath(), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := ReadPID(); got != 0 {
		t.Errorf("ReadPID() = %d, want 0 for empty file", got)
	}
}

func TestReadPIDParsing(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{name: "valid pid", content: "4242\n", want: 4242},
		{name: "padded pid", content: "  4242  ", want: 4242},
		{name: "garbage", content: "not-a-pid", want: 0},
		{name: "empty", content: "", want: 0},
		{name: "negative is parsed verbatim but never alive", content: "-1", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateRuntime(t)
			if err := os.MkdirAll(filepath.Dir(PIDPath()), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(PIDPath(), []byte(tt.content), 0600); err != nil {
				t.Fatal(err)
			}
			if got := ReadPID(); got != tt.want {
				t.Errorf("ReadPID() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWritePIDIsAtomicAndCleansTempFile(t *testing.T) {
	isolateRuntime(t)

	if err := WritePID(); err != nil {
		t.Fatalf("WritePID() = %v, want nil", err)
	}
	if got := ReadPID(); got != os.Getpid() {
		t.Errorf("ReadPID() = %d, want %d", got, os.Getpid())
	}
	if _, err := os.Stat(PIDPath() + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temporary PID file was left behind: %v", err)
	}

	RemovePID()
	if _, err := os.Stat(PIDPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("RemovePID() left the PID file behind: %v", err)
	}
}

func TestRuntimeDirCreatesPrivateDirectory(t *testing.T) {
	root := isolateRuntime(t)

	dir, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir() = %v, want nil", err)
	}
	if want := filepath.Join(root, "devtether"); dir != want {
		t.Errorf("RuntimeDir() = %q, want %q", dir, want)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	// ADR-003: the socket directory is owner-only.
	if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("runtime directory mode = %o, want 0700", got)
	}
}

func TestRuntimeDirRejectsSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", root)
	target := t.TempDir()

	if err := os.Symlink(target, filepath.Join(root, "devtether")); err != nil {
		t.Skipf("cannot create symlink on this platform: %v", err)
	}

	_, err := RuntimeDir()
	if err == nil {
		t.Fatal("RuntimeDir() succeeded on a symlinked directory, want failure")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("runtime directory error = %v, want a symlink warning", err)
	}
}

// TestRuntimeDirRejectsDanglingSymlink covers the case where the symlinked
// runtime directory points at a target that does not exist: the refusal must
// still be explicit (ADR-003), not a generic "file exists" from MkdirAll.
func TestRuntimeDirRejectsDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", root)

	if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, "devtether")); err != nil {
		t.Skipf("cannot create symlink on this platform: %v", err)
	}

	_, err := RuntimeDir()
	if err == nil {
		t.Fatal("RuntimeDir() succeeded on a dangling symlinked directory, want failure")
	}
	if !strings.Contains(err.Error(), "is a symlink (possible attack)") {
		t.Errorf("runtime directory error = %v, want the explicit ADR-003 symlink refusal", err)
	}
}

func TestStatePathsShareRuntimeDir(t *testing.T) {
	root := isolateRuntime(t)
	wantDir := filepath.Join(root, "devtether")

	paths := []struct {
		name string
		path string
	}{
		{name: "lock", path: LockPath()},
		{name: "pid", path: PIDPath()},
		{name: "socket", path: SocketPath()},
	}
	for _, p := range paths {
		if filepath.Dir(p.path) != wantDir {
			t.Errorf("%s path = %q, want parent %q", p.name, p.path, wantDir)
		}
	}
}

func TestRuntimeDirRejectsFileAtPath(t *testing.T) {
	root := isolateRuntime(t)
	// Remove the directory the helper created, then occupy its path with a file.
	dir := filepath.Join(root, "devtether")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Test fixture
	if err := os.WriteFile(dir, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := RuntimeDir(); err == nil {
		t.Fatal("RuntimeDir() succeeded when a regular file occupies the path, want failure")
	}
}

// TestSecretArtifacts asserts the IPC nonce and lock file meet the ADR-003
// permission contract and that nonces are not reused across restarts.
func TestSecretArtifacts(t *testing.T) {
	isolateRuntime(t)

	first, err := writeNonce()
	if err != nil {
		t.Fatalf("writeNonce() = %v, want nil", err)
	}
	if len(first) != 64 {
		t.Errorf("nonce length = %d, want 64 hex characters", len(first))
	}
	if _, hexErr := hex.DecodeString(first); hexErr != nil {
		t.Errorf("nonce is not valid hex: %v", hexErr)
	}

	info, err := os.Stat(noncePath())
	if err != nil {
		t.Fatalf("failed to stat nonce file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("nonce file mode = %o, want 0600", got)
	}
	stored, err := readNonce()
	if err != nil {
		t.Fatalf("readNonce() = %v, want nil", err)
	}
	if stored != first {
		t.Errorf("readNonce() = %q, want the written nonce %q", stored, first)
	}

	second, err := writeNonce()
	if err != nil {
		t.Fatalf("second writeNonce() = %v, want nil", err)
	}
	if second == first {
		t.Error("nonce was reused across restarts; it must be freshly random")
	}

	removeNonce()
	if _, err := os.Stat(noncePath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("removeNonce() left the nonce file behind: %v", err)
	}
}

// TestLockFilePermissions asserts the instance lock is owner-only (ADR-003).
func TestLockFilePermissions(t *testing.T) {
	isolateRuntime(t)

	lock, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock() = %v, want nil", err)
	}
	defer func() { _ = lock.Close() }()

	info, err := os.Stat(LockPath())
	if err != nil {
		t.Fatalf("failed to stat lock file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("lock file mode = %o, want 0600", got)
	}
}

// TestDaemonProbesReportUnreadableLock covers the cross-user case where the
// lock exists but belongs to another user: probes must report a distinguishable
// error rather than claiming no daemon is running.
func TestDaemonProbesReportUnreadableLock(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any file; unreadable-lock path is untestable as root")
	}
	isolateRuntime(t)

	//nolint:gosec // Test fixture: deliberately unreadable lock file
	if err := os.WriteFile(LockPath(), nil, 0000); err != nil {
		t.Fatal(err)
	}

	if running, err := DaemonRunning(); !errors.Is(err, ErrLockUnreadable) || running {
		t.Errorf("DaemonRunning() = (%v, %v), want (false, ErrLockUnreadable)", running, err)
	}
	if err := WaitForExit(context.Background()); !errors.Is(err, ErrLockUnreadable) {
		t.Errorf("WaitForExit() = %v, want ErrLockUnreadable", err)
	}
}

func TestAcquireLockExcludesSecondInstance(t *testing.T) {
	isolateRuntime(t)

	first, err := AcquireLock()
	if err != nil {
		t.Fatalf("first AcquireLock() = %v, want nil", err)
	}

	if _, lockErr := AcquireLock(); !errors.Is(lockErr, os.ErrExist) {
		t.Fatalf("second AcquireLock() = %v, want os.ErrExist", lockErr)
	}

	// Releasing the lock must allow a new instance to take ownership.
	if closeErr := first.Close(); closeErr != nil {
		t.Fatalf("failed to release lock: %v", closeErr)
	}
	second, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock() after release = %v, want nil", err)
	}
	if closeErr := second.Close(); closeErr != nil {
		t.Fatalf("failed to release second lock: %v", closeErr)
	}
}

func TestDaemonRunningTracksLockOwnership(t *testing.T) {
	isolateRuntime(t)

	running, err := DaemonRunning()
	if err != nil {
		t.Fatalf("DaemonRunning() = %v, want nil", err)
	}
	if running {
		t.Fatal("DaemonRunning() = true when no lock file exists, want false")
	}

	lock, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock() = %v, want nil", err)
	}
	defer func() { _ = lock.Close() }()

	running, err = DaemonRunning()
	if err != nil {
		t.Fatalf("DaemonRunning() = %v, want nil", err)
	}
	if !running {
		t.Error("DaemonRunning() = false while the lock is held, want true")
	}
}

func TestWaitForExitReturnsErrNoDaemonWithoutLockFile(t *testing.T) {
	isolateRuntime(t)

	if err := WaitForExit(context.Background()); !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("WaitForExit() = %v, want ErrNoDaemon", err)
	}
}

func TestWaitForExitUnblocksWhenLockIsReleased(t *testing.T) {
	isolateRuntime(t)

	lock, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock() = %v, want nil", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- WaitForExit(ctx) }()

	// Give the goroutine time to block on flock(LOCK_EX).
	time.Sleep(100 * time.Millisecond)
	_ = lock.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForExit() = %v, want nil after lock release", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WaitForExit() did not observe the released lock (daemon 'down' would hang)")
	}
}

func TestWaitForExitHonorsContextCancellation(t *testing.T) {
	isolateRuntime(t)

	lock, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock() = %v, want nil", err)
	}
	defer func() { _ = lock.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := WaitForExit(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForExit() = %v, want context.DeadlineExceeded", err)
	}
}

func TestPIDAlive(t *testing.T) {
	// Reap a child so its PID is guaranteed dead rather than a zombie.
	child := exec.CommandContext(context.Background(), "true")
	if err := child.Start(); err != nil {
		t.Fatalf("failed to start helper process: %v", err)
	}
	deadPID := child.Process.Pid
	if err := child.Wait(); err != nil {
		t.Fatalf("failed to reap helper process: %v", err)
	}

	tests := []struct {
		name string
		pid  int
		want bool
	}{
		{name: "current process", pid: os.Getpid(), want: true},
		{name: "reaped child", pid: deadPID, want: false},
		{name: "zero", pid: 0, want: false},
		{name: "negative", pid: -1, want: false},
		{name: "out of range", pid: 999999999, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PIDAlive(tt.pid); got != tt.want {
				t.Errorf("PIDAlive(%d) = %v, want %v", tt.pid, got, tt.want)
			}
		})
	}
}

// TestCleanStaleSocket is a regression test: an orphaned socket must be
// reclaimed, while a live daemon's socket must never be deleted underneath it.
func TestCleanStaleSocket(t *testing.T) {
	tests := []struct {
		name       string
		socket     bool
		pid        string
		wantSocket bool
		wantPID    bool
	}{
		{name: "nothing to clean"},
		{name: "orphaned socket without PID file", socket: true, wantSocket: false},
		{name: "orphaned socket with dead PID", socket: true, pid: "999999999", wantSocket: false},
		{name: "orphaned PID file without socket", pid: "999999999", wantPID: false},
		{name: "live daemon left untouched", socket: true, pid: strconv.Itoa(os.Getpid()), wantSocket: true, wantPID: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Table sanity: a "preserved" case must declare a live PID, a
			// "removed" case must declare a dead/absent one.
			if tt.wantSocket && tt.pid != strconv.Itoa(os.Getpid()) {
				t.Fatalf("case %q expects the socket to survive, so it needs a live PID", tt.name)
			}
			isolateRuntime(t)
			if err := os.MkdirAll(filepath.Dir(PIDPath()), 0700); err != nil {
				t.Fatal(err)
			}
			if tt.socket {
				writeSocket(t)
			}
			if tt.pid != "" {
				//nolint:gosec // Test fixture mirrors the 0644 PID file
				if err := os.WriteFile(PIDPath(), []byte(tt.pid+"\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			CleanStaleSocket()

			_, socketErr := os.Lstat(SocketPath())
			if gotSocket := socketErr == nil; gotSocket != tt.wantSocket {
				t.Errorf("socket present = %v, want %v", gotSocket, tt.wantSocket)
			}
			_, pidErr := os.Lstat(PIDPath())
			if gotPID := pidErr == nil; gotPID != tt.wantPID {
				t.Errorf("PID file present = %v, want %v", gotPID, tt.wantPID)
			}
		})
	}
}
