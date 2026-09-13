# Release v2.0.0-beta.7
**Status:** Planned
**Target:** TBD

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->

- [ ] **[Feature] `settings:` Configuration Block**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* Critical
  *Description:* Add a `settings:` top-level key to `devtether.yaml` and `internal/config` for daemon mode, verbose, log level, and log path. This is the foundation all other beta.7 features depend on.

- [ ] **[Feature] Detached Daemon Mode (`devtether up -d`)**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* Critical
  *Description:* Add `-d`/`--detach` flag. Spawn a child process via `exec.Command` with `SysProcAttr{Setsid: true}`. Also support `settings.daemon: true` for config-driven backgrounding.

- [ ] **[Feature] Log Routing for Daemon Mode**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* High
  *Description:* When running detached, route `slog` output to `.logs/devtether.log` (relative to config dir). Create log directory with `0700`. Configurable via `settings.log_path`.

- [ ] **[Feature] `devtether status` Command**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* High
  *Description:* Add `GET /status` IPC endpoint (uptime, PID, routes, memory). Create `internal/cli/status.go` with `text/tabwriter` output.

- [ ] **[Feature] `devtether doctor` Command**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* High
  *Description:* Active diagnostics: port 80 conflicts, `setcap` check, DNS resolution test, stale `.sock` detection, `devtether.yaml` validation.

- [ ] **[Feature] `devtether logs` Command**
  *Assignee:* `@ivin-titus (via AntiGravity)`
  *Priority:* Medium
  *Description:* Tail daemon log files with `--lines N` support. Pure Go implementation (no external deps).

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
