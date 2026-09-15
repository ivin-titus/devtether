# v2.0.0-beta.7 System Integration — Implementation Plan

## Pre-Flight Checklist
- [x] Codebase audited for related logic and edge cases.
- [x] Changes verified against `docs/engineering-standards.md`.
- [x] Architectural decisions comply with `docs/adr/`.

This release elevates Engine 1 (Core Networking) from a foreground-only CLI tool into a system-level utility that can run detached, persist logs, report its own health, and diagnose its environment. Scope is strictly Engine 1. Engine 2 (Orchestrator) and the Web GUI are deferred to future releases per the Anti-Slop Architecture mandate.

> [!IMPORTANT]
> All implementations must adhere to `docs/engineering-standards.md`.
> Defaults must live centrally in `internal/config`. No CGo. No speculative abstractions.

---

## Phase 1: Daemon Infrastructure

### Subphase 1.1: `settings:` Configuration Block ✅
The YAML config currently has no namespace for engine-level settings. All daemon, logging, and runtime knobs need a home that won't pollute the `routes:` namespace.
- **Step 1:** Add a `Settings` struct to `internal/config/config.go` with fields: `Daemon bool`, `Verbose bool`, `LogPath string`.
- **Step 2:** Add `Settings SettingsConfig yaml:"settings,omitempty"` to the top-level `Config` struct.
- **Step 3:** Add test cases to `config_test.go`.

> **Resolution (09/15/2026):**
> - Shipped `SettingsConfig` with 3 fields: `daemon`, `verbose`, `log_path`.
> - `LogLevel` was dropped (YAGNI) — the binary `verbose` toggle already covers the codebase's needs. See discussion_space.md Pre-Mortem Anomaly #1.
> - `settings.dns.fallback_port` was dropped — the top-level `dns:` key already owns DNS config. See Pre-Mortem Anomaly #2.
> - No `applyDefaults()` changes needed — Go zero-values are the correct defaults (`daemon=false`, `verbose=false`, `log_path=""`).
> - No `validate()` changes needed — the fields are validated at use-time (log dir creation, daemon spawn). Ponytail: skip premature validation.
> - 2 new table-driven test cases added (`settings config parsed`, `settings defaults when absent`).
> - **Files:** `internal/config/config.go`, `internal/config/config_test.go`.

### Subphase 1.2: Detached Daemon Mode (`-d`) ✅
`devtether up` currently blocks the terminal. Developers need a way to run it in the background.
- **Step 1:** Add a `-d` / `--detach` flag to the `up` command in `internal/cli/up.go`.
- **Step 2:** When `-d` is set (or `settings.daemon: true` in config), use `exec.CommandContext` with `SysProcAttr{Setsid: true}` to spawn a detached child, then exit the parent.
- **Step 3:** The child process detects it is the forked child via `DEVTETHER_FORKED=1` env var and proceeds with normal `runUp` logic.

> **Resolution (09/15/2026):**
> - Shipped `-d`/`--detach` flag on the `up` command.
> - Config-driven via `settings.daemon: true`. Uses `cmd.Flags().Changed("detach")` to avoid overriding explicit CLI flags.
> - Used `DEVTETHER_FORKED=1` env var instead of a hidden `--_forked` flag — cleaner, no CLI surface pollution.
> - PID file was dropped (YAGNI) — the IPC socket already serves as the liveness indicator via `daemon.CheckRunning`. PID is printed to stdout on spawn. `devtether status` (Phase 2) will expose PID via `/status` endpoint.
> - Parent runs `daemon.CheckRunning` pre-flight for immediate user feedback before forking.
> - `settings.verbose` passthrough: if CLI `--verbose` wasn't explicitly set but config has `verbose: true`, re-initializes the logger in debug mode.
> - **Files:** `internal/cli/up.go`.

### Subphase 1.3: Log Routing & Storage ✅
Detached mode is useless without persistent logs.
- **Step 1:** When running detached, redirect child stdout/stderr to a log file inside the configured `settings.log_path` directory (default: `./.logs/devtether.log` relative to the config file's directory).
- **Step 2:** Ensure the log directory is created with `0700` permissions.

> **Resolution (09/15/2026):**
> - Implemented in the `daemonize()` function in `internal/cli/up.go`.
> - Log dir: `settings.log_path` if set, else `./.logs/` resolved relative to the config file's absolute path.
> - Log dir created with `0700` (ADR-003). Log file `0644` (readable for debugging).
> - Child's `os.Stdout` and `os.Stderr` are both redirected to the log file. Since the logger and `fmt.Printf` both write to stdout, all output is captured. The existing `term.IsTerminal` check automatically strips ANSI codes when stdout is a file.
> - OS-native service log routing (journalctl, `/var/log/devtether/`) is deferred to Subphase 3.2 (service installer) — it cannot be implemented without the init system detection logic.
> - **Files:** `internal/cli/up.go`.

### Subphase 1.4: Documentation & Cleanup (Tech Debt) ✅
Phase 1 shipped code but left documentation surfaces stale. These are non-architectural fixes from audit findings 1, 4–7, and 10.
- **Step 1:** Update root help cheat-sheet in `internal/cli/root.go` — add `devtether up -d` line. *(Audit Finding 4)*
- **Step 2:** Update `up` command Long description in `internal/cli/up.go` — mention `-d` background mode and log path. *(Audit Finding 5)*
- **Step 3:** Update `README.md` Commands section — add `devtether up -d` example. *(Audit Finding 6)*
- **Step 4:** Update `README.md` download example — replace stale `beta.2` with `<VERSION>` placeholder. *(Audit Finding 7)*
- **Step 5:** Add commented `settings:` block to `init.go`'s default config template so users discover `daemon`, `verbose`, `log_path`. *(Audit Finding 8, partial)*
- **Step 6:** Delete `internal/cli/root_out.go` if confirmed unused. *(Audit Finding 10)*
- **Step 7:** Remove stale `DefaultReadTimeout`, `DefaultWriteTimeout` constants, their `applyDefaults()` wiring, and their `validateProxy()` checks from `internal/config/config.go`. Update `config_test.go` accordingly. *(Audit Finding 1)*

> **Resolution (09/15/2026):**
> - All 7 steps completed. `make test` passes (8/8 checks, 0 lint issues).
> - **Files modified:** `internal/cli/root.go`, `internal/cli/up.go`, `internal/cli/init.go`, `README.md`, `internal/config/config.go`, `internal/config/config_test.go`.
> - **Files deleted:** `internal/cli/root_out.go` (dead placeholder, confirmed via `git log` and `grep`).
> - Finding 8 is partially resolved — `settings:` block added to init template, but help text update deferred to Phase 3.1 wizard.
> - Finding 9 (unexported godoc) remains open — no action needed per engineering standards.
---

## Phase 2: CLI Observability & Diagnostics

### Subphase 2.1: `devtether status` (Passive Reporting)
The CLI needs a way to query the running daemon's state without modifying anything.
- **Step 1:** Add a `GET /status` handler to the IPC daemon (`internal/daemon/daemon.go`). Return JSON: uptime, PID, active route count, `runtime.MemStats.Alloc`.
- **Step 2:** Create `internal/cli/status.go`. Dial the Unix socket, fetch `/status`, render a `text/tabwriter` table.
- **Step 3:** If the daemon is not running, print a clear message and exit 1.

### Subphase 2.2: `devtether doctor` (Active Diagnostics)
A proactive environment scanner that does NOT require a running daemon.
- **Step 1:** Create `internal/cli/doctor.go`.
- **Step 2:** Implement checks (each prints ✓ or ✗ with an explanation):
  - Port 80 availability (detect XAMPP/Apache/Nginx conflicts).
  - `setcap` status on the DevTether binary.
  - DNS resolution test (`dig` / `nslookup` equivalent for `.localhost`).
  - Stale `.sock` file detection.
  - `devtether.yaml` syntax validation (reuse `config.LoadConfig`).
- **Step 3:** Add test cases for individual check functions.

### Subphase 2.3: `devtether logs` (Daemon Log Tailing)
- **Step 1:** Create `internal/cli/logs.go`. Read the log path from the config (or default).
- **Step 2:** Implement a `tail -f` style follower using `os.Open` + periodic `Read` (no external dependencies).
- **Step 3:** Support `--lines N` flag for initial backlog.

---

## Phase 3: System Integration & Onboarding

### Subphase 3.1: `devtether init` Interactive Wizard
The current `init` command dumps a static template. It should guide new users through a minimal, secure setup.
- **Step 1:** Replace the static `defaultConfig` in `internal/cli/init.go` with an interactive prompt flow using `fmt.Scan` / `bufio.Scanner` (no external TUI libraries — ponytail rule).
- **Step 2:** Prompts:
  1. *System daemon or standalone?* → sets `settings.daemon`.
  2. *Detect OS DNS resolver* → print guidance for `systemd-resolved` or `/etc/hosts`.
  3. *Apply `setcap` for port 80?* → explain consequences, default N (least privilege).
- **Step 3:** Generate and validate the config (reuse `config.LoadConfig` for `nginx -t` style validation).
- **Step 4:** Print the config path and next steps (`devtether up`).

### Subphase 3.2: Cross-Platform Service Installer
- **Step 1:** Create `internal/daemon/service.go`.
- **Step 2:** Implement generator functions for `systemd` (Linux), `launchd` (macOS), `OpenRC` (Alpine/Arch).
- **Step 3:** Add `devtether service install` and `devtether service uninstall` CLI commands.
- **Step 4:** Ensure idempotent behavior and safe privilege escalation.

---

## Deferred to Future Release

> [!NOTE]
> The following items are documented in the discussion space but explicitly deferred per the Anti-Slop Architecture mandate and the "Engine 1 Focus" sprint direction.

- **IPC REST API Refactor** (`GET /api/config`, `POST /api/config/routes`, `DELETE /api/config/routes/{domain}`).
- **Stateless Web GUI** (`devtether.localhost`, Vanilla JS, `//go:embed`).
- **Webhook Traffic Inspection** (`sync.Pool` buffering, SSE endpoint `/api/stream`).

These will be scoped into a future release once the daemon infrastructure is stable and battle-tested.

---

## Verification Plan

### Automated Tests
```bash
make test
```
Each new CLI command and config change must ship with table-driven tests.

### Manual Verification
- `devtether up -d` starts a background process, writes PID, logs to `.logs/`.
- `devtether status` reports the daemon's state.
- `devtether doctor` identifies port conflicts and missing `setcap`.
- `devtether init` walks through interactive setup.
- `devtether service install` generates the correct daemon file for the host OS.
