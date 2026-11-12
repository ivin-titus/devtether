package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/ivin-titus/devtether/internal/config"
	"golang.org/x/sys/unix"
)

// TestMain supports a fake detached-daemon child for daemonize tests. When
// DEVTETHER_CLI_TEST_CHILD=1 is set (inherited by the re-executed test
// binary), it records a spawn marker, writes one line to stdout (which the
// parent redirects to the log file), signals readiness on FD 3, and exits —
// without running the test suite or parsing the "up --config ..." args.
func TestMain(m *testing.M) {
	if os.Getenv("DEVTETHER_CLI_TEST_CHILD") == "1" {
		if marker := os.Getenv("DEVTETHER_CLI_TEST_CHILD_MARKER"); marker != "" {
			_ = os.WriteFile(marker, []byte("spawned"), 0600) //nolint:gosec // test writes its own temp marker
		}
		_, _ = fmt.Fprintln(os.Stdout, "fake daemon ready")
		if f := os.NewFile(3, "readiness"); f != nil {
			_, _ = f.WriteString(`{"pid":0,"port":"","configured_port":0,"reason":""}`)
			_ = f.Close()
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// safeLogDir returns a fresh owner-only temporary directory usable as a log
// directory. t.TempDir() perms are umask-dependent (0777 &^ umask), so the
// mode is pinned explicitly to keep the suite independent of the ambient
// umask (e.g. 0002 yields a group-writable 0775 that safe-open rejects).
func safeLogDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	//nolint:gosec // Test-controlled path; 0700 is the intended owner-only mode
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("failed to pin temp dir permissions: %v", err)
	}
	return dir
}

// statPath returns the unix.Stat_t for path by opening it read-only and
// fstat-ing the descriptor. It inspects ownership without relying on
// os.FileInfo.Sys() type assertions and works for both files and directories.
func statPath(t *testing.T, path string) *unix.Stat_t {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // test opens its own temp path
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatalf("fstat %s: %v", path, err)
	}
	return &st
}

// ownerUID returns the owning UID of path.
func ownerUID(t *testing.T, path string) int {
	t.Helper()
	return int(statPath(t, path).Uid)
}

func TestOpenLogFileForWriteSafe(t *testing.T) {
	dir := safeLogDir(t)

	f, err := openLogFileForWrite(dir)
	if err != nil {
		t.Fatalf("openLogFileForWrite() error = %v", err)
	}
	if _, werr := f.WriteString("first\n"); werr != nil {
		t.Fatalf("write: %v", werr)
	}
	if cerr := f.Close(); cerr != nil {
		t.Fatalf("close: %v", cerr)
	}

	// Reopen and confirm append semantics: existing content is retained.
	f2, err := openLogFileForWrite(dir)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	if _, werr := f2.WriteString("second\n"); werr != nil {
		t.Fatalf("append write: %v", werr)
	}
	if cerr := f2.Close(); cerr != nil {
		t.Fatalf("close: %v", cerr)
	}

	data, err := os.ReadFile(filepath.Join(dir, "devtether.log")) //nolint:gosec // reads test temp log
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Errorf("log content = %q, want %q", string(data), "first\nsecond\n")
	}

	// The created file must not be group/world-writable.
	info, err := os.Stat(filepath.Join(dir, "devtether.log"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0022 != 0 {
		t.Errorf("log file mode %o is group/world-writable", info.Mode().Perm())
	}
}

func TestOpenLogFileForWriteRejectsFileSymlink(t *testing.T) {
	dir := safeLogDir(t)
	target := filepath.Join(t.TempDir(), "victim.txt")
	const sentinel = "do-not-touch"
	if err := os.WriteFile(target, []byte(sentinel), 0600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "devtether.log")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := openLogFileForWrite(dir); err == nil {
		t.Fatal("expected error for symlinked log file, got nil")
	} else if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %v, want symlink mention", err)
	}

	data, err := os.ReadFile(target) //nolint:gosec // reads test temp target
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != sentinel {
		t.Errorf("symlink target modified: got %q, want %q", string(data), sentinel)
	}
}

func TestOpenLogFileForWriteRejectsDirSymlink(t *testing.T) {
	realDir := safeLogDir(t)
	linkDir := filepath.Join(t.TempDir(), ".logs")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := openLogFileForWrite(linkDir); err == nil {
		t.Fatal("expected error for symlinked log directory, got nil")
	} else if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %v, want symlink mention", err)
	}

	// No file may be created through the symlinked directory.
	if _, err := os.Stat(filepath.Join(realDir, "devtether.log")); !os.IsNotExist(err) {
		t.Errorf("devtether.log created through symlinked directory (stat err = %v)", err)
	}
}

func TestOpenLogFileForWriteRejectsDirSymlinkTrailingSlash(t *testing.T) {
	realDir := safeLogDir(t)
	linkDir := filepath.Join(t.TempDir(), ".logs")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// A trailing slash would force symlink resolution during open(2) even with
	// O_NOFOLLOW; filepath.Clean must strip it so the symlink is still rejected.
	if _, err := openLogFileForWrite(linkDir + "/"); err == nil {
		t.Fatal("expected error for symlinked log directory with trailing slash, got nil")
	} else if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %v, want symlink mention", err)
	}
	if _, err := os.Stat(filepath.Join(realDir, "devtether.log")); !os.IsNotExist(err) {
		t.Errorf("devtether.log created through trailing-slash symlink (stat err = %v)", err)
	}
}

func TestOpenLogFileForWriteRejectsCustomLogPathDirSymlink(t *testing.T) {
	realDir := safeLogDir(t)
	base := t.TempDir()
	customDir := filepath.Join(base, "custom-logs")
	if err := os.Symlink(realDir, customDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cfg := &config.Config{Settings: config.SettingsConfig{LogPath: customDir}}
	resolved, err := resolveLogDir(cfg)
	if err != nil {
		t.Fatalf("resolveLogDir: %v", err)
	}
	if resolved != customDir {
		t.Errorf("resolveLogDir = %q, want %q", resolved, customDir)
	}

	if _, err := openLogFileForWrite(resolved); err == nil {
		t.Fatal("expected error for custom log_path symlinked directory, got nil")
	}
	if _, err := os.Stat(filepath.Join(realDir, "devtether.log")); !os.IsNotExist(err) {
		t.Errorf("devtether.log created through custom symlinked directory (stat err = %v)", err)
	}
}

func TestOpenLogFileForWriteRejectsHardlink(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "logs")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A second name for an inode the attacker controls elsewhere.
	other := filepath.Join(base, "elsewhere.txt")
	if err := os.WriteFile(other, []byte("x"), 0600); err != nil {
		t.Fatalf("write other: %v", err)
	}
	if err := os.Link(other, filepath.Join(dir, "devtether.log")); err != nil {
		t.Fatalf("link: %v", err)
	}

	if _, err := openLogFileForWrite(dir); err == nil {
		t.Fatal("expected error for hard-linked log file, got nil")
	} else if !strings.Contains(err.Error(), "hard link") {
		t.Errorf("error = %v, want hard link mention", err)
	}
}

func TestOpenLogFileForWriteRejectsGroupWorldWritableDir(t *testing.T) {
	dir := safeLogDir(t)
	if err := os.Chmod(dir, 0777); err != nil { //nolint:gosec // test sets unsafe perms to verify rejection
		t.Fatalf("chmod: %v", err)
	}
	if _, err := openLogFileForWrite(dir); err == nil {
		t.Fatal("expected error for group/world-writable log directory, got nil")
	} else if !strings.Contains(err.Error(), "group/world-writable") {
		t.Errorf("error = %v, want permission mention", err)
	}
}

func TestOpenLogFileForWriteRejectsGroupWorldWritableFile(t *testing.T) {
	dir := safeLogDir(t)
	logFile := filepath.Join(dir, "devtether.log")
	if err := os.WriteFile(logFile, []byte("x"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(logFile, 0666); err != nil { //nolint:gosec // test sets unsafe perms to verify rejection
		t.Fatalf("chmod: %v", err)
	}
	if _, err := openLogFileForWrite(dir); err == nil {
		t.Fatal("expected error for group/world-writable log file, got nil")
	} else if !strings.Contains(err.Error(), "group/world-writable") {
		t.Errorf("error = %v, want permission mention", err)
	}
}

func TestOpenLogFileForWriteRejectsNonRegularFile(t *testing.T) {
	dir := safeLogDir(t)
	fifo := filepath.Join(dir, "devtether.log")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	// O_NONBLOCK must prevent an indefinite block on the FIFO; the file is
	// then rejected as non-regular.
	if _, err := openLogFileForWrite(dir); err == nil {
		t.Fatal("expected error for FIFO log file, got nil")
	} else if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("error = %v, want non-regular mention", err)
	}
}

func TestOpenLogFileForReadSafe(t *testing.T) {
	dir := safeLogDir(t)
	f, err := openLogFileForWrite(dir)
	if err != nil {
		t.Fatalf("write open: %v", err)
	}
	if _, werr := f.WriteString("hello\n"); werr != nil {
		t.Fatalf("write: %v", werr)
	}
	if cerr := f.Close(); cerr != nil {
		t.Fatalf("close: %v", cerr)
	}

	r, err := openLogFileForRead(dir)
	if err != nil {
		t.Fatalf("read open: %v", err)
	}
	defer func() { _ = r.Close() }()
	buf := make([]byte, 64)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "hello\n" {
		t.Errorf("read content = %q, want %q", string(buf[:n]), "hello\n")
	}
}

func TestOpenLogFileForReadRejectsFileSymlink(t *testing.T) {
	dir := safeLogDir(t)
	target := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "devtether.log")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := openLogFileForRead(dir); err == nil {
		t.Fatal("expected error for symlinked log file on read, got nil")
	} else if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %v, want symlink mention", err)
	}
}

func TestOpenLogFileForReadRejectsNonRegularFile(t *testing.T) {
	dir := safeLogDir(t)
	fifo := filepath.Join(dir, "devtether.log")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	// O_RDONLY|O_NONBLOCK succeeds on a FIFO; the S_ISREG check must reject it.
	if _, err := openLogFileForRead(dir); err == nil {
		t.Fatal("expected error for FIFO log file on read, got nil")
	} else if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("error = %v, want non-regular mention", err)
	}
}

func TestLastStartupErrorRejectsSymlink(t *testing.T) {
	dir := safeLogDir(t)
	target := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(target, []byte("Error: leaked secret\n"), 0600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "devtether.log")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// lastStartupError is best-effort and returns "" on any unsafe open, so it
	// must not leak the symlink target's contents.
	if got := lastStartupError(dir); got != "" {
		t.Errorf("lastStartupError() = %q, want empty (unsafe path must not be read)", got)
	}
}

// setupDaemonizeChild configures the fake-child environment and configPath
// for a daemonize test, returning a cleanup function.
func setupDaemonizeChild(t *testing.T, base string) {
	t.Helper()
	marker := filepath.Join(base, "child-marker")
	t.Setenv("DEVTETHER_CLI_TEST_CHILD", "1")
	t.Setenv("DEVTETHER_CLI_TEST_CHILD_MARKER", marker)

	origConfig := configPath
	configPath = filepath.Join(base, "devtether.yaml")
	t.Cleanup(func() { configPath = origConfig })
}

func childSpawned(t *testing.T, base string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(base, "child-marker"))
	return err == nil
}

func TestDaemonizeUnsafeCustomLogPathFailsBeforeSpawn(t *testing.T) {
	base := t.TempDir()
	realTarget := safeLogDir(t)
	linkDir := filepath.Join(base, "custom-logs")
	if err := os.Symlink(realTarget, linkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	setupDaemonizeChild(t, base)

	cfg := &config.Config{Settings: config.SettingsConfig{LogPath: linkDir}}
	if err := daemonize(cfg); err == nil {
		t.Fatal("expected daemonize to fail for symlinked custom log_path, got nil")
	}
	if childSpawned(t, base) {
		t.Error("detached child was spawned despite unsafe log path")
	}
	if _, err := os.Stat(filepath.Join(realTarget, "devtether.log")); !os.IsNotExist(err) {
		t.Errorf("log file created through symlinked custom log_path (stat err = %v)", err)
	}
}

func TestDaemonizeUnsafeDefaultLogsSymlinkFailsBeforeSpawn(t *testing.T) {
	base := t.TempDir()
	realTarget := safeLogDir(t)
	// Default log dir is <config-dir>/.logs; make it a symlink.
	if err := os.Symlink(realTarget, filepath.Join(base, ".logs")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	setupDaemonizeChild(t, base) // configPath = base/devtether.yaml

	cfg := &config.Config{} // no settings.log_path -> default <config-dir>/.logs
	if err := daemonize(cfg); err == nil {
		t.Fatal("expected daemonize to fail for symlinked .logs, got nil")
	}
	if childSpawned(t, base) {
		t.Error("detached child was spawned despite unsafe default .logs symlink")
	}
	if _, err := os.Stat(filepath.Join(realTarget, "devtether.log")); !os.IsNotExist(err) {
		t.Errorf("log file created through symlinked .logs (stat err = %v)", err)
	}
}

func TestDaemonizeSafePathStartsChildAndCapturesLog(t *testing.T) {
	base := t.TempDir()
	logDir := filepath.Join(base, ".logs")
	setupDaemonizeChild(t, base)

	cfg := &config.Config{} // default <config-dir>/.logs
	if err := daemonize(cfg); err != nil {
		t.Fatalf("daemonize failed: %v", err)
	}
	if !childSpawned(t, base) {
		t.Error("detached child was not spawned for safe log path")
	}

	logFile := filepath.Join(logDir, "devtether.log")
	data, err := os.ReadFile(logFile) //nolint:gosec // reads test temp log
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "fake daemon ready") {
		t.Errorf("log file missing child stdout; got %q", string(data))
	}
}

func TestResolveLogDirPreservesCustomRelativePath(t *testing.T) {
	// A custom relative log_path must be preserved as-is (CWD-relative), not
	// absolutized against the config file location.
	cfg := &config.Config{Settings: config.SettingsConfig{LogPath: "./relative-logs"}}
	got, err := resolveLogDir(cfg)
	if err != nil {
		t.Fatalf("resolveLogDir: %v", err)
	}
	if got != "./relative-logs" {
		t.Errorf("resolveLogDir = %q, want %q (relative path must be preserved)", got, "./relative-logs")
	}
}

func TestResolveLogDirDefaultNextToConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	base := t.TempDir()
	configPath = filepath.Join(base, "devtether.yaml")

	got, err := resolveLogDir(&config.Config{})
	if err != nil {
		t.Fatalf("resolveLogDir: %v", err)
	}
	want := filepath.Join(base, ".logs")
	if got != want {
		t.Errorf("resolveLogDir = %q, want %q", got, want)
	}
}

func TestOpenLogFileForWriteCreatesDefaultLogsOwnedByCurrentUser(t *testing.T) {
	base := t.TempDir()
	origConfig := configPath
	configPath = filepath.Join(base, "devtether.yaml")
	t.Cleanup(func() { configPath = origConfig })

	logDir, err := resolveLogDir(&config.Config{})
	if err != nil {
		t.Fatalf("resolveLogDir: %v", err)
	}
	if want := filepath.Join(base, ".logs"); logDir != want {
		t.Fatalf("resolveLogDir = %q, want %q", logDir, want)
	}
	if _, statErr := os.Stat(logDir); !os.IsNotExist(statErr) {
		t.Fatalf("precondition: .logs should not exist yet (stat err = %v)", statErr)
	}

	f, err := openLogFileForWrite(logDir)
	if err != nil {
		t.Fatalf("openLogFileForWrite: %v", err)
	}
	if _, werr := f.WriteString("boot\n"); werr != nil {
		t.Fatalf("write: %v", werr)
	}
	if cerr := f.Close(); cerr != nil {
		t.Fatalf("close: %v", cerr)
	}

	// The created directory must be owned by the current user and not
	// group/world-writable (0700 &^ umask).
	dinfo, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := ownerUID(t, logDir); got != os.Getuid() {
		t.Errorf("log dir owner uid = %d, want %d", got, os.Getuid())
	}
	if dinfo.Mode().Perm()&0022 != 0 {
		t.Errorf("log dir mode %o is group/world-writable", dinfo.Mode().Perm())
	}

	// The created log file must be owned by the current user, a single-link
	// regular file, and not group/world-writable.
	logFile := filepath.Join(logDir, "devtether.log")
	finfo, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := ownerUID(t, logFile); got != os.Getuid() {
		t.Errorf("log file owner uid = %d, want %d", got, os.Getuid())
	}
	if finfo.Mode().Perm()&0022 != 0 {
		t.Errorf("log file mode %o is group/world-writable", finfo.Mode().Perm())
	}
	if nlink := statPath(t, logFile).Nlink; nlink != 1 {
		t.Errorf("log file nlink = %d, want 1", nlink)
	}
}

func TestValidateLogDirRejectsForeignOwner(t *testing.T) {
	dir := safeLogDir(t)
	st := statPath(t, dir)
	st.Uid++ // simulate a foreign owner without needing root
	if err := validateLogDir(st, dir); err == nil {
		t.Fatal("expected foreign-owner error, got nil")
	} else {
		if !strings.Contains(err.Error(), "owned by uid") {
			t.Errorf("error = %v, want ownership mention", err)
		}
		if !strings.Contains(err.Error(), "settings.log_path") {
			t.Errorf("error = %v, want recovery guidance", err)
		}
	}
}

func TestValidateLogFileRejectsForeignOwner(t *testing.T) {
	dir := safeLogDir(t)
	path := filepath.Join(dir, "devtether.log")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	st := statPath(t, path)
	st.Uid++ // simulate a foreign owner without needing root
	if err := validateLogFile(st, path); err == nil {
		t.Fatal("expected foreign-owner error, got nil")
	} else {
		if !strings.Contains(err.Error(), "owned by uid") {
			t.Errorf("error = %v, want ownership mention", err)
		}
		if !strings.Contains(err.Error(), "settings.log_path") {
			t.Errorf("error = %v, want recovery guidance", err)
		}
	}
}

func TestOpenLogFileForWriteConcurrentCreation(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), ".logs")
	const workers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, err := openLogFileForWrite(logDir)
			if err != nil {
				errCh <- err
				return
			}
			_, werr := f.WriteString("x\n")
			cerr := f.Close()
			if werr != nil {
				errCh <- werr
				return
			}
			if cerr != nil {
				errCh <- cerr
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent openLogFileForWrite: %v", err)
	}

	// The single log file must remain valid: regular, one link, owned by us.
	st := statPath(t, filepath.Join(logDir, "devtether.log"))
	if st.Nlink != 1 {
		t.Errorf("log file nlink = %d, want 1", st.Nlink)
	}
}
