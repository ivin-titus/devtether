package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ivin-titus/devtether/internal/config"
	"golang.org/x/sys/unix"
)

// logFileName is the fixed name of the daemon log file inside the log
// directory. Keeping it constant lets the safe-open helper anchor the final
// path component with openat(2) relative to an already-open directory fd.
const logFileName = "devtether.log"

// resolveLogDir returns the configured log directory, or the default
// <config-dir>/.logs when settings.log_path is unset. Custom relative paths
// keep their existing process-CWD-relative resolution.
func resolveLogDir(cfg *config.Config) (string, error) {
	if cfg != nil && cfg.Settings.LogPath != "" {
		return cfg.Settings.LogPath, nil
	}
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve config path: %w", err)
	}
	return filepath.Join(filepath.Dir(absConfig), ".logs"), nil
}

// logFilePath returns the display path of the daemon log file for a log
// directory. It is used only for messages; opening always goes through
// openLogFileForWrite/openLogFileForRead.
func logFilePath(logDir string) string {
	return filepath.Join(logDir, logFileName)
}

// openLogFileForWrite creates the log directory if needed and opens the log
// file for appending. It refuses symlinked or otherwise unsafe paths and
// fails closed before any detached child is spawned.
func openLogFileForWrite(logDir string) (*os.File, error) {
	//nolint:gosec // Log directory uses 0700 per ADR-003 security model
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}
	return openLogFile(logDir, true)
}

// openLogFileForRead opens the log file for reading without creating
// anything. It applies the same safety checks as the write path.
func openLogFileForRead(logDir string) (*os.File, error) {
	return openLogFile(logDir, false)
}

// openLogFile opens <logDir>/devtether.log safely.
//
// Security boundary: only the final directory component and the fixed
// devtether.log name are protected. The directory is opened with
// O_DIRECTORY|O_NOFOLLOW and validated with Fstat; the file is then opened
// with openat(2) relative to that directory fd using O_NOFOLLOW, so neither
// the directory nor the file can be a symlink and no path-based
// check-then-open race exists for these final components. Intermediate
// ancestors of logDir may still be symlinks (e.g. macOS /var -> /private/var);
// this is deliberate and matches the pre-existing path resolution.
//
// The opened directory must be a real directory owned by the current user
// and not group/world-writable. The opened file must be a regular file owned
// by the current user, have exactly one hard link, and not be
// group/world-writable.
func openLogFile(logDir string, forWrite bool) (*os.File, error) {
	dirFD, err := openLogDir(logDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(dirFD) }()

	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	if forWrite {
		flags = unix.O_WRONLY | unix.O_CREAT | unix.O_APPEND | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	}
	fd, err := unix.Openat(dirFD, logFileName, flags, 0644)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, fmt.Errorf("log file %s is a symlink (unsafe)", logFilePath(logDir))
		}
		if errors.Is(err, unix.ENXIO) {
			return nil, fmt.Errorf("log file %s is not a regular file (unsafe)", logFilePath(logDir))
		}
		return nil, fmt.Errorf("failed to open log file %s: %w", logFilePath(logDir), err)
	}

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("failed to stat log file %s: %w", logFilePath(logDir), err)
	}
	if err := validateLogFile(&st, logFilePath(logDir)); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	return os.NewFile(uintptr(fd), logFilePath(logDir)), nil
}

// openLogDir opens logDir as a directory without following a final-component
// symlink and validates it. It returns a raw fd suitable for openat(2); the
// caller must close it.
func openLogDir(logDir string) (int, error) {
	if logDir == "" {
		return -1, fmt.Errorf("log directory is empty")
	}
	// filepath.Clean strips trailing separators so O_NOFOLLOW applies to the
	// real final component (a trailing slash would force symlink resolution).
	cleanDir := filepath.Clean(logDir)
	fd, err := unix.Open(cleanDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		// The open already failed safely; the checks below only improve the
		// error message and are never the security boundary. Linux reports
		// ENOTDIR (not ELOOP) for O_DIRECTORY|O_NOFOLLOW on a symlink to a
		// directory, so also detect the symlink case explicitly.
		if errors.Is(err, unix.ELOOP) {
			return -1, fmt.Errorf("log directory %s is a symlink (unsafe)", logDir)
		}
		if info, lerr := os.Lstat(cleanDir); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			return -1, fmt.Errorf("log directory %s is a symlink (unsafe)", logDir)
		}
		return -1, fmt.Errorf("failed to open log directory %s: %w", logDir, err)
	}

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("failed to stat log directory %s: %w", logDir, err)
	}
	if err := validateLogDir(&st, logDir); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

// logOwnershipGuidance is appended to ownership-mismatch errors. It names the
// supported recovery paths and deliberately performs no automatic repair: the
// safe-open checks are never bypassed.
const logOwnershipGuidance = "use the same execution mode that created it (e.g. sudo vs non-sudo), or set settings.log_path to a separate directory; if the path is on a filesystem that does not honor Unix ownership (e.g. NTFS/exFAT/FUSE), point settings.log_path at a native filesystem"

// logOwnershipError formats an ownership mismatch with the affected path, the
// actual owner UID, and the expected UID, plus recovery guidance.
func logOwnershipError(kind, path string, st *unix.Stat_t) error {
	return fmt.Errorf("%s %s is owned by uid %d, not %d (unsafe); %s", kind, path, st.Uid, os.Getuid(), logOwnershipGuidance)
}

func validateLogDir(st *unix.Stat_t, path string) error {
	mode := st.Mode
	if mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("log directory %s is not a directory", path)
	}
	if int(st.Uid) != os.Getuid() {
		return logOwnershipError("log directory", path, st)
	}
	if mode&0022 != 0 {
		return fmt.Errorf("log directory %s is group/world-writable (unsafe)", path)
	}
	return nil
}

func validateLogFile(st *unix.Stat_t, path string) error {
	mode := st.Mode
	if mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("log file %s is not a regular file (unsafe)", path)
	}
	if int(st.Uid) != os.Getuid() {
		return logOwnershipError("log file", path, st)
	}
	if st.Nlink != 1 {
		return fmt.Errorf("log file %s has %d hard links (unsafe)", path, st.Nlink)
	}
	if mode&0022 != 0 {
		return fmt.Errorf("log file %s is group/world-writable (unsafe)", path)
	}
	return nil
}
