# Release v2.0.0-beta.7
**Status:** In Progress
**Target:** TBD

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->

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
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* Medium
  *Description:* Perform a holistic audit of Phase 2 against the devtether-audit standard, clean up deprecated files (`premortem_2.6_2.7.md`), and resolve minor FD leaks.

- [x] **[Feature] MacOS & Root Lifecycle Support (Subphase 2.9)**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* Medium
  *Description:* Removed the `sudo -d` block. Validated that root daemons safely isolate into `/tmp/devtether-0` (or root `$XDG_RUNTIME_DIR`), allowing macOS users to bind port 80 natively. Updated CLI help texts to conditionally omit Linux `setcap` references on macOS builds.

- [ ] **[Feature] `devtether init` Interactive Wizard**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* Medium
  *Description:* Replace the static template dump with an interactive OS-aware setup: daemon vs standalone, DNS resolver detection, `setcap` opt-in, config validation. Least privilege by default.

- [ ] **[Feature] Cross-Platform Service Installer (`devtether service install`)**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* Medium
  *Description:* Generate daemon files for `systemd` (Linux), `launchd` (macOS), `OpenRC` (Alpine/Arch). Idempotent install/uninstall.

## Scope Discoveries (Deferred / Backlog)
*(If an audit or idea creates new problems mid-sprint, log it here. Do NOT derail the current release.)*
- [ ] **[Feature] IPC REST API Refactor**
  *Description:* Transition IPC daemon to full `http.ServeMux` with REST endpoints for config mutation (`GET /api/config`, `POST /api/config/routes`, `DELETE /api/config/routes/{domain}`). Deferred until daemon infrastructure is stable.
- [ ] **[Feature] Stateless Web GUI (`devtether.localhost`)**
  *Description:* Vanilla JS/HTML dashboard embedded via `//go:embed`. Bridges browser API requests to Unix socket. Deferred to post-daemon release.
- [ ] **[Feature] Zero-Bloat Webhook Traffic Inspection (SSE)**
  *Description:* `sync.Pool`-based request buffering + SSE endpoint for real-time network inspection. Deferred to post-GUI release.
