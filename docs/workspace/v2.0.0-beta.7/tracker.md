# Release v2.0.0-beta.7
**Status:** In Progress
**Target:** TBD

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->

> **Note:** The OS Service module and its associated Phase 3 features (install, uninstall, daemonization) have been entirely deferred to future releases due to complexities. Their tracker history has been moved to `.ideas/service_mode/tracker.md`.


- [x] **[Feature] `settings:` Configuration Block**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* Critical
  *Description:* Add a `settings:` top-level key to `devtether.yaml` and `internal/config` for daemon mode, verbose, and log path.

- [x] **[Feature] Detached Daemon Mode (`devtether up -d`)**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* Critical
  *Description:* Add `-d`/`--detach` flag. Spawn a child process via `exec.CommandContext` with `SysProcAttr{Setsid: true}`. Also support `settings.daemon: true` for config-driven backgrounding.

- [x] **[Feature] Log Routing for Daemon Mode**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* High
  *Description:* When running detached, route output to `.logs/devtether.log` (relative to config dir). Create log directory with `0700`. Configurable via `settings.log_path`.

- [x] **[Docs] Phase 1 Documentation & Cleanup (Subphase 1.4)**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* High
  *Description:* Fix documentation debt from Phase 1: update root/up help text, README commands, init template, remove stale config fields, delete dead `root_out.go`. See audit findings 1, 4–10.

- [x] **[Feature] `devtether status` & `devtether down` Commands**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* High
  *Description:* Add `GET /status` and `POST /shutdown` IPC endpoints. Create `internal/cli/status.go` (tabwriter output) and `internal/cli/down.go` (graceful daemon shutdown). Both share the same Unix socket client path.

- [x] **[Feature] `devtether doctor` Command**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* High
  *Description:* Active diagnostics: port 80 conflicts, `setcap` check, DNS resolution test, stale `.sock` detection, `devtether.yaml` validation.

- [x] **[Feature] `devtether logs` Command**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* Medium
  *Description:* Tail daemon log files with `--lines N` support. Pure Go implementation (no external deps).

- [x] **[Fix] Phase 2 Remediation (Subphase 2.4)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Description:* Fix Findings 11-16 from the Phase 2 audit: block `sudo -d`, rename memory label to Heap, overhaul doctor UX (warning tier, macOS setcap guard), and add `-f` flag to logs.

- [x] **[Docs] Phase 2 Documentation Updates (Subphase 2.5)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Description:* After Phase 2 code is verified — update root cheat-sheet, README Commands, Cobra Long descriptions for all 4 new commands, godoc on new IPC handlers, and IPC endpoint table in `docs/architecture.md`.

- [x] **[Fix] Daemon Lifecycle Remediation (Subphase 2.6)**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* Critical
  *Blocked by:* Subphase 2.7 (architectural hardening must land first)
  *Description:* Fix severe daemon UX bugs: `logs -f` zombie loop (F18), 100ms shutdown race condition (F19), ghost TCP binds during graceful shutdown (F20), and silent port fallbacks when running in `-d` mode (F21).

- [x] **[Security] Daemon Lifecycle Hardening (Subphase 2.7)**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* Critical
  *Description:* Implement the minimum viable secure daemon startup sequence mandated by ADR-003. This is the single highest-priority task in the release. See audit findings A1–A7.
  *Scope:*
    - A1: flock-based instance ownership (lock file before any socket operations)
    - A2: Atomic PID file management (temp+fsync+rename)
    - A3: Secure fallback path (`/tmp/devtether-<uid>/` with 0700, Lstat, UID check)
    - A4: Symlink protection on socket directory (Lstat + ownership verification)
    - A5: Daemonize readiness verification (child → parent pipe)
    - A6: IPC server hardening (MaxHeaderBytes, ReadTimeout, WriteTimeout, MaxBytesReader)
    - A7: Document umask constraint

- [x] **[Audit] Phase 2 Post-Mortem & Cleanups (Subphase 2.8)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Medium
  *Description:* Perform a holistic audit of Phase 2 against the devtether-audit standard, clean up deprecated files (`premortem_2.6_2.7.md`), and resolve minor FD leaks.

- [x] **[Feature] MacOS & Root Lifecycle Support (Subphase 2.9)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Medium
  *Description:* Removed the `sudo -d` block. Validated that root daemons safely isolate into `/tmp/devtether-0` (or root `$XDG_RUNTIME_DIR`), allowing macOS users to bind port 80 natively. Updated CLI help texts to conditionally omit Linux `setcap` references on macOS builds.

- [x] **[Audit] Pre-Phase 3 Readiness Audit**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* High
  *Description:* Conducted full codebase, documentation, and dependency audit before Phase 3 implementation. Produced 8 findings (P3-1 through P3-8) covering idempotency semantics, non-interactive flags, service activation gaps, and documentation drift. See `audit_report.md` Section 10.

- [x] **[Feature] `devtether init` Interactive Wizard (Subphase 3.1)**
  *Assignee:* `@ivin-titus (via AntiGravity: Claude Opus 4.6)`
  *Priority:* Medium
  *Description:* Upgrade `internal/cli/init.go` from static template to interactive, flag-driven wizard. TTY detection via `golang.org/x/term`, Next.js-style flags (`--daemon`, `--setcap`), OS-aware prompt flow, and `config.LoadConfig` validation. Testable via `io.Reader`/`io.Writer` injection.

- [x] **[Bugfix] Consolidated Remediation — Audit & Hardening Fixes (Subphase 3.3)**
  *Assignee:* `@ivin-titus (via Codex: GPT 5.6 Terra)`
  *Priority:* Critical
  *Status:* **Implemented and Verified**. All fixes are fully implemented and regression-tested. Direct code verification and test suite execution (Subphase 3.4 Part F) confirm the fixes are safe and active.
  *Description:* Resolve all open audit findings (project-wide QA and second-opinion audit). Consolidates the former Subphase 3.2.1 and 3.2.2. Every fix requires a regression test; no finding is closed without one. Full step list in `implementation_plan.md` Subphase 3.3.
  *Scope:*
    - **Part 1 (init):** TTY via injected reader, setcap `/tmp` warning, quoted setcap hint.
    - **Part 2 (Daemon lifecycle):** Readiness read deadline, synchronous IPC bind before readiness signal, service-manager foreground guard, `ReadPID` panic fix, PID-based SIGTERM fallback + cross-UID status hint, readiness encode error.
    - **Part 3 (CLI core):** Typed errors preserve exit 1 via `main.go`; the shutdown watchdog is an authorized `os.Exit(1)` exception; respect `NO_COLOR`, errgroup goroutines, sorted route logs, `errors.Is`, doctor check correctness, DNS failure visibility, sudo messaging coherence.
    - **Part 4 (Network/proxy/config):** Client dialer timeout, remove dead `proxy.timeouts.read/write` schema, context propagation in daemon client, narrow interface for `HeadersSent`, `defer` umask, normalize host on delete + empty-domain guard + sorted API output, `errors.Is` in netutil, DNS fallback preserves bind IP, strict sequence — drop `TrimSpace`, strict YAML decoding.
    - **Testing:** Regression test per fix; minimal syscall seam for lifecycle tests.
    - **Final remediation:** Isolate Unix-only service ownership validation; add the IPC shutdown nonce; replace unbounded log reads with a backwards chunk reader and polling with authorized pure-Go `fsnotify`; cap the daemon log on startup. Proxy `ReadTimeout`/`WriteTimeout` remain an ADR-008 streaming exemption.

- [x] **[Bug Fix] Product & DX Polish / Missing Pieces (Subphase 3.4)**
  *Assignee:* `@ivin-titus (via Cline: Deepseek v4.1 Flash)`
  *Priority:* High
  *Blocked by:* Subphase 3.3
  *Blocks:* Subphase 3.5 (documentation sync must reflect these behavior changes)
  *Description:* Resolve the product-level blind-spot findings DX-1…DX-18 (`audit_report.md` §15): truthful port-fallback and DNS-state reporting in `up`/`doctor`, consistent cross-UID daemon hints, `logs -f` false shutdown message and `--lines` validation, daemon identity in `status`/`CheckRunning`, `down` recovery on non-permission IPC failures, replacing the deleted `examples/` directory with a rich `devtether init` template to solve schema drift, the ADR-009 §5→§6 citation correction, and documentation contradictions (hot-reload claim, README Commands/`status`/`~internal` DNS snippet). **Part F executed the DX-18 per-item verification pass and corrected the Subphase 3.3 completion record.** Full step list in `implementation_plan.md` Subphase 3.4.

- [x] **[Docs] Documentation Synchronization & Final Consistency (Subphase 3.5)**
  *Assignee:* `@ivin-titus (via Cline: GLM 5.3 Flash)`
  *Priority:* High
  *Blocked by:* Subphase 3.4
  *Description:* Sync every permanent documentation surface with the resulting implementation: `docs/architecture.md` (init wizard, nonce-protected shutdown endpoint, re-verified startup sequence, corrected route-reload semantics), `README.md` (Commands table, Post-Install cross-references, bounded detached-log policy), `root.go` cheat-sheet, and `init` Cobra `Long` descriptions. Final consistency gate: record the ADR-008 streaming timeout exemption, the authorized watchdog exit, `fsnotify` dependency rationale, and the remaining Windows platform boundary; no ephemeral terminology or "production-ready" claims. (Formerly Subphase 3.3; renumbered to Subphase 3.4 when remediation became Subphase 3.3, and to Subphase 3.5 when Product & DX Polish took Subphase 3.4.)

- [x] **[Feature] Phase 4 Polish (DNS Wizard & CLI Cleanups)**
  *Assignee:* `@ivin-titus (via Cline: Deepseek v4.1 Flash)`
  *Priority:* High
  *Status:* **Implemented — Awaiting Human Verification.** Code, regression tests, lint/vet/race gates, and documentation have landed. The checkbox stays open until the system owner completes the manual verification steps below (per `devtether-change` §6).
  *Description:* Enhance `devtether init` with a smart OS-aware DNS configuration wizard (`systemd-resolved`, `macOS resolver`, `dnsmasq`). Clean up Cobra CLI help generation by hiding the `completion` command and removing redundant manual command lists from `Long` descriptions. Expand `scripts/uninstall.sh` to remove generated DNS configs. Strictly enforce `.localhost` structure in config via regex to block wildcards and resolve QA-2. Resolve orphaned root sockets (QA-1) via `kill -0` liveness checks and proactive unlinking.
  *Delivered:*
    - `internal/cli/init.go`: `detectDNSProvider` + `applyDNSConfig` + a shared `runSudo` privileged-command helper (also used by `applySetcap`); the DNS prompt is interactive-only and skipped when no supported resolver is detected.
    - `internal/cli/root.go`: `CompletionOptions.DisableDefaultCmd = true`; the manual command cheat-sheet is gone from `rootCmd.Long`, and `initCmd.Long` no longer duplicates the flags block.
    - `internal/config/config.go`: `localhostRouteName` regex rejects wildcards, single-label names, malformed labels, and non-`.localhost` TLDs in `validateRoutes()` (structure is checked on the normalized name, matching the router's `NormalizeHost`).
    - `internal/daemon/lifecycle.go`: `PIDAlive` (`kill -0`, EPERM counts as alive) + `CleanStaleSocket`. `internal/daemon/daemon.go`: the socket is unlinked on `Serve` exit while the instance lock is still held. `internal/cli/status.go` / `down.go` recover from orphaned sockets.
    - `scripts/uninstall.sh`: removes the generated DNS configs and reloads the resolver.
    - `README.md` + `docs/architecture.md`: Post-Install DNS steps now point at `devtether init`.
    - *(Brutal smoke-test follow-ups, QA-3):* `internal/cli/up.go`: when a detached daemon crashes during startup, the parent now surfaces the child's fatal error line (bounded tail read via `lastStartupError`, `\r`-aware) instead of only pointing at the log; regression `TestLastStartupError`. `internal/daemon/lifecycle.go`: `RuntimeDir()` validates a pre-existing runtime directory *before* `MkdirAll`, so a symlinked path (including a dangling target) is refused with the explicit ADR-003 message rather than a generic `file exists` error; regression `TestRuntimeDirRejectsDanglingSymlink`.
  *Manual verification:*
    1. `devtether --help` — no `completion` command and no duplicated command cheat-sheet.
    2. `devtether init --help` — flags are printed once (Cobra-native only).
    3. `devtether init` (Linux/macOS TTY) — prompts for system DNS and writes the resolver config after `sudo`.
    4. A route such as `app.internal: 3000` or `"*.localhost": 3000` fails validation with exit 1 and a `.localhost` hint. (Note: the repository's local, gitignored `devtether.yaml` still contains `job-flow.internal`, so `devtether up` will now fail until that key is changed.)
    5. `kill -9` a detached daemon, then `devtether status` / `devtether down` — the orphaned socket is unlinked and the daemon reported as not running.

- [x] **[Bug Fix] Deep UX/CLI Remediation (Subphase 4.5)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Critical
  *Description:* Implement robust, industry-standard fixes for orchestration and UX defects discovered post-Phase 4 (DX-23 through DX-30). Converts `logs --lines` to an O(1) memory GNU `tail` algorithm, handles log truncation via stat polling, decouples `fsnotify` via a dedicated event-draining goroutine to prevent race conditions, and adds dynamic port calculation to the DNS configuration wizard.
  *Planned Tasks:*
    - `dns/server.go` & `proxy/server.go`: Suppress fallback logs to `.Debug()`.
    - `cli/up.go`: Soften DNS warning, propagate logger setup, use `O_APPEND`.
    - `cli/logs.go`: Rewrite `readBackwardChunk` to O(1) backward scan, implement `tail -F` truncation detection, add event drainer loop.
    - `cli/init.go`: Inject dynamically bound port `53`/`5353` into OS resolver configuration.
    - `internal/cli`: Update unit tests (`cli_test.go`) to expect dynamic DNS ports and new message strings.
    - All tests must pass cleanly.

- [x] **[Chore] Codebase & Documentation Audit Cleanup (Subphase 4.5)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Medium
  *Status:* **Implemented**. All AI slop, contradictory comments, unedited placeholders, and false claims purged.
  *Description:* Perform a comprehensive sweep of all code comments and markdown documentation to remove contradictory statements, AI slop, repository rule violations (e.g., ephemeral tracking tags), and cryptic references discovered during the Subphase 4.6 architectural checks.
  *Delivered:*
    - Codebase: Fix incorrect claims, remove AI fluff, and scrub ephemeral tags/production claims.
    - Documentation: Reframe Windows compatibility and hot-reloading realistically.

- [x] **[Feature] Strict Fail-Fast DNS Architecture (Subphase 4.7)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Status:* **Implemented**. The DNS daemon now binds natively to `127.0.0.1:5335` by default, eliminating unprivileged port conflicts. The wizard writes `5335` into the OS resolver configuration directly. The silent `5353` fallback logic in the DNS server is entirely removed to guarantee fail-fast determinism. DX-30 fully resolved.
    - Documentation: Purge "Vercel DevTether" hallucination and "Intelligent IP Cycling" false claims.
    - Documentation: Remove "Production-ready" claims from `PRD.md` to respect Beta Transparency.
    - Documentation: Update architectural docs to reflect `.localhost` hardcoding and the removal of the `:0` fallback port (documenting the fail-fast rationale).

- [x] **[Chore] Deep Documentation Sync (Subphase 4.6)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Status:* **Implemented**. All documentation drift eliminated via YAGNI constraints and architectural formalization.
  *Description:* Perform a deep synchronization of the formal documentation (`docs/adr` and `docs/architecture.md`) against the actual shipped codebase to eliminate structural drift ahead of the beta.7 release.
  *Delivered Tasks:*
    - **ADR-011**: Created ADR for `devtether init` automated setup flow and system OS mutations.
    - **ADR-001**: Amended to enforce a YAGNI zero-allocation `//go:embed` static asset strategy for any future GUI (preventing Electron bloat).
    - **ADR-009**: Amended to formalize `throttleCache` eviction and OS Service Management (`service install`).
    - **ADR-006**: Amended to document pure-Go CLI TTY / `golang.org/x/term` usage.
    - **ADR-002 & ADR-005**: Amended with strict YAGNI scope limitations (dropping `.internal`/custom TLDs and live config reloading).
    - **Architecture Updates**: Added Proxy Security (throttleCache), CLI Daemon Interactivity (`logs -f`), OS Integration, and Daemon Liveness Polling (`flock`) to `architecture.md`.

- [x] **[Bug Fix] Graceful Shutdown & IPC Concurrency Fixes (Round 6)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Critical
  *Status:* **Implemented**. All concurrency defects resolved.
  *Description:* Resolved an array of deep Go concurrency bugs discovered via ad-hoc mental tracing (Findings 17.1 to 17.4).
  *Delivered Tasks:*
    - **Foreground Interrupts:** Wrapped foreground boot sequence in `signal.NotifyContext` (catching `SIGINT` and `SIGTERM`) to guarantee socket cleanup on Ctrl+C or System Monitor termination.
    - **Status False Positive:** Purged the flawed `RootDaemonMayBeRunning` heuristic that permanently flagged the sudo hint for standard users after a single root execution. The hint is now accurately scoped to active `EACCES` IPC failures.
    - **Graceful Shutdown Bypass:** Prevented `devtether down` from bypassing the 5-second proxy active connection drain by introducing `shutdownComplete` channels. `Serve()` methods across Proxy, DNS, and IPC now explicitly block until `Shutdown()` finishes.
    - **IPC Deadlock:** Fixed a bug where `handleShutdown` blocked infinitely on `<-r.Context().Done()`, causing the internal `Shutdown()` to hit its 10-second timeout on every invocation. `devtether down` now polls `daemon.WaitForExit()` to correctly track daemon termination without artificial delays.

## Scope Discoveries (Deferred / Backlog)
*(If an audit or idea creates new problems mid-sprint, log it here. Do NOT derail the current release.)*
- [ ] **[Feature] IPC REST API Refactor**
  *Description:* Migrate `GET /routes`, `GET /status` to `/api/*` prefix and introduce `POST /api/config/routes` and `DELETE /api/config/routes/{domain}` to enable dynamic routing updates without daemon restarts.
- [ ] **[Feature] Stateless Web GUI**
  *Description:* Built with Vanilla JS and `//go:embed`, served at `devtether.localhost`. Will require strict Origin validation (ADR-009 §5). Blocked by IPC REST API Refactor.
- [ ] **[Feature] Webhook Traffic Inspection**
  *Description:* Use `sync.Pool` to buffer traffic and serve via SSE on `/api/stream` for local webhook debugging.
- [ ] **[Chore] Expire the `internal/daemon` Test Exemption**
  *Description:* Formally drop the `engineering-standards.md` exemption now that the lifecycle package has a syscall seam.
- [ ] **[Feature] Non-TTY CLI Overrides & Automation Flags**
  *Description:* Fully defer global non-interactive flags (e.g., `--debug`, `--port`, `--log-level`) across the CLI to support automated generation. (Note: basic automation flags like `--daemon` and `--setcap` for `init` were successfully implemented in Phase 3.1).
