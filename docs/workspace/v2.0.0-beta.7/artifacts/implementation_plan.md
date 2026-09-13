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

### Subphase 1.1: `settings:` Configuration Block
The YAML config currently has no namespace for engine-level settings. All daemon, logging, and runtime knobs need a home that won't pollute the `routes:` namespace.
- **Step 1:** Add a `Settings` struct to `internal/config/config.go` with fields: `Daemon bool`, `Verbose bool`, `LogLevel string`, `LogPath string`.
- **Step 2:** Add `Settings SettingsConfig yaml:"settings,omitempty"` to the top-level `Config` struct.
- **Step 3:** Wire defaults in `applyDefaults()` (e.g., `LogLevel` defaults to `"info"`, `LogPath` defaults to `"./.logs"`).
- **Step 4:** Add validation in `validate()` (valid log levels, writable log path).
- **Step 5:** Add test cases to `config_test.go`.

### Subphase 1.2: Detached Daemon Mode (`-d`)
`devtether up` currently blocks the terminal. Developers need a way to run it in the background.
- **Step 1:** Add a `-d` / `--detach` flag to the `up` command in `internal/cli/up.go`.
- **Step 2:** When `-d` is set (or `settings.daemon: true` in config), use `exec.Command(os.Args[0], "up", "--config", configPath)` with `SysProcAttr{Setsid: true}` to spawn a detached child, then exit the parent.
- **Step 3:** The child process must detect it is the detached child (e.g., via an internal `--_forked` flag) and proceed with normal `runUp` logic.
- **Step 4:** Write the child PID to a predictable location for `devtether status` to read.

### Subphase 1.3: Log Routing & Storage
Detached mode is useless without persistent logs.
- **Step 1:** When running detached, redirect `slog` output to a file inside the configured `settings.log_path` directory (default: `./.logs/devtether.log` relative to the config file's directory).
- **Step 2:** Ensure the log directory is created with `0700` permissions.
- **Step 3:** When running as an OS-native service (future `service install`), detect the init system and route to standard OS paths (`journalctl` / `/var/log/devtether/`).

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
