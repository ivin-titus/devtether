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

### Subphase 2.1: `devtether status` & `devtether down` (IPC Client Commands)
The CLI needs a way to query and control the running daemon via the Unix socket.
- **Step 1:** Add a `GET /status` handler to the IPC daemon (`internal/daemon/daemon.go`). Return JSON: uptime, PID, active route count, `runtime.MemStats.Alloc`.
- **Step 2:** Create `internal/cli/status.go`. Dial the Unix socket, fetch `/status`, render a `text/tabwriter` table.
- **Step 3:** If the daemon is not running, print a clear message and exit 1.
- **Step 4:** Add a `POST /shutdown` handler to the IPC daemon. Trigger the root context's `cancel()` to initiate the existing graceful shutdown path.
- **Step 5:** Create `internal/cli/down.go`. Dial the Unix socket, send the shutdown request, print confirmation.
- **Step 6:** If the daemon is not running, print a clear message and exit 0 (idempotent — `down` on a stopped daemon is a no-op, not an error).

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

### Subphase 2.4: Phase 2 Remediation (Audit Findings)
Based on the Phase 2 audit, several fixes are required before closing the implementation phase.
- **Step 1:** Fix Finding 11: In `internal/cli/up.go`, add a check inside the `daemonize()` guard to reject daemonization if `os.Getuid() == 0`, logging a clear error that `sudo -d` is not supported.
- **Step 2:** Fix Finding 12: In `internal/cli/status.go`, change the tabwriter label from "Memory" to "Heap" to clarify that the metric is `MemStats.Alloc`.
- **Step 3:** Fix Finding 13: In `internal/cli/doctor.go`, introduce a `printWarn` helper and a `warnings` counter. Change the stale socket check and the port 80 check to use `printWarn` instead of `printCheck(false, ...)`.
- **Step 4:** Fix Finding 16: In `internal/cli/doctor.go`, guard the `checkSetcap` call with `if runtime.GOOS == "linux" { checkSetcap(...) }` and add a macOS fallback. Also update the messaging to not recommend `sudo`.
- **Step 5:** Fix Finding 14 & 15: In `internal/cli/logs.go`, add a `-f`/`--follow` boolean flag. If `-f` is true, enter follow mode directly. Update the `Long` description to clarify "last N lines" and document `-f`.
- **Step 6:** Run `make test` and verify CLI UX.

### Subphase 2.5: Documentation Updates (Phase 2)
All four new commands and two new IPC endpoints require corresponding documentation updates across every surface. Fix in a single focused commit after all Phase 2 code is verified.

**`internal/cli/root.go` — Root help cheat-sheet:**
- **Step 1:** Add `devtether status`, `devtether down`, `devtether doctor`, `devtether logs` to the `Long` description cheat-sheet. *(Same pattern as Subphase 1.4 Step 1.)*

**`README.md` — Commands section:**
- **Step 2:** Add all four new commands to the Commands section with one-line descriptions.

**Cobra `Long` descriptions — each new command file:**
- **Step 3:** `internal/cli/status.go` Long: describe output columns (uptime, PID, routes, Heap), and note exit 1 if daemon not running.
- **Step 4:** `internal/cli/down.go` Long: describe graceful shutdown behaviour and idempotent exit 0.
- **Step 5:** `internal/cli/doctor.go` Long: list the checks performed and describe ✓/⚠/✗ output format, mentioning that ⚠ is for expected system conditions (like port 80 restrictions without setcap).
- **Step 6:** `internal/cli/logs.go` Long: describe tail behaviour, `-f`/`--follow` flag, `--lines N` (explicitly stating "last N lines"), and the foreground logging limitation.
- **Step 6b:** `internal/cli/up.go` Long: Add a note explaining that daemon mode (`-d`) is not supported when running as root/sudo.

**`internal/daemon/daemon.go` — IPC handler godoc:**
- **Step 7:** Add godoc comment to `handleStatus` explaining the JSON response shape (`uptime`, `pid`, `routes`, `mem_alloc`).
- **Step 8:** Add godoc comment to `handleShutdown` explaining that it triggers graceful context cancellation.

**`docs/architecture.md` — IPC API surface:**
- **Step 9:** Update the IPC API section to list all registered endpoints: `GET /routes`, `GET /status`, `POST /shutdown`. *(Keeps the architecture doc as the single source of truth for the IPC contract.)*

### Subphase 2.6: Daemon Lifecycle Remediation (UX Bugs)
Based on post-Phase 2 verification, several critical daemon lifecycle bugs must be fixed before Phase 3. **All fixes use event-driven kernel primitives — no polling.** See `premortem_2.6_2.7.md` for the full design rationale.

> [!IMPORTANT]
> Subphase 2.6 is **blocked by Subphase 2.7** — the `logs -f` fix depends on flock, and the shutdown fix depends on IPC server hardening (WriteTimeout).

**Issue 1: The `logs -f` Zombie Trap (F18)**
- **Step 1:** In `internal/daemon/`, add a `WaitForExit(ctx context.Context) error` helper that opens the lock file read-only and calls `syscall.Flock(fd, LOCK_EX)` (blocking). This blocks in the kernel until the daemon releases the lock (any exit, including SIGKILL). Returns `nil` when the daemon exits, or `ctx.Err()` if the context is cancelled.
- **Step 2:** In `internal/cli/logs.go`, modify `tailFollow` to accept a `context.Context`. Launch `daemon.WaitForExit(ctx)` in a background goroutine. When it returns, cancel the tail context and print `DevTether daemon shut down.`

**Issue 2 & 3: The Shutdown Race & Ghost TCP Bind (F19, F20)**
- **Step 3:** In `internal/daemon/daemon.go`, rewrite `handleShutdown`:
  1. Encode and flush the JSON response using `http.Flusher.Flush()`.
  2. Call `s.cancelFunc()` immediately (remove the `time.Sleep(100ms)`).
  3. Block on `<-r.Context().Done()` to keep the HTTP connection alive during the entire graceful shutdown (including proxy's 5s drain).
  The handler only returns after the server's shutdown sequence fires `r.Context()`, which means the entire daemon (IPC, proxy, DNS) has finished draining.
- **Step 4:** In `internal/daemon/client.go`, add a `ShutdownAndWait() (*http.Response, error)` method that sends the `POST /shutdown` but uses a longer client timeout (15s) to accommodate the full drain window.
- **Step 5:** In `internal/cli/down.go`, modify `runDown` to call `client.ShutdownAndWait()`, then drain the response body with `io.Copy(io.Discard, resp.Body)`. When `io.Copy` returns (EOF from server closing the connection), the daemon has fully stopped. Print `DevTether daemon stopped.`

**Issue 4: Silent Port Fallback in Daemon Mode (F21)**
- **Step 6:** In `internal/cli/up.go`, modify `daemonize()` to create an `os.Pipe()` and pass the write-end to the child via `cmd.ExtraFiles` (maps to fd 3 in the child).
- **Step 7:** In `runUp()`, after all servers are successfully bound (DNS, proxy, IPC), detect fd 3 via `os.NewFile(3, "readiness-pipe")` and write a JSON payload containing the bound port, then close the pipe.
- **Step 8:** In `daemonize()`, the parent blocks on `readEnd.Read()` with a `SetReadDeadline` of 5 seconds. On success, decode the JSON payload and print `DevTether daemon started (PID X, port Y)`. On EOF (child crashed), print the error and exit non-zero. On timeout, kill the child and report failure.

### Subphase 2.7: Daemon Lifecycle Hardening (Architectural)
Based on the adversarial security audit (Findings A1–A7), the daemon's instance management and IPC layer require fundamental architectural hardening to meet ADR-003 requirements. This must be implemented before Phase 3.

- **Step 1:** Implement `flock(LOCK_EX|LOCK_NB)` on a dedicated lock file (`$XDG_RUNTIME_DIR/devtether/devtether.lock` or `/tmp/devtether-<uid>/devtether.lock`). This must be acquired *before* any socket operations to serve as the primary instance ownership mechanism.
- **Step 2:** Write an atomic PID file (temp file → fsync → rename) alongside the lock file. Used for diagnostics and as a fallback for `SIGTERM`.
- **Step 3:** Secure the fallback socket path. Change `/tmp/devtether.sock` to `/tmp/devtether-<uid>/devtether.sock`. Ensure the directory is created with `0700` permissions.
- **Step 4:** Implement symlink protection. After creating the socket directory, use `os.Lstat` to verify it is a real directory (not a symlink) and owned by the current UID (`stat.Sys().(*syscall.Stat_t).Uid == os.Getuid()`). Fail closed on mismatch.
- **Step 5:** Implement daemonize readiness verification. **Use the anonymous pipe pattern from Subphase 2.6 Step 6–8.** The readiness pipe is sent after flock + socket bind + server bind are all complete. This replaces any socket-polling alternative.
- **Step 6:** Harden the IPC HTTP server. Set `MaxHeaderBytes: 1<<16`, `ReadTimeout: 10s`, `WriteTimeout: 15s`. The `WriteTimeout` is 15s (not 10s) to accommodate the shutdown connection-hold pattern from Subphase 2.6 Step 3. Use `http.MaxBytesReader` for any endpoints that read a body.
- **Step 7:** Document the `syscall.Umask(0177)` constraint in `daemon.go` to prevent future goroutine race conditions on file creation.

### Subphase 2.8: Phase 2 Final Post-Mortem & UX Cleanups
- **Step 1:** Perform a final post-implementation audit of all Phase 2 changes against standard (`devtether-audit`).
- **Step 2:** Close the minor FD leak in `up.go` (closing the logger FD after successful fork).
- **Step 3:** Enhance the UI silent fallback mechanism from Subphase 2.6/2.7 by parsing the JSON payload returned over the readiness pipe and presenting an OS-aware warning on the CLI if a port fallback occurred.
- **Step 4:** Update `setcap` references universally to use `runtime.GOOS == "linux"` gating, preventing macOS/Windows confusion.

### Subphase 2.9: MacOS & Root Lifecycle Support (Unblocking `sudo -d`)

#### Goal Description
In early Phase 2, `sudo devtether up -d` was blocked because it created a root-owned Unix socket in a world-writable `/tmp` folder, locking the user out of `devtether down`. However, with the new Subphase 2.7 secure runtime directory model (`/tmp/devtether-0` with `0700` permissions), running as a root daemon is now completely safe and isolated.
Removing this restriction is essential for macOS users, who do not have `setcap` and *must* use `sudo` to bind to port 80.

#### Proposed Changes
- **`internal/cli/up.go`**:
  - [MODIFY] Remove the `if os.Getuid() == 0 && detach` hard block.
  - [MODIFY] Update the `upCmd.Long` help text to remove the "strictly blocked" warning. Instead, clearly document the OS-specific path to port 80:
    - *Linux:* Recommend `sudo setcap cap_net_bind_service=+ep` so users don't have to run as root.
    - *macOS:* Recommend running `sudo devtether up -d` to bind port 80.

#### Verification Plan
- **Automated Tests**: Standard `make test`.
- **Manual Verification**: Run `sudo ./devtether up -d` (should succeed). Run `./devtether status` (should say not running, because it looks at UID 1000). Run `sudo ./devtether status` (should connect to UID 0 socket). Run `sudo ./devtether down`.

> [!IMPORTANT]
> **User Review Required**: Does this unblocking strategy align with your expectations for macOS users? Once you approve this plan, I will execute the change and update the artifacts and tracker.

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
