# Release v2.0.0-beta.8
**Status:** In Progress
**Target:** TBD

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->

- [ ] **[Feature] Live Route Management (`devtether routes add` / `devtether routes remove`)**
  *Assignee:* `@ivin-titus (via Devin)`
  *Priority:* High
  *Status:* **Slice 1 (config + router) implemented and pushed at `20ef459`; Slice 2 (health incarnations) implemented and pushed at `babaaf42`; Slice 3 (IPC + CLI) implemented and pushed at `1dd37bb`, pending owner review; Slice 4 Batch 1 (documentation) committed as `c84acbd`; Slice 4 Batch 2 pushed as `71c2160`; Slice 4 Batch 2.1 (UX accuracy corrections) pushed as `1bedca7` and reviewed — independent read-only audit returned ACCEPT WITH CONDITIONS with no confirmed code defects; Slice 4 remaining work and beta.8 release not started.** Owner review of `feature/devin` @ `20ef459` vs `develop` (2026-11-07): no confirmed code defects. Independent review of Slice 2 @ `babaaf42` (2026-11-07, fresh read-only audit + external review): no confirmed code defects; closure pass strengthened incarnation concurrency tests and reconciled workspace status (see `artifacts/audit_report.md` §1d). Slice 3 read-only pre-implementation audit at `7c25ff5` (2026-11-10): READY WITH CONDITIONS, no confirmed defects; owner accepted C1–C7 + clarifications and the strict `inc`/JSON decisions (see `artifacts/audit_report.md` §1e); owner review of docs `7ff7e67` accepted the design with three corrections F1–F3 applied (see §1f). Slice 3 implementation published at `1dd37bb` (2026-11-10) with a focused closure pass (literal CLI domain validation, opaque incarnation output, corrected `routes` help); pending owner review — see `artifacts/audit_report.md` §1g. Slice 4 Batch 1 committed as `c84acbd`. Slice 4 Batch 2 pushed as `71c2160`. Slice 4 Batch 2.1 (conclusive-observation wording in `routes`/`routes add` output and help, `noRoutesHint` static+live guidance) pushed as `1bedca7`; independent read-only audit (2026-11-10) returned ACCEPT WITH CONDITIONS with no confirmed code defects — see `artifacts/audit_report.md` §1k. Two minor observations recorded: owner-side TTY verification remains pending, and the UNKNOWN footnote is less actionable in config-fallback mode when the daemon is unavailable (optional future UX polish, not a defect). Remaining Slice 4 work and beta.8 release not started — awaiting explicit approval. Full design in `artifacts/implementation_plan.md`; decisions in `artifacts/discussion_space.md`; pre-mortem + corrective review in `artifacts/audit_report.md`.
  *Description:* Runtime route mutation without `devtether down && devtether up`. Adds `POST /routes` (strict create → 201/409/413) and `DELETE /routes/{domain}?inc=N` (→204/404/409, incarnation required) on the existing flat IPC surface behind the IPC nonce, shared config route validation (`config.ValidateRoute`/`config.ValidateRouteDomain`), atomic create inside `router.Engine`, and incarnation-scoped health identity (`internal/health` extension of the generation model shipped in beta.7/post-beta.7). Runtime-only — never writes `devtether.yaml`: adds create ephemeral `type:"live"` routes discarded on restart; removes work on static AND live routes and require confirmation for BOTH (warning tailored to type; `-y/--yes` skips; non-TTY fails safe), and deletion is incarnation-required (`DELETE ?inc=N`; missing → 400, mismatch → 409) so a stale confirmation can never kill a concurrent replacement (D14/D15). No YAML persistence or file watching.

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
