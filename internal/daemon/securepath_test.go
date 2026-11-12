package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// secureTestUID is the real process UID. Pure policy helpers accept synthetic
// UIDs, but every test here runs with the actual os.Getuid() unless it
// explicitly exercises the uid-injectable helpers.
func secureTestUID() int { return os.Getuid() }

// xdgBase returns a fresh XDG_RUNTIME_DIR candidate with exact 0700 perms.
func xdgBase(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	//nolint:gosec // Test fixture: XDG_RUNTIME_DIR must be exactly 0700.
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestClassifyStatPolicy(t *testing.T) {
	uid := secureTestUID()
	foreign := uid + 100000
	if foreign == 0 {
		foreign = 4242
	}

	// Stat_t.Mode is uint32 on Linux but uint16 on macOS/BSD, so the fixtures
	// must use untyped constant expressions — a typed perm variable would not
	// be assignable to the field on both platform families.
	tests := []struct {
		name string
		st   *unix.Stat_t
		want dirClass
	}{
		{"root-owned 0755 is trusted system", &unix.Stat_t{Mode: unix.S_IFDIR | 0755, Uid: 0}, classTrustedSystem},
		{"root-owned 0700 is trusted system", &unix.Stat_t{Mode: unix.S_IFDIR | 0700, Uid: 0}, classTrustedSystem},
		{"root-owned 1777 sticky temp", &unix.Stat_t{Mode: unix.S_IFDIR | 01777, Uid: 0}, classStickyTemp},
		{"root-owned 0777 non-sticky rejected", &unix.Stat_t{Mode: unix.S_IFDIR | 0777, Uid: 0}, classReject},
		{"self-owned 0700 user owned", &unix.Stat_t{Mode: unix.S_IFDIR | 0700, Uid: uint32(uid)}, classUserOwned},      //nolint:gosec // test fixture
		{"self-owned 0755 user owned", &unix.Stat_t{Mode: unix.S_IFDIR | 0755, Uid: uint32(uid)}, classUserOwned},      //nolint:gosec // test fixture
		{"self-owned 0777 rejected", &unix.Stat_t{Mode: unix.S_IFDIR | 0777, Uid: uint32(uid)}, classReject},           //nolint:gosec // test fixture
		{"self-owned sticky temp", &unix.Stat_t{Mode: unix.S_IFDIR | 01777, Uid: uint32(uid)}, classStickyTemp},        //nolint:gosec // test fixture
		{"foreign-owned 0700 rejected", &unix.Stat_t{Mode: unix.S_IFDIR | 0700, Uid: uint32(foreign)}, classReject},    //nolint:gosec // test fixture
		{"foreign-owned sticky rejected", &unix.Stat_t{Mode: unix.S_IFDIR | 01777, Uid: uint32(foreign)}, classReject}, //nolint:gosec // test fixture
		{"non-directory rejected", &unix.Stat_t{Mode: unix.S_IFREG | 0644, Uid: uint32(uid)}, classReject},             //nolint:gosec // test fixture
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyStat(tt.st, uid); got != tt.want {
				t.Errorf("classifyStat() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestResolveRuntimeDirXDGCreateAndRequireExisting covers the create=true /
// create=false contract: creation is secure, and the no-create resolver
// requires an existing validated leaf.
func TestResolveRuntimeDirXDGCreateAndRequireExisting(t *testing.T) {
	xdg := xdgBase(t)
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	// create=false with an absent leaf must fail closed: no usable path.
	if _, err := resolveRuntimeDir(false); !errors.Is(err, ErrNoRuntimeDir) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolveRuntimeDir(false) with absent leaf = %v, want ErrNoRuntimeDir wrapping os.ErrNotExist", err)
	}
	if path, err := SocketPath(); err == nil || path != "" {
		t.Fatalf("SocketPath() with absent leaf = (%q, %v), want error and no path", path, err)
	}

	// create=true securely creates the leaf.
	dir, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir() = %v, want nil", err)
	}
	if want := filepath.Join(xdg, "devtether"); dir != want {
		t.Fatalf("RuntimeDir() = %q, want %q", dir, want)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("runtime leaf mode = %o, want 0700", got)
	}

	// Now the no-create resolver accepts the existing validated leaf.
	if _, err := resolveRuntimeDir(false); err != nil {
		t.Fatalf("resolveRuntimeDir(false) with existing leaf = %v, want nil", err)
	}
}

// TestResolveRuntimeDirRejects0777Leaf is the regression test for the
// existing-directory permission defect: a uid-owned 0777 runtime directory
// must be rejected even though MkdirAll would never create it that way.
func TestResolveRuntimeDirRejects0777Leaf(t *testing.T) {
	xdg := xdgBase(t)
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	leaf := filepath.Join(xdg, "devtether")
	//nolint:gosec // Test fixture.
	if err := os.Mkdir(leaf, 0700); err != nil {
		t.Fatal(err)
	}
	// Mkdir is umask-affected; force the exact unsafe mode.
	//nolint:gosec // Test fixture: deliberately unsafe permissions.
	if err := os.Chmod(leaf, 0777); err != nil {
		t.Fatal(err)
	}

	if _, err := RuntimeDir(); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("RuntimeDir() with 0777 leaf = %v, want unsafe-permissions rejection", err)
	}
	if _, err := resolveRuntimeDir(false); err == nil {
		t.Fatal("resolveRuntimeDir(false) accepted a 0777 leaf, want rejection")
	}
}

// TestResolveRuntimeDirAcceptsReasonableLeafPerms verifies an existing leaf
// with acceptable (non group/world-writable) permissions is accepted.
func TestResolveRuntimeDirAcceptsReasonableLeafPerms(t *testing.T) {
	for _, perm := range []os.FileMode{0700, 0750, 0755} {
		xdg := xdgBase(t)
		t.Setenv("XDG_RUNTIME_DIR", xdg)
		leaf := filepath.Join(xdg, "devtether")
		//nolint:gosec // Test fixture: explicit permission matrix.
		if err := os.Mkdir(leaf, perm); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveRuntimeDir(false); err != nil {
			t.Errorf("resolveRuntimeDir(false) with leaf mode %o = %v, want nil", perm, err)
		}
	}
}

// TestXDGRuntimeDirRejectsNon0700Base verifies the XDG base must be exactly
// 0700; any other permission set invalidates the base (and only then does the
// resolver fall back to the temporary directory).
func TestXDGRuntimeDirRejectsNon0700Base(t *testing.T) {
	uid := secureTestUID()
	for _, perm := range []os.FileMode{0755, 0777, 0750} {
		base := t.TempDir()
		//nolint:gosec // Test fixture: explicit permission matrix.
		if err := os.Chmod(base, perm); err != nil {
			t.Fatal(err)
		}
		if _, err := xdgRuntimeDir(base, uid, true); !errors.Is(err, errInvalidRuntimeBase) {
			t.Errorf("xdgRuntimeDir with base mode %o = %v, want errInvalidRuntimeBase", perm, err)
		}
	}
}

// TestXDGRuntimeDirAccepts0700Base is the positive counterpart.
func TestXDGRuntimeDirAccepts0700Base(t *testing.T) {
	base := xdgBase(t)
	dir, err := xdgRuntimeDir(base, secureTestUID(), true)
	if err != nil {
		t.Fatalf("xdgRuntimeDir() = %v, want nil", err)
	}
	if want := filepath.Join(base, "devtether"); dir != want {
		t.Errorf("xdgRuntimeDir() = %q, want %q", dir, want)
	}
}

// TestSymlinkLeafRejected covers both the ELOOP and the Linux ENOTDIR
// behaviors of O_DIRECTORY|O_NOFOLLOW on a symlink-to-directory leaf: the
// refusal must be explicit and must not fall back to another location.
func TestSymlinkLeafRejected(t *testing.T) {
	xdg := xdgBase(t)
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(xdg, "devtether")); err != nil {
		t.Skipf("cannot create symlink on this platform: %v", err)
	}

	if _, err := RuntimeDir(); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("RuntimeDir() on symlink leaf = %v, want explicit symlink refusal", err)
	}
	if _, err := resolveRuntimeDir(false); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("resolveRuntimeDir(false) on symlink leaf = %v, want explicit symlink refusal", err)
	}
}

// TestForeignOwnedLeafRejectedSynthetic uses the uid-injectable helper with a
// synthetic foreign UID: ownership policy is exercised without needing a
// second real user. Actual cross-UID behavior is covered by the root-gated
// test below.
func TestForeignOwnedLeafRejectedSynthetic(t *testing.T) {
	xdg := xdgBase(t)
	uid := secureTestUID()
	leaf := filepath.Join(xdg, "devtether")
	//nolint:gosec // Test fixture.
	if err := os.Mkdir(leaf, 0700); err != nil {
		t.Fatal(err)
	}

	fd, _, _, err := secureOpenDir(xdg, uid)
	if err != nil {
		t.Fatalf("secureOpenDir() = %v", err)
	}
	defer func() { _ = unix.Close(fd) }()

	foreign := uid + 100000
	if foreign == 0 {
		foreign = 4242
	}
	if _, err := secureEnsureLeaf(fd, xdg, "devtether", foreign, false); err == nil ||
		!strings.Contains(err.Error(), "owned by uid") {
		t.Fatalf("secureEnsureLeaf with foreign uid = %v, want ownership rejection", err)
	}
}

// TestUserControlledSymlinksRejected verifies arbitrary user-controlled
// symlinks in environment-provided paths are rejected: a symlinked XDG base
// or TMPDIR is never followed into attacker-chosen territory.
func TestUserControlledSymlinksRejected(t *testing.T) {
	t.Run("xdg base symlink invalidates base", func(t *testing.T) {
		real := xdgBase(t)
		linkParent := xdgBase(t)
		link := filepath.Join(linkParent, "link")
		if err := os.Symlink(real, link); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
		if _, err := xdgRuntimeDir(link, secureTestUID(), true); !errors.Is(err, errInvalidRuntimeBase) {
			t.Fatalf("xdgRuntimeDir on user symlink = %v, want errInvalidRuntimeBase", err)
		}
	})

	t.Run("relative user symlink stays rejected", func(t *testing.T) {
		real := xdgBase(t)
		linkParent := xdgBase(t)
		link := filepath.Join(linkParent, "link")
		// Relative target resolved against linkParent; the link is user-owned,
		// so it is never eligible as a system alias regardless of where it
		// points.
		if err := os.Symlink(filepath.Base(real), link); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
		if _, err := xdgRuntimeDir(link, secureTestUID(), true); !errors.Is(err, errInvalidRuntimeBase) {
			t.Fatalf("xdgRuntimeDir on user-owned relative symlink = %v, want errInvalidRuntimeBase", err)
		}
	})

	t.Run("tmpdir symlink falls back to /tmp", func(t *testing.T) {
		real := t.TempDir()
		linkParent := xdgBase(t)
		link := filepath.Join(linkParent, "tmp-link")
		if err := os.Symlink(real, link); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
		t.Setenv("TMPDIR", link)
		t.Setenv("XDG_RUNTIME_DIR", "")

		leaf := filepath.Join("/tmp", "devtether-"+strconv.Itoa(secureTestUID()))
		t.Cleanup(func() { _ = os.RemoveAll(leaf) })

		dir, err := resolveRuntimeDir(true)
		if err != nil {
			t.Fatalf("resolveRuntimeDir(true) = %v, want fallback to /tmp", err)
		}
		if !strings.HasPrefix(dir, string(filepath.Separator)+"tmp"+string(filepath.Separator)) {
			t.Errorf("resolveRuntimeDir(true) = %q, want a /tmp-based fallback, not the symlink target", dir)
		}
	})
}

// TestStickyTempChildCases covers the sticky temporary parent policy: the
// per-UID child must be uid-owned, a real directory, not a symlink, and not
// group/world-writable; anything else is a clear failure with no repair.
func TestStickyTempChildCases(t *testing.T) {
	uid := secureTestUID()
	fd, base, err := secureTempBase(uid)
	if err != nil {
		t.Fatalf("secureTempBase() = %v", err)
	}
	defer func() { _ = unix.Close(fd) }()

	leaf := "devtether-securepath-test"
	leafPath := filepath.Join(base, leaf)
	t.Cleanup(func() { _ = os.RemoveAll(leafPath) })

	// Symlink child is rejected.
	if err := os.Symlink(t.TempDir(), leafPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := secureEnsureLeaf(fd, base, leaf, uid, false); err == nil ||
		!strings.Contains(err.Error(), "symlink") {
		t.Fatalf("secureEnsureLeaf on symlink child = %v, want symlink rejection", err)
	}
	_ = os.Remove(leafPath)

	// Foreign-owned child (synthetic uid) is rejected.
	//nolint:gosec // Test fixture.
	if err := os.Mkdir(leafPath, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := uid + 100000
	if foreign == 0 {
		foreign = 4242
	}
	if _, err := secureEnsureLeaf(fd, base, leaf, foreign, false); err == nil {
		t.Fatal("secureEnsureLeaf accepted a foreign-owned child, want rejection")
	}

	// Group/world-writable child is rejected.
	//nolint:gosec // Test fixture: deliberately unsafe permissions.
	if err := os.Chmod(leafPath, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := secureEnsureLeaf(fd, base, leaf, uid, false); err == nil {
		t.Fatal("secureEnsureLeaf accepted a 0777 child, want rejection")
	}

	// A clean uid-owned 0700 child is accepted.
	//nolint:gosec // Test fixture.
	if err := os.Chmod(leafPath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := secureEnsureLeaf(fd, base, leaf, uid, false); err != nil {
		t.Fatalf("secureEnsureLeaf on clean child = %v, want nil", err)
	}
}

// TestTempFallbackCreatesIsolatedLeaf verifies the no-XDG fallback creates a
// per-UID leaf under a validated sticky temp parent.
func TestTempFallbackCreatesIsolatedLeaf(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", "")

	leaf := filepath.Join("/tmp", "devtether-"+strconv.Itoa(secureTestUID()))
	existed := false
	if _, err := os.Lstat(leaf); err == nil {
		existed = true
	}
	if existed {
		t.Skipf("refusing to touch pre-existing %s", leaf)
	}
	t.Cleanup(func() { _ = os.RemoveAll(leaf) })

	dir, err := resolveRuntimeDir(true)
	if err != nil {
		t.Fatalf("resolveRuntimeDir(true) = %v, want nil", err)
	}
	if dir != leaf {
		t.Errorf("resolveRuntimeDir(true) = %q, want %q", dir, leaf)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("temp runtime leaf mode = %o, want 0700", got)
	}
}

// TestRootOwnedSystemAlias is root-gated: it verifies the approved exception
// that a root-owned absolute symlink in trusted-system territory is followed,
// while documenting that this cannot be exercised without privileges.
func TestRootOwnedSystemAlias(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to create root-owned symlinks")
	}
	base := filepath.Join("/", "devtether-alias-test")
	real := filepath.Join(base, "real")
	alias := filepath.Join(base, "alias")
	//nolint:gosec // G301: root-owned trusted-system fixture directory
	if err := os.MkdirAll(real, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })

	fd, _, _, err := secureOpenDir(alias, 0)
	if err != nil {
		t.Fatalf("secureOpenDir on root-owned alias = %v, want nil", err)
	}
	_ = unix.Close(fd)
}

// TestRootOwnedRelativeSystemAlias is root-gated: it verifies that a
// root-owned RELATIVE symlink — the real macOS form (/tmp -> private/tmp,
// /var -> private/var) — is followed as a system alias, and that a ".."
// relative target escaping into foreign-owned territory is still rejected by
// the re-walked traversal policy. TestRootOwnedSystemAlias covers the
// absolute-target form.
func TestRootOwnedRelativeSystemAlias(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to create root-owned symlinks")
	}
	// Unique root-level fixture: fixed paths can collide with leftovers or
	// concurrent runs, so each run gets a fresh directory.
	base, err := os.MkdirTemp("/", "devtether-rel-alias-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	real := filepath.Join(base, "real")
	//nolint:gosec // G301: root-owned trusted-system fixture directory
	if mkErr := os.Mkdir(real, 0755); mkErr != nil {
		t.Fatal(mkErr)
	}

	// macOS form: relative target anchored to the link's containing dir.
	alias := filepath.Join(base, "alias")
	if lnErr := os.Symlink("real", alias); lnErr != nil {
		t.Fatal(lnErr)
	}
	fd, _, _, err := secureOpenDir(alias, 0)
	if err != nil {
		t.Fatalf("secureOpenDir on root-owned relative alias = %v, want nil", err)
	}
	_ = unix.Close(fd)

	// A ".." escape into foreign-owned territory: the resolved path is
	// re-walked from the root, so the foreign-owned component is rejected by
	// the normal component policy rather than smuggled through the alias.
	foreign, err := os.MkdirTemp("/", "devtether-rel-foreign-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(foreign) })
	if chErr := os.Chown(foreign, 1000, 1000); chErr != nil {
		t.Skipf("cannot chown fixture to a foreign uid: %v", chErr)
	}
	escape := filepath.Join(base, "escape")
	if lnErr := os.Symlink(filepath.Join("..", filepath.Base(foreign)), escape); lnErr != nil {
		t.Fatal(lnErr)
	}
	fd, _, _, err = secureOpenDir(escape, 0)
	if err == nil {
		_ = unix.Close(fd)
		t.Fatal("secureOpenDir followed a relative alias into foreign-owned territory")
	}
}

// TestForeignOwnedLeafRejectedReal is root-gated: it exercises REAL filesystem
// ownership (chown to a different UID) through the production resolver, unlike
// the synthetic-UID policy tests above. It skips cleanly when unprivileged.
func TestForeignOwnedLeafRejectedReal(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to chown the leaf to a different UID")
	}
	uid := os.Getuid() // 0: the production resolver identity under test.
	foreign := 1000
	if foreign == uid {
		foreign = 65534 // nobody
	}

	// Isolated XDG base owned by the process UID with exact 0700.
	base, err := os.MkdirTemp("", "devtether-foreign-owner")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	//nolint:gosec // Test fixture: XDG_RUNTIME_DIR must be exactly 0700.
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(base, "devtether")
	//nolint:gosec // Test fixture.
	if err := os.Mkdir(leaf, 0700); err != nil {
		t.Fatal(err)
	}
	// Genuinely assign the leaf to a different UID.
	if err := os.Chown(leaf, foreign, foreign); err != nil {
		t.Skipf("cannot chown leaf to uid %d: %v", foreign, err)
	}

	t.Setenv("XDG_RUNTIME_DIR", base)

	// The production resolver uses os.Getuid() (0 here); the leaf is owned by
	// `foreign`, so it must be rejected and no usable path returned.
	if dir, err := resolveRuntimeDir(false); err == nil || dir != "" {
		t.Fatalf("resolveRuntimeDir(false) = (%q, %v), want rejection and empty path", dir, err)
	}
	if path, err := SocketPath(); err == nil || path != "" {
		t.Fatalf("SocketPath() = (%q, %v), want error and no usable path", path, err)
	}
}

// TestSymlinkToFileLeafRejected verifies a leaf symlinked to a non-directory is
// refused explicitly as a symlink, not misreported as an ordinary
// non-directory. On Linux O_DIRECTORY|O_NOFOLLOW reports ENOTDIR here too (the
// ELOOP branch is the POSIX/BSD behavior and is not reachable on Linux), so the
// Fstatat(AT_SYMLINK_NOFOLLOW) disambiguation is what this test exercises.
func TestSymlinkToFileLeafRejected(t *testing.T) {
	xdg := xdgBase(t)
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	target := filepath.Join(t.TempDir(), "regular-file")
	//nolint:gosec // Test fixture.
	if err := os.WriteFile(target, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(xdg, "devtether")); err != nil {
		t.Skipf("cannot create symlink on this platform: %v", err)
	}

	if _, err := RuntimeDir(); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("RuntimeDir() on symlink-to-file leaf = %v, want explicit symlink refusal", err)
	}
}

// TestRootDaemonMayBeRunningNeverConnects verifies the heuristic stays
// best-effort and side-effect free in an environment without root sockets.
func TestRootDaemonMayBeRunningNeverConnects(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root trivially reports false")
	}
	// The heuristic inspects host-global candidate paths, so the suite is
	// only hermetic when none of them exist or are inaccessible (e.g. a
	// leftover root-owned /tmp/devtether-0 from an earlier sudo run).
	for _, p := range []string{
		"/run/user/0/devtether/devtether.sock",
		filepath.Join(os.TempDir(), "devtether-0", "devtether.sock"),
		filepath.Join("/tmp", "devtether-0", "devtether.sock"),
	} {
		if _, err := os.Lstat(p); err == nil || errors.Is(err, os.ErrPermission) {
			t.Skipf("root daemon candidate %s present or inaccessible; host not hermetic", p)
		}
	}
	// No candidate should exist in the test environment; the function must
	// return false without creating or connecting to anything.
	if RootDaemonMayBeRunning() {
		t.Error("RootDaemonMayBeRunning() = true with no root daemon candidates present")
	}
}
