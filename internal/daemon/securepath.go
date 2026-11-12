package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ErrNoRuntimeDir is returned when the runtime directory leaf does not exist.
// It wraps os.ErrNotExist so callers preserve the established "not running"
// semantics while remaining distinguishable from an unsafe directory.
var ErrNoRuntimeDir = errors.New("daemon: no runtime directory")

// errInvalidRuntimeBase marks a failure to validate the XDG runtime base
// itself (as opposed to its leaf). Only this class of failure falls back to
// the temporary runtime directory; an unsafe or absent leaf is never
// bypassed by falling back.
var errInvalidRuntimeBase = errors.New("daemon: invalid runtime base")

func noRuntimeDir(path string) error {
	return fmt.Errorf("%w: %s", errors.Join(os.ErrNotExist, ErrNoRuntimeDir), path)
}

// dirClass classifies a directory component during secure traversal.
type dirClass int

const (
	classReject        dirClass = iota
	classTrustedSystem          // root-owned, not group/world-writable
	classStickyTemp             // sticky bit set, root- or self-owned (temp parent)
	classUserOwned              // self-owned, not group/world-writable
)

// classifyStat applies the runtime/state ancestor policy to a directory stat.
// uid is the owning user the directory must belong to for user-owned classes.
func classifyStat(st *unix.Stat_t, uid int) dirClass {
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return classReject
	}
	owner := int(st.Uid)
	perm := int(st.Mode) & 0777
	groupWorldWritable := perm&0022 != 0
	sticky := st.Mode&unix.S_ISVTX != 0
	switch {
	case owner == 0 && !groupWorldWritable:
		return classTrustedSystem
	case sticky && (owner == 0 || owner == uid):
		return classStickyTemp
	case owner == uid && !groupWorldWritable:
		return classUserOwned
	default:
		return classReject
	}
}

const openDirFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

// secureOpenDir opens an existing directory at path without following
// symlinks in user-controlled components. It walks from the filesystem root
// descriptor-relative, pinning each component, and returns the directory FD,
// its class, and its final stat. Only root-owned symlinks encountered while
// still in trusted-system territory are followed (system aliases such as
// macOS /var -> /private/var, a relative target); every other symlink is
// rejected.
func secureOpenDir(path string, uid int) (fd int, class dirClass, st *unix.Stat_t, err error) {
	return secureOpenDirDepth(path, uid, 0)
}

func secureOpenDirDepth(path string, uid, depth int) (int, dirClass, *unix.Stat_t, error) {
	if depth > 40 {
		return -1, classReject, nil, errors.New("daemon: too many runtime directory symlinks")
	}
	if !filepath.IsAbs(path) {
		return -1, classReject, nil, fmt.Errorf("daemon: runtime path %q is not absolute", path)
	}

	fd, err := unix.Open("/", openDirFlags, 0)
	if err != nil {
		return -1, classReject, nil, fmt.Errorf("daemon: failed to open filesystem root: %w", err)
	}

	curPath := "/"
	curClass := classTrustedSystem
	comps := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")

	for i, comp := range comps {
		if comp == "" || comp == "." {
			continue
		}
		newFD, oerr := unix.Openat(fd, comp, openDirFlags, 0)
		if oerr != nil {
			if oerr == unix.ELOOP || oerr == unix.ENOTDIR {
				if target, ok := rootAliasTarget(fd, curPath, comp, curClass); ok {
					_ = unix.Close(fd)
					rest := strings.Join(comps[i+1:], "/")
					next := target
					if rest != "" {
						next = filepath.Join(target, rest)
					}
					return secureOpenDirDepth(next, uid, depth+1)
				}
			}
			_ = unix.Close(fd)
			return -1, classReject, nil, fmt.Errorf("daemon: failed to open runtime path component %q: %w", comp, oerr)
		}

		var cst unix.Stat_t
		if ferr := unix.Fstat(newFD, &cst); ferr != nil {
			_ = unix.Close(newFD)
			_ = unix.Close(fd)
			return -1, classReject, nil, fmt.Errorf("daemon: failed to stat runtime path component %q: %w", comp, ferr)
		}
		cls := classifyStat(&cst, uid)
		if cls == classReject {
			_ = unix.Close(newFD)
			_ = unix.Close(fd)
			return -1, classReject, nil, fmt.Errorf("daemon: unsafe runtime path component %q (possible attack)", comp)
		}

		_ = unix.Close(fd)
		fd = newFD
		curClass = cls
		curPath = filepath.Join(curPath, comp)
	}

	var fst unix.Stat_t
	if ferr := unix.Fstat(fd, &fst); ferr != nil {
		_ = unix.Close(fd)
		return -1, classReject, nil, fmt.Errorf("daemon: failed to stat runtime directory: %w", ferr)
	}
	return fd, curClass, &fst, nil
}

// rootAliasTarget reports whether comp (relative to the open parent fd) is a
// root-owned symlink that may be followed as a system alias. Aliases are only
// permitted while still in trusted-system territory. Relative targets (the
// real macOS form, e.g. /tmp -> private/tmp) are anchored to the link's
// containing directory; the cleaned absolute result is then re-walked from
// the filesystem root under the same per-component policy, so an alias can
// never smuggle traversal into unvalidated territory.
func rootAliasTarget(parentFD int, curPath, comp string, curClass dirClass) (string, bool) {
	if curClass != classTrustedSystem {
		return "", false
	}
	var lst unix.Stat_t
	if err := unix.Fstatat(parentFD, comp, &lst, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return "", false
	}
	if lst.Mode&unix.S_IFMT != unix.S_IFLNK || int(lst.Uid) != 0 {
		return "", false
	}
	target, err := os.Readlink(filepath.Join(curPath, comp))
	if err != nil {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(curPath, target)
	}
	return filepath.Clean(target), true
}

// secureEnsureLeaf validates or securely creates a single-component leaf
// directory under the already-open, already-validated base directory. It never
// follows a symlink leaf and never repairs an unsafe existing leaf.
func secureEnsureLeaf(baseFD int, basePath, leaf string, uid int, create bool) (string, error) {
	full := filepath.Join(basePath, leaf)
	fd, err := unix.Openat(baseFD, leaf, openDirFlags, 0)
	if err != nil {
		switch {
		case errors.Is(err, unix.ENOENT):
			if !create {
				return "", noRuntimeDir(full)
			}
			if merr := unix.Mkdirat(baseFD, leaf, 0700); merr != nil {
				return "", fmt.Errorf("daemon: failed to create runtime directory %s: %w", full, merr)
			}
			fd, err = unix.Openat(baseFD, leaf, openDirFlags, 0)
			if err != nil {
				return "", fmt.Errorf("daemon: failed to open created runtime directory %s: %w", full, err)
			}
		case errors.Is(err, unix.ELOOP), errors.Is(err, unix.ENOTDIR):
			// Linux reports ENOTDIR (not ELOOP) for O_DIRECTORY|O_NOFOLLOW on a
			// symlink-to-directory. Distinguish a symlink leaf from an ordinary
			// non-directory so the refusal message stays explicit (ADR-003).
			var lst unix.Stat_t
			if ferr := unix.Fstatat(baseFD, leaf, &lst, unix.AT_SYMLINK_NOFOLLOW); ferr == nil && lst.Mode&unix.S_IFMT == unix.S_IFLNK {
				return "", fmt.Errorf("daemon: runtime directory %s is a symlink (possible attack)", full)
			}
			return "", fmt.Errorf("daemon: %s exists but is not a directory", full)
		default:
			return "", fmt.Errorf("daemon: failed to open runtime directory %s: %w", full, err)
		}
	}
	defer func() { _ = unix.Close(fd) }()

	var st unix.Stat_t
	if ferr := unix.Fstat(fd, &st); ferr != nil {
		return "", fmt.Errorf("daemon: failed to stat runtime directory %s: %w", full, ferr)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", fmt.Errorf("daemon: runtime path %s exists but is not a directory", full)
	}
	//nolint:gosec // G115: Unix UIDs fit in uint32 safely
	if int(st.Uid) != uid {
		return "", fmt.Errorf("daemon: runtime directory %s is owned by uid %d, not %d (possible attack)", full, st.Uid, uid)
	}
	if int(st.Mode)&0022 != 0 {
		return "", fmt.Errorf("daemon: runtime directory %s has unsafe group/world-writable permissions", full)
	}
	return full, nil
}

// secureTempBase returns an open FD and path for a validated temporary parent
// directory. It never trusts os.TempDir()/TMPDIR blindly: each candidate must
// be absolute, symlink-free in user-controlled components, and classify as a
// sticky-temp or user-owned directory.
func secureTempBase(uid int) (int, string, error) {
	candidates := []string{os.TempDir(), "/tmp"}
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if c == "" || !filepath.IsAbs(c) || seen[c] {
			continue
		}
		seen[c] = true
		fd, class, _, err := secureOpenDir(c, uid)
		if err != nil {
			continue
		}
		// For uid 0 a root-owned non-writable directory is the user's own
		// territory (classTrustedSystem); accept it as a temp base too.
		acceptable := class == classStickyTemp || class == classUserOwned || (uid == 0 && class == classTrustedSystem)
		if acceptable {
			return fd, c, nil
		}
		_ = unix.Close(fd)
	}
	return -1, "", errors.New("daemon: no safe temporary directory for runtime state")
}

func xdgRuntimeDir(xdg string, uid int, create bool) (string, error) {
	fd, _, st, err := secureOpenDir(xdg, uid)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errInvalidRuntimeBase, err)
	}
	defer func() { _ = unix.Close(fd) }()
	// Ownership is checked from the stat directly: for uid 0 a root-owned
	// directory classifies as trusted-system, not user-owned, but is still the
	// correct owner. The walk above already rejected unsafe components.
	//nolint:gosec // G115: Unix UIDs fit in uint32 safely
	if int(st.Uid) != uid {
		return "", fmt.Errorf("%w: XDG_RUNTIME_DIR %s is not owned by uid %d", errInvalidRuntimeBase, xdg, uid)
	}
	if int(st.Mode)&0777 != 0700 {
		return "", fmt.Errorf("%w: XDG_RUNTIME_DIR %s must have mode 0700", errInvalidRuntimeBase, xdg)
	}
	return secureEnsureLeaf(fd, xdg, "devtether", uid, create)
}

func tempRuntimeDir(uid int, create bool) (string, error) {
	fd, base, err := secureTempBase(uid)
	if err != nil {
		return "", err
	}
	defer func() { _ = unix.Close(fd) }()
	return secureEnsureLeaf(fd, base, fmt.Sprintf("devtether-%d", uid), uid, create)
}

// resolveRuntimeDirUID is the uid-injectable core of resolveRuntimeDir. It is
// unexported so production callers always resolve with os.Getuid().
func resolveRuntimeDirUID(create bool, uid int) (string, error) {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		dir, err := xdgRuntimeDir(xdg, uid, create)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, errInvalidRuntimeBase) {
			// The base is valid; a missing or unsafe leaf is authoritative.
			return "", err
		}
		// Invalid XDG base: fall through to the temporary runtime directory.
	}
	return tempRuntimeDir(uid, create)
}

// resolveRuntimeDir returns the validated runtime directory. When create is
// true it securely creates and validates the leaf; when false it requires an
// existing, validated leaf and never returns a candidate for an absent leaf.
func resolveRuntimeDir(create bool) (string, error) {
	return resolveRuntimeDirUID(create, os.Getuid())
}

// runtimeFilePath resolves the runtime directory and joins a leaf filename.
func runtimeFilePath(name string) (string, error) {
	dir, err := resolveRuntimeDir(false)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
