# Deferred Audit Findings (Service Module)

## 10. Pre-Phase 3 Readiness Audit

**Date:** 09/17/2026
**Auditor:** `@ivintitus (via Antigravity)`

A full pre-implementation audit of the codebase, documentation, and dependency state was performed before beginning Phase 3 (System Integration & Onboarding). Three independent research passes were conducted: codebase inspection, documentation consistency, and test/dependency readiness.

### Scope of Review
- `internal/cli/init.go` (current init implementation)
- `internal/cli/root.go` (global flags and config path resolution)
- `internal/cli/cli_test.go` (existing init tests)
- `internal/config/config.go` (schema, validation, defaults)
- `internal/daemon/lifecycle.go` (RuntimeDir, path resolution)
- `docs/architecture.md`, `README.md`, `docs/engineering-standards.md`
- `docs/adr/003-security-model.md`, `docs/adr/006-platform-support-and-cgo-policy.md`
- `go.mod` (dependency state)

### Finding P3-1: Idempotency Exit Code Mismatch
- **Severity:** Medium
- **Location:** `internal/cli/init.go:40-42`
- **Issue:** Current `init.go` returns `fmt.Errorf` (exit code 1) when the config file already exists. The implementation plan (Subphase 3.1 Step 2) specifies "print and exit cleanly" (exit code 0). This is a behavioral change: scripts checking `$?` would see different results.
- **Resolution Required:** Decide whether to keep exit 1 (safer for scripts) or switch to exit 0 with a skip message (friendlier for humans). Recommend keeping exit 1 but improving the message format to match the user's requested UX: `"devtether.yaml already exists. Skipping initialization. (rerun with --force to overwrite)"`.

### Finding P3-2: Missing Non-Interactive Flags for `service install`
- **Severity:** Medium
- **Location:** Implementation plan Subphase 3.2 Step 3
- **Issue:** The plan describes an interactive privilege elevation prompt for `devtether service install`, but defines no explicit flags (`--user`, `--system`) for non-TTY environments (CI/CD, provisioning scripts). This is the same TTY blindspot that Subphase 3.1 addresses for `init`.
- **Resolution Required:** Add `--user` and `--system` flags. If neither is provided AND stdin is a TTY, prompt interactively. If not a TTY and no flag is given, default to `--user`.

### Finding P3-3: Service Logic Placement Ambiguity
- **Severity:** Low
- **Location:** Implementation plan Subphase 3.2 Step 1
- **Issue:** The plan places service generation logic in `internal/daemon/service.go`. Per `engineering-standards.md` (lines 63-76), `internal/daemon` owns "Unix socket IPC, CLI ↔ daemon communication". Service unit generation is a distinct operational concern.
- **Resolution:** Acceptable for now per ponytail (YAGNI). Creating a separate `internal/service/` package for a single file would be overengineering. Keep it in `internal/daemon/service.go` but ensure it has zero coupling to socket/flock/PID lifecycle code.

### Finding P3-4: Service Activation Gap
- **Severity:** Medium
- **Location:** Implementation plan Subphase 3.2 Steps 4-5
- **Issue:** The plan specifies "generator and remover functions" but does not clarify whether `service install` also activates the service (e.g., `systemctl daemon-reload && systemctl enable --now devtether`) or merely writes the unit file. Similarly, `service uninstall` does not specify whether it stops/disables before removing.
- **Resolution Required:** The plan must explicitly state: `install` = write file + reload + enable. `uninstall` = stop + disable + remove file + reload.

### Finding P3-5: `--setcap` Flag Semantics Unclear
- **Severity:** Low
- **Location:** Implementation plan Subphase 3.1 Step 3
- **Issue:** The plan lists `--setcap` as a flag for `devtether init`. But `init` generates a YAML config file — it does not bind ports. Should `--setcap` trigger an actual `sudo setcap cap_net_bind_service=+ep` call on the binary, or merely control whether the generated config comments mention setcap? The wizard prompt (Step 4.2) says "Prompt to configure setcap for port 80 natively", implying the former.
- **Resolution Required:** Clarify: the wizard should offer to *apply* setcap (run the shell command) as a convenience, not just document it.

### Finding P3-6: Documentation Gaps (architecture.md and README.md)
- **Severity:** Medium
- **Location:** `architecture.md`, `README.md`
- **Issue:**
  - `architecture.md`: Zero mentions of `devtether init`, `devtether service install`, or `devtether service uninstall`. The architecture diagram and lifecycle sections do not reference OS service integration.
  - `README.md`: `devtether init` is described only as a static template creator (line 144). `devtether service` is completely absent from the Commands table.
- **Resolution:** Phase 3 must include explicit documentation steps to update both files after implementation.

### Finding P3-7: Existing Test Coverage
- **Severity:** Info
- **Location:** `cli_test.go:13-42`
- **Issue:** `TestInitCmd` exists and tests basic creation + idempotency guard. It will need updating to cover the new interactive flow, TTY detection, and flag-driven behavior. Per `engineering-standards.md` (lines 143-144), the wizard logic should accept `io.Reader`/`io.Writer` parameters for testability.

### Finding P3-8: Dependency State (No New Dependencies Required)
- **Severity:** Info
- **Location:** `go.mod:9`
- **Issue:** `golang.org/x/term v0.46.0` is already a direct dependency. No new `go get` required for TTY detection. Per `engineering-standards.md` (lines 187-194), no third-party prompt libraries (survey, bubbletea, promptui) are permitted. All prompts must use `bufio.NewReader` + `fmt.Fscanln`.

### Open Items
| ID | Finding | Status | Resolution |
|----|---------|--------|------------|
| P3-1 | Idempotency exit code | ⚠️ Decision needed | Keep exit 1 with improved message |
| P3-2 | Missing `--user`/`--system` flags | ⚠️ Decision needed | Add flags, default `--user` in non-TTY |
| P3-3 | Service logic placement | ✅ Acceptable | Keep in `internal/daemon/service.go` |
| P3-4 | Service activation gap | ⚠️ Decision needed | `install` = write+reload+enable |
| P3-5 | `--setcap` semantics | ⚠️ Decision needed | Apply setcap via shell command |
| P3-6 | Docs gaps | ⚠️ Planned | Update as part of Phase 3 |
| P3-7 | Test coverage | ✅ Info | Update existing tests |
| P3-8 | Dependencies | ✅ No action | Already present |

---

## 11. Post-Implementation Audit (Subphases 3.1 & 3.2)

**Date:** 09/17/2026
**Auditor:** `@ivintitus (via Antigravity / devtether-audit)`

Following the implementation of the `init` interactive wizard (3.1) and OS service installer (3.2), an independent verification pass was conducted according to the `devtether-audit` standard. While `make test` passes all 8 gates (including race and lint checks), the audit surfaced architectural defects that compromise testing integrity and UX edge cases.

### Finding P3-9: TTY Detection Testability Bypass
- **Severity:** High (Breaks `engineering-standards.md` testing rules)
- **Location:**
  - `internal/cli/init.go:90` (`gatherInitAnswers`)
  - `internal/cli/service.go:150` (`resolveServiceTier`)
- **Failure Mechanism:** Both functions accept an `io.Reader stdin` parameter (for dependency injection during tests), but they check TTY status using a hardcoded `os.Stdin`: `term.IsTerminal(int(os.Stdin.Fd()))`.
- **Impact:** The `io.Reader` abstraction is bypassed. If a developer runs `go test` from an actual interactive terminal, `term.IsTerminal` returns `true`, and the test will hang indefinitely waiting for input from the empty test buffer.

### Finding P3-10: Uninstall Service State Leak
- **Severity:** High (Broken State Machine)
- **Location:** `internal/daemon/service.go:148-150` (`UninstallService`)
- **Failure Mechanism:** The function checks if the service file exists *before* deactivating the service. If the file is missing (e.g., deleted manually), it immediately returns an error and skips `deactivateService(unitPath, system)`.
- **Impact:** The service remains permanently running and enabled in `systemd`/`launchd`. The user is stuck with a ghost process they must manually kill using `systemctl stop`. Deactivation must occur independently of file existence.

### Finding P3-11: Blind `systemd` Execution on Linux
- **Severity:** Medium
- **Location:** `internal/daemon/service.go:242` (`runSystemctl`)
- **Failure Mechanism:** On Linux, `service install` blindly assumes `systemd` is the init system and executes `systemctl`.
- **Impact:** Cryptic failure on Alpine Linux (OpenRC), WSL 1, or standard Docker containers. The CLI layer should explicitly verify systemd presence (e.g., checking if `/run/systemd/system` exists) and return a clean, actionable error before attempting installation.

### Finding P3-12: `launchd` Start vs. Load Behavior
- **Severity:** Low
- **Location:** `internal/daemon/service.go:250` (`activateLaunchd`)
- **Failure Mechanism:** `launchctl load -w` fails if the service is already loaded.
- **Impact:** Running `devtether service install` twice in a row on macOS fails, violating idempotency. `activateLaunchd` should either ignore "already loaded" errors or explicitly unload before loading.

### Finding P3-13: Systemd/Launchd Environment Stripping
- **Severity:** High (Crash Risk)
- **Location:** `internal/daemon/service.go:36` (systemdTemplate), `56` (launchdTemplate)
- **Failure Mechanism:** System service managers strip environment variables (like `$HOME`, `$USER`, `$PATH`). If the user generates a `devtether.yaml` with relative paths that depend on the user's `$HOME` (or if `devtether up` attempts to use `os.UserHomeDir()` and `$HOME` is unset), the daemon will crash or write to the root filesystem `/`.
- **Impact:** System-wide daemon installations are prone to crash on startup if they rely on implicit user environment variables.

### Finding P3-14: Missing Service Working Directory
- **Severity:** High
- **Location:** `internal/daemon/service.go` (Service templates)
- **Failure Mechanism:** Neither the `systemd` nor `launchd` templates specify a working directory. By default, they will execute `devtether` with `/` as the working directory. Any relative paths in `devtether.yaml` (e.g., `log_path: "./.logs"`) will attempt to resolve to `/.logs`, resulting in a fatal permission denied error.
- **Impact:** The daemon will fail to initialize logging or load relative assets. The templates must explicitly define `WorkingDirectory={{.WorkingDir}}` (systemd) and `<key>WorkingDirectory</key>` (launchd).

### Finding P3-15: `launchd` Log Directory Initialization
- **Severity:** Medium
- **Location:** `internal/daemon/service.go:66-68` (launchdTemplate)
- **Failure Mechanism:** `launchd` requires the parent directories of `StandardOutPath` and `StandardErrorPath` to already exist. If we specify `~/.logs/devtether.log` and `~/.logs` does not exist, `launchctl load` will silently fail to start the process.
- **Impact:** Broken logging and silent failures on macOS. The CLI installer must explicitly `os.MkdirAll` the log directory before loading the plist.

### Finding P3-16: `setcap` on Ephemeral Binaries
- **Severity:** Low (UX Trap)
- **Location:** `internal/cli/init.go:160` (`applySetcap`)
- **Failure Mechanism:** `applySetcap` relies on `os.Executable()`. If a user runs the wizard via `go run main.go init --setcap` or from a `/tmp/go-build*` directory, setcap is applied to an ephemeral binary that is immediately deleted or lost.
- **Impact:** Confusing behavior. The wizard should warn the user if `os.Executable()` resolves to `/tmp`.

### Finding P3-17: Systemd Path Whitespace Parsing Failure
- **Severity:** Critical
- **Location:** `internal/daemon/service.go:36` (systemdTemplate)
- **Failure Mechanism:** The `systemdTemplate` uses an unquoted `ExecStart`: `ExecStart={{.BinaryPath}} up --config {{.ConfigPath}}`. Systemd splits `ExecStart` by spaces. If `BinaryPath` or `ConfigPath` contain spaces (e.g., `/media/ivintitus/Data/My Projects/...`), systemd will fail to parse the path and immediately crash.
- **Impact:** Service installation is 100% broken for users running from paths with spaces. Paths must be quoted: `ExecStart="{{.BinaryPath}}" up --config "{{.ConfigPath}}"`.

### Finding P3-18: Privilege Inversion on User Services
- **Severity:** High (Crash & Incorrect State)
- **Location:** `internal/cli/service.go` (`runServiceInstall`, `runServiceUninstall`)
- **Failure Mechanism:** If a user executes `sudo devtether service install --user`, two critical failures occur:
  1. `os.UserHomeDir()` may resolve to `/root`, placing the user service in root's config directory.
  2. The CLI executes `systemctl --user enable` as root. Since root rarely has a user D-Bus session running, `systemctl` crashes with `Failed to connect to bus`.
- **Impact:** Running `--user` commands with `sudo` creates orphaned files and throws cryptic D-Bus errors. The CLI must explicitly reject `--user` when executed with root privileges.

### Finding P3-19: Unquoted Shell Fallback Command
- **Severity:** Low
- **Location:** `internal/cli/init.go:70` (`runInit`)
- **Failure Mechanism:** If `setcap` fails, the CLI prints: `sudo setcap ... $(which devtether)`. If the path contains spaces, the user copying this directly into their shell will experience a secondary failure due to word splitting.
- **Impact:** Poor UX. The hint should explicitly quote the path: `sudo setcap ... "$(which devtether)"` or use the resolved `%q` formatted string.
