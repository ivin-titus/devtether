# Release v2.0.0-beta.8
**Status:** In Progress
**Target:** TBD

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->

- [ ] **[Feature] Live Route Management (`devtether routes add` / `devtether routes remove`)**
  *Assignee:* `@ivin-titus (via Devin)`
  *Priority:* High
  *Status:* **Planned — awaiting slice-by-slice implementation approval.** Full design in `artifacts/implementation_plan.md`; decisions in `artifacts/discussion_space.md`; pre-mortem in `artifacts/audit_report.md`.
  *Description:* Runtime route mutation without `devtether down && devtether up`. Adds `POST /routes` (strict create → 201/409/413) and `DELETE /routes/{domain}` (→204/404/409) on the existing flat IPC surface behind the IPC nonce, shared `config.ValidateRoute`, atomic create inside `router.Engine`, and incarnation-scoped health identity (`internal/health` extension of the generation model shipped in beta.7/post-beta.7). Runtime-only — never writes `devtether.yaml`: adds create ephemeral `type:"live"` routes discarded on restart; removes work on static AND live routes and require confirmation for BOTH (warning tailored to type; `-y/--yes` skips; non-TTY fails safe), and deletion is incarnation-required (`DELETE ?inc=N`; missing → 400, mismatch → 409) so a stale confirmation can never kill a concurrent replacement (D14/D15). No YAML persistence or file watching.

- [ ] **[Chore] `scripts/test.sh` executable bit**
  *Assignee:* TBD
  *Priority:* Low
  *Status:* **Finding recorded.** `scripts/test.sh` is committed mode 100644, so `make test` (`./scripts/test.sh`) cannot exec it; local runs use `bash scripts/test.sh`. CI unaffected (calls `go test` directly). One-line `chmod +x` fix; kept as a separate repository-hygiene item, not folded into the live-routes diff.

## Scope Discoveries (Deferred / Backlog)
*(If an audit or idea creates new problems mid-sprint, log it here. Do NOT derail the current release.)*
- [ ] **[Feature] IPC REST API Refactor** *(carried from beta.7)* — `/api/*` prefix migration remains deferred; live routes ship on the existing flat endpoints. The live-route endpoints cover the dynamic-routing portion of this idea.
- [ ] **[Feature] Stateless Web GUI** *(carried from beta.7)* — blocked by IPC REST API Refactor.
- [ ] **[Feature] Webhook Traffic Inspection** *(carried from beta.7)*
- [ ] **[Chore] Expire the `internal/daemon` Test Exemption** *(carried from beta.7)*
- [ ] **[Feature] Non-TTY CLI Overrides & Automation Flags** *(carried from beta.7)*

## Baseline & Provenance
- Baseline: `develop` @ `6fac500` (branch: `feature/devin` → PR into `develop`; never merge).
- `origin/main` is an ancestor of `origin/develop`; `develop` = `main` + 2 squash-merged commits not yet on main:
  - `2b6a75f` — proxy test strengthening + dep bumps (PR #14).
  - `6fac500` — on-demand route health: `internal/health` tracker, one-shot startup check, `POST /routes/check`, `routes --check` (PR #15). Live routes extend this tracker.
