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

## Phase 1: Daemon Infrastructure (Completed)

### Subphase 1.1: `settings:` Configuration Block ✅
Introduced a `settings:` block (`daemon`, `verbose`, `log_path`) in `devtether.yaml` to isolate engine-level knobs from `routes:`.
> **Resolution (09/15):** Shipped with zero-value defaults. Dropped `log_level` and `settings.dns.fallback_port` (YAGNI; see `discussion_space.md`). Added table-driven tests.

### Subphase 1.2: Detached Daemon Mode (`-d`) ✅
Added `-d` / `--detach` flag and `settings.daemon` config to spawn `devtether up` in the background via `SysProcAttr{Setsid: true}` and `DEVTETHER_FORKED=1`.
> **Resolution (09/15):** Shipped. The IPC socket serves as liveness (PID file dropped as YAGNI in Phase 1, restored in Phase 2). Parent pre-flights `daemon.CheckRunning` before exiting.

### Subphase 1.3: Log Routing & Storage ✅
Redirected detached child `stdout`/`stderr` to `settings.log_path` (default `./.logs/devtether.log`).
> **Resolution (09/15):** Shipped. Log dir uses `0700` per ADR-003, log file uses `0644`. OS-native routing deferred.

### Subphase 1.4: Documentation & Cleanup ✅
Resolved audit findings 1, 4-7, and 10 to sync documentation with Phase 1 code.
> **Resolution (09/15):** Updated `root.go`, `up.go`, `init.go`, and `README.md` to document `-d` and `settings`. Deleted unused `root_out.go`. Removed stale timeout config blocks from `config.go`.
---

## Phase 2: CLI Observability & Diagnostics (Completed)

### Subphase 2.1 - 2.3: IPC Client Commands (`status`, `down`, `doctor`, `logs`) ✅
Implemented the CLI toolset for observing and diagnosing the daemon.
> **Resolution:** Shipped `GET /status`, `POST /shutdown` via Unix socket. Implemented `status` (table rendering), `down` (idempotent shutdown), `doctor` (proactive environment scanner), and `logs -f` (tail follower).

### Subphase 2.4 - 2.5: Documentation & Remediation ✅
Addressed audit findings to fix CLI UX and synchronize the README/architecture docs.
> **Resolution:** Added all new commands to `root.go`, `README.md`, and `docs/architecture.md`. Clarified `-d` root restrictions and modified `doctor` to use non-fatal warnings for expected conditions.

### Subphase 2.6 - 2.7: Daemon Lifecycle Hardening (Architectural) ✅
Hardened the daemon's lifecycle management and IPC server to meet ADR-003 security standards, fixing zombie tails and race conditions.
> **Resolution:** Shipped `flock(LOCK_EX|LOCK_NB)` instance ownership on `$XDG_RUNTIME_DIR`. Secured the Unix socket with `0700` permissions and symlink protection. Introduced a readiness pipe (fd 3) for the parent to await complete bind success (or timeout) before exiting. Hardened the HTTP server (`WriteTimeout: 15s`) to keep connections alive during graceful drain.

### Subphase 2.8 - 2.9: MacOS & Root Lifecycle Support ✅
Verified all Phase 2 changes and removed early root-daemon blocks.
> **Resolution:** Removed the `if os.Getuid() == 0` block from `up -d` now that the runtime directory uses secure `0700` isolation. Unblocked macOS users requiring `sudo` to bind port 80.

---

## Phase 3: System Integration & Onboarding (Completed)

### Subphase 3.1: `devtether init` Interactive Wizard ✅
Upgraded the `init` command to an interactive, flag-driven wizard (`--daemon`, `--setcap`, `--force`) using standard `bufio` (no 3rd-party TUIs). Included TTY detection and testable `io.Reader` logic.

### Subphase 3.2: Cross-Platform Service Installer
> **DEFERRED**: OS Service module deferred to future releases (see `.ideas/service_mode`).

### Subphase 3.3: Consolidated Remediation — Audit & Hardening Fixes ✅
Addressed 24 open findings (P3, R4, and SC independent audit findings).
> **Resolution:** 
> - **Init Wizard:** Added TTY fallbacks, setcap bounds checking, and hint quoting.
> - **Daemon Lifecycle:** Synchronized IPC listener binding (SC-5), restored timeout for readiness pipe (SC-1), and fixed PID-based fallback (SC-4).
> - **CLI Core:** Removed `os.Exit(1)` from library code, honoured `NO_COLOR`, and tightened `doctor` validation logic (SC-8).
> - **Network/Proxy:** Bound explicit timeouts to HTTP clients and fixed proxy handler concrete assertion errors (ADR-008 streaming compliance). Enforced trailing dot / lowercase validation strictly.
> - **Verification:** All fixes confirmed with regression tests per `engineering-standards.md`.

### Subphase 3.4: Product & DX Polish / Missing Pieces ✅
Aligned observable behavior with developer expectations per audit findings DX-1 through DX-18.
> **Resolution:**
> - Fixed false-positive "fell back" port warnings (`DX-2`, `DX-3`).
> - Refined `logs` to explicitly differentiate between cross-UID permission errors and stopped daemons (`DX-5`).
> - Enhanced `status` to output the exact `config_path` in use (`DX-7`).
> - Updated `init` to generate a heavily commented, self-documenting template instead of relying on external `examples/` (`DX-1`).
> - Reconciled docs with reality: route changes require restarts, no hot-reload (`DX-8`).

### Subphase 3.5: Documentation Synchronization & Final Consistency ✅
Synchronized all documentation surfaces to reflect Subphase 3.3 and 3.4 code.
> **Resolution:** Updated `docs/architecture.md`, `README.md`, and Cobra `Long` help texts. Verified no ephemeral sprint terminology remains.

---

## Phase 4: Final Polish (DNS Wizard, Edge Cases & DX)

Following manual testing in the sandbox, we identified critical DX improvements to make `devtether init` a true "magic" setup wizard, clean up the help text, and resolve the remaining lifecycle edge cases (QA-1 and QA-2).

### Goal Description
1. **Smart DNS Auto-Configuration (DX-19):** Enhance `devtether init` to automatically detect the host OS and active resolver (`systemd-resolved`, `macOS resolver`, or `dnsmasq`), prompt the user, and configure system-level DNS natively. This simplifies the `README.md` installation guide drastically.
2. **Help Clutter & Duplication (DX-20, DX-22):** Hide the auto-generated `completion` command from Cobra's help output. Remove manually maintained cheat-sheets from `rootCmd.Long` and `initCmd.Long`.
3. **Uninstaller Completeness (DX-21):** Update `scripts/uninstall.sh` to cleanly delete the DNS configuration files generated by the new wizard.
4. **Strict TLD Validation (QA-2):** Enforce strict `.localhost` validation via Regex in `validateRoutes()` to reject malformed/unsupported domains (e.g., wildcards like `*.ivin.localhost` or invalid characters). Must enforce `^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)*\.localhost$`.
5. **Orphaned Root Socket Cleanup (QA-1):** Fix the false-positive status reporting by using `kill -0` for PID verification, and proactively destroying orphaned sockets on `ECONNREFUSED`.

### Proposed Changes

#### 1. `internal/cli/init.go` (DNS Setup Wizard)
- **[MODIFY]** `internal/cli/init.go`:
  - Introduce `applyDNSConfig()` which detects the OS (using `runtime.GOOS`).
  - **Linux:** Check for `/etc/systemd/resolved.conf` to detect `systemd-resolved`, otherwise check for `/etc/dnsmasq.d`. 
    - If `systemd-resolved`: write `[Resolve]\nDNS=127.0.0.1:53\nDomains=~localhost` to `/etc/systemd/resolved.conf.d/devtether.conf` and run `systemctl restart systemd-resolved`.
    - If `dnsmasq`: write `server=/localhost/127.0.0.1#53` to `/etc/dnsmasq.d/devtether.conf` and run `systemctl restart dnsmasq`.
  - **macOS:** Write `nameserver 127.0.0.1` to `/etc/resolver/localhost`.
  - Wrap these in a `promptYN("Apply system-level DNS configuration so *.localhost resolves automatically? (requires sudo)")`.
  - Delete the manual "Supported Flags:" text from `initCmd.Long` to let Cobra's native flags block handle it.

#### 2. `internal/cli/root.go` (Cobra Completion Hide & DRY Help)
- **[MODIFY]** `internal/cli/root.go`:
  - Set `rootCmd.CompletionOptions.DisableDefaultCmd = true`.
  - Delete the manual command list from `rootCmd.Long` so `devtether --help` relies on Cobra's standard block.

#### 3. `internal/config/config.go` (Strict `.localhost` Validation)
- **[MODIFY]** `internal/config/config.go`:
  - In `validateRoutes()`, replace basic string suffix checks with a pre-compiled regex: `^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)*\.localhost$` to safely allow nested subdomains but block wildcards (`*`) and invalid characters.

#### 4. `internal/cli/status.go` & `internal/cli/down.go` (QA-1 Orphan Cleanup)
- **[MODIFY]** `internal/cli/status.go` / `internal/daemon/client.go`:
  - Improve cross-UID and liveness checks to verify PID liveness instead of relying solely on socket presence.
- **[MODIFY]** `internal/cli/down.go`:
  - Proactively `os.Remove()` the socket if `ECONNREFUSED` is encountered (orphaned socket).

#### 5. `scripts/uninstall.sh` & `README.md` (Cleanup & Docs)
- **[MODIFY]** `scripts/uninstall.sh`: Add `rm -f` commands for the DNS configs and reload resolvers.
- **[MODIFY]** `README.md`: Replace manual DNS config instructions with a simple instruction to run `devtether init`.

---

### Subphase 4.5: Deep UX/CLI Remediation Plan

Following a deep product-level audit based on recent manual testing, several critical UX defects and regressions were discovered. These bypass our existing test suite because they occur at the CLI orchestration layer.

#### 1. `internal/dns/server.go` & `internal/proxy/server.go` (Information Leakage)
- **[MODIFY]** `internal/dns/server.go`: Change `log.Info` to `log.Debug` during port fallback notifications (lines 66-78).
- **[MODIFY]** `internal/proxy/server.go`: Change `log.Info` to `log.Debug` during port fallback notifications (line 104).

#### 2. `internal/cli/up.go` (DNS Warning & Logger Re-initialization)
- **[MODIFY]** `internal/cli/up.go`:
  - Update `dnsUnavailableNotice()` to return a non-scary message: `"Internal DNS server unavailable (usually fine, modern OSes resolve *.localhost natively)."`.
  - In `upCmd.RunE`, immediately after parsing `cfg.Settings.Verbose` and modifying the `verbose` variable, add a call to `logger.Setup(verbose)` to re-configure the global logger.
  - In `daemonize()`, change `os.O_TRUNC` to `os.O_APPEND` when opening `devtether.log` so that daemon logs accumulate across restarts rather than being destroyed.

#### 3. `internal/cli/logs.go` (Memory Leaks & Race Conditions)
- **[MODIFY]** `internal/cli/logs.go`:
  - **Memory Leak (Industry Standard)**: Implement the GNU `tail -n` algorithm. Traverse backwards through the file using a fixed, small buffer (e.g., 4KB) purely to count `\n` characters. Once the Nth newline is found, note the byte offset, seek to it via `f.Seek()`, and stream to stdout via `io.Copy()`. This reduces memory usage from O(N^2) gigabytes to a flat ~4KB (O(1)), exactly matching the performance profile of GNU `tail`.
  - **FD Desync (Industry Standard)**: Emulate GNU `tail -F`. Because we are switching to `O_APPEND` (Issue #4), daemon restarts will *no longer truncate* the log, inherently fixing the restart data-loss bug! However, to protect against users or external tools (`logrotate`) manually truncating the file, we will add a standard GNU `tail` check: if `f.Stat().Size() < currentOffset`, reset the offset to 0 and print `--- Log Truncated ---`.
  - **Event Dropping (Robust Decoupling)**: Replace the patchwork `select` block. Spawn a dedicated goroutine that exclusively drains `fsnotify` events in an infinite loop, translating them into a non-blocking signal (via a 1-capacity channel). This completely decouples the OS event queue from the file I/O operations, guaranteeing `fsnotify` never blocks and never drops events. This is the gold standard for high-performance event loops (like Docker/Nginx epoll loops).

#### 4. `internal/cli/init.go` (DNS Config Hardcoding)
- **[MODIFY]** `internal/cli/init.go`:
  - **Avoid Port Drift (Least Tech Debt)**: To prevent a situation where the OS resolves to `5353` but a future `sudo devtether up` binds to `53`, the wizard will write the agreed-upon port directly into `devtether.yaml` (`dns.bind: "127.0.0.1:5353"` or `:53`). By explicitly locking the port in the YAML config, both the daemon and the OS resolver will always stay perfectly in sync, eliminating fallback race conditions entirely.

#### 5. Codebase & Documentation Audit Cleanup
- **[MODIFY]** Various Go Files (`internal/cli/logs.go`, `internal/daemon/lifecycle.go`, etc.):
  - Fix contradictory claims regarding polling vs. event-driven architecture, impossible startup sequences, and eviction strategies.
  - Remove "AI slop" and verbose pseudo-architectural fluff (e.g., SRP, "Smart Framework Noise Filter").
  - Scrub all files for ephemeral tracking tags (`(P3-1)`, `(QA-2)`, `(DX-20)`) and "production" terminology to strictly adhere to `AGENTS.md` rules.
- **[MODIFY]** Various Markdown Files (`PRD.md`, `architecture.md`, `engineering-standards.md`, `CHANGELOG.md`, `ADRs`):
  - Strip "Production-ready" claims to respect Beta Transparency.
  - Reframe unsupported features professionally (e.g. Windows compatibility, hot-reloading).
  - Clean up AI hallucinations (e.g. "Vercel DevTether") and inaccurate technical claims (e.g. O(1) `/proc/net/tcp` parsing, configured TLDs).
  - Document the rationale for the removal of the random `:0` port fallback (fail-fast architecture).

#### 6. Deep Documentation Sync (Subphase 4.6)
- **[NEW]** `docs/adr/011-init-wizard-and-system-mutations.md`: Document OS resolver mutations and `setcap` logic.
- **[NEW]** `docs/adr/012-static-asset-embedding-and-error-pages.md`: Document zero-allocation `//go:embed` GUI strategy.
- **[MODIFY]** `docs/adr/009-proxy-security-lessons.md`: Append proxy log throttling (`throttleCache`) mechanism and OS Service Management Implementation.
- **[MODIFY]** `docs/adr/006-platform-support-and-cgo-policy.md`: Append TTY ANSI color handling (pure-Go `golang.org/x/term` usage).
- **[MODIFY]** `docs/adr/005-yaml-config-schema.md`: Sync canonical schema example to match actual strict Go parsing.
- **[MODIFY]** `docs/architecture.md`: Add Proxy Layer Security, CLI Daemon Interactivity, expanded OS Integration, and Daemon Liveness Polling.
- **[MODIFY]** `docs/adr/README.md`: Index ADR-011 and ADR-012.

#### 7. Graceful Shutdown & IPC Concurrency Fixes (Round 6)
- **[MODIFY]** `internal/cli/up.go`: Added `signal.NotifyContext` (SIGINT/SIGTERM) to ensure foreground daemon runs gracefully clean up socket/lock files on user or system-monitor termination.
- **[MODIFY]** `internal/daemon/lifecycle.go` & `internal/cli/status.go`: Purged the static `/tmp/devtether-0` "root daemon" heuristic. Replaced with dynamic cross-UID detection that only surfaces the root hint upon explicit `EACCES` socket dial failures.
- **[MODIFY]** `internal/proxy/server.go`, `internal/dns/server.go`, `internal/daemon/daemon.go`: Introduced `shutdownComplete` channels to force `Serve()` to block until `Shutdown()` completes. Fixes a severe race condition where the parent process exited prematurely, bypassing the 5-second graceful connection drain.
- **[MODIFY]** `internal/daemon/daemon.go`: Removed an artificial infinite block (`<-r.Context().Done()`) inside the `POST /shutdown` handler that was causing `Shutdown()` to hit its 10-second timeout every single time.
- **[MODIFY]** `internal/cli/down.go`: Updated the client to explicitly poll `daemon.WaitForExit()` after triggering shutdown, ensuring `devtether down` accurately reflects actual daemon termination (5ms when idle, up to 5s when proxy is active).

---

## Deferred to Future Release

> [!NOTE]
> The following items are documented in the discussion space but explicitly deferred per the Anti-Slop Architecture mandate and the "Engine 1 Focus" sprint direction.

- **IPC REST API Refactor** (`GET /api/config`, `POST /api/config/routes`, `DELETE /api/config/routes/{domain}`).
- **Stateless Web GUI** (`devtether.localhost`, Vanilla JS, `//go:embed`).
- **Webhook Traffic Inspection** (`sync.Pool` buffering, SSE endpoint `/api/stream`).
- **Proxy OS-Assigned Port Tier Reconsideration (SC-10)** — product decision: the random-port tier conflicts with the fail-fast standard; decide keep/remove.
- **Daemon Log Rotation / Size Cap (SC-14)** — `.logs/devtether.log` currently grows unbounded.
- **Build-Tag Isolation of Unix-Only Syscalls (SC-15)** — `GOOS=windows` vet fails with raw type errors instead of the ADR-006 Tier 3 designed message.
- **Bounded Backwards Read for `devtether logs --lines` (SC-13)** — current `io.ReadAll` is unbounded as the log grows.
- **[Chore] Expire the `internal/daemon` test exemption in `engineering-standards.md`** — requires human owner approval (see `audit_report.md` §13.3).

These will be scoped into a future release once the daemon infrastructure is stable and battle-tested.

---

## Verification Plan

### Automated Tests
```bash
make test
```
Each new CLI command and config change must ship with table-driven tests. Ensure `go test -race ./...` introduces no regressions.

### Manual Verification
- `devtether up -d` starts a background process, writes PID, logs to `.logs/`.
- `devtether status` reports the daemon's state.
- `devtether doctor` identifies port conflicts and missing `setcap`.
- `devtether init` walks through interactive setup.
- ANSI colors disable cleanly via `NO_COLOR=1 devtether up`.
- `devtether up -d && devtether status` in a tight loop never reports "not running" (SC-5).
