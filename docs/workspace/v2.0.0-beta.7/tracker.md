# Release v2.0.0-beta.7
**Status:** Planned
**Target:** TBD

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->

- [ ] **[Feature] Cross-Platform Service Installer (`devtether service install`)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Description:* Build `internal/daemon/service.go` to generate daemon files for `systemd`, `launchd`, and `OpenRC`. Ensure privilege escalation safely binds to port 80.

- [ ] **[Feature] Active Diagnostics Command (`devtether doctor`)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Description:* Implement `devtether doctor` to test for XAMPP/Nginx port conflicts, validate `setcap`, ping DNS, and validate `devtether.yaml` syntax.

- [ ] **[Feature] IPC REST API Refactor**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Description:* Transition the IPC Unix socket server to `http.ServeMux` exposing REST endpoints (`GET /api/config`, `POST /api/config/routes`).

- [ ] **[Feature] Stateless Web GUI (`devtether.localhost`)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Medium
  *Description:* Scaffold a Vanilla JS/HTML dashboard embedded via `//go:embed`. Bridge browser API requests over the proxy to the internal Unix socket.

- [ ] **[Feature] Zero-Bloat Webhook Traffic Inspection (SSE)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Medium
  *Description:* Implement a `sync.Pool` of `bytes.Buffer` in the proxy middleware and expose an SSE endpoint at `/api/stream` for real-time, low-memory network inspection in the GUI.

## Scope Discoveries (Deferred / Backlog)
*(If an audit or idea creates new problems mid-sprint, log it here. Do NOT derail the current release.)*
- [ ] **[Category] Task Title**
  *Description:* Description of the discovered issue.
