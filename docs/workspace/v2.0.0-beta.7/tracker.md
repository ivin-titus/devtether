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

- [ ] **[Feature] Phase 4 Polish (DNS Wizard & CLI Cleanups)**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* High
  *Status:* **Planned (Pending Execution)**
  *Description:* Enhance `devtether init` with a smart OS-aware DNS configuration wizard (`systemd-resolved`, `macOS resolver`, `dnsmasq`). Clean up Cobra CLI help generation by hiding the `completion` command and removing redundant manual command lists from `Long` descriptions. Expand `scripts/uninstall.sh` to remove generated DNS configs. Strictly enforce `.localhost` TLD validation in config to resolve QA-2. Resolve orphaned root sockets (QA-1) via `kill -0` liveness checks and proactive unlinking.

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
