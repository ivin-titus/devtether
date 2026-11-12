# Release v2.0.0-beta.8
**Status:** In Progress
**Target:** TBD

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->

- [ ] **[Feature] Live Route Management (`devtether routes add` / `devtether routes remove`)**
  *Assignee:* `@ivin-titus (via Devin: SWE-2)`
  *Priority:* High
  *Status:* **Implemented and merged into `develop` via owner-signed squash commit `f897117` (2026-11-10). Owner-verified: local run on Zorin OS 18 covered all normal interactive flows including remove-confirmation and `-y` (owner, 2026-11-11); abnormal-case QA remains part of final pre-release QA. Remaining Slice 4 work and beta.8 release closeout are open.**
  *Delivered:* `f897117` on `develop`.
  *Implementation history:* developed on `feature/devin` as Slices 1–4 — `20ef459` (config + router), `babaaf42` (health incarnations), `1dd37bb` (IPC + CLI), `c84acbd` (docs Batch 1), `71c2160` (docs Batch 2), `1bedca7` (Batch 2.1 UX accuracy corrections) — and squash-merged as `f897117`. The slice SHAs are historical references: they are superseded by `f897117` and are not reachable in the current tree.
  *Reviews recorded:* owner review of Slice 1 @ `20ef459` vs `develop` (2026-11-07): no confirmed code defects. Independent review of Slice 2 @ `babaaf42` (2026-11-07, fresh read-only audit + external review): no confirmed code defects; closure pass strengthened incarnation concurrency tests and reconciled workspace status (see [audit report](artifacts/audit_report.md) §1d). Slice 3 read-only pre-implementation audit at `7c25ff5` (2026-11-10): READY WITH CONDITIONS, no confirmed defects; owner accepted C1–C7 + clarifications and the strict `inc`/JSON decisions (§1e of the [audit report](artifacts/audit_report.md)); owner review of docs `7ff7e67` accepted the design with three corrections F1–F3 applied (§1f). Slice 3 implementation published at `1dd37bb` with a focused closure pass (literal CLI domain validation, opaque incarnation output, corrected `routes` help) — see [audit report](artifacts/audit_report.md) §1g. Slice 4 Batch 2.1 `1bedca7` independent read-only audit (2026-11-10): ACCEPT WITH CONDITIONS, no confirmed code defects — see [audit report](artifacts/audit_report.md) §1k. Minor observations recorded: the UNKNOWN footnote is less actionable in config-fallback mode when the daemon is unavailable (optional future UX polish, not a defect). Full design in [implementation_plan.md](artifacts/implementation_plan.md); decisions in [discussion_space.md](artifacts/discussion_space.md); pre-mortem + corrective review in [audit_report.md](artifacts/audit_report.md).
  *Manual verification:* owner local run on Zorin OS 18 covering all normal interactive flows passed (owner, 2026-11-11); abnormal-case QA deferred to final pre-release QA.
  *Blocked by:* nothing for the merged scope; Slice 4 remainder and beta.8 release closeout remain open.
  *Description:* Runtime route mutation without `devtether down && devtether up`. Adds `POST /routes` (strict create → 201/409/413) and `DELETE /routes/{domain}?inc=N` (→204/404/409, incarnation required) on the existing flat IPC surface behind the IPC nonce, shared config route validation (`config.ValidateRoute`/`config.ValidateRouteDomain`), atomic create inside `router.Engine`, and incarnation-scoped health identity (`internal/health` extension of the generation model shipped in beta.7/post-beta.7). Runtime-only — never writes `devtether.yaml`: adds create ephemeral `type:"live"` routes discarded on restart; removes work on static AND live routes and require confirmation for BOTH (warning tailored to type; `-y/--yes` skips; non-TTY fails safe), and deletion is incarnation-required (`DELETE ?inc=N`; missing → 400, mismatch → 409) so a stale confirmation can never kill a concurrent replacement (D14/D15). No YAML persistence or file watching.

- [x] **[Chore] Proxy test strengthening + dependency updates**
  *Assignee:* `@ivin-titus (via Devin: SWE-2)`
  *Priority:* Medium
  *Status:* **Merged into `develop` via owner-signed squash commit `2b6a75f` (PR #14, 2026-10-31).**
  *Description:* Strengthened `internal/proxy` handler tests (+409 lines in `internal/proxy/handler_test.go`) and updated module dependencies (`go.mod`/`go.sum`) plus the CI action pin (`.github/workflows/ci.yml`).
  *Delivered:* `2b6a75f` on `develop`.
  *Manual verification:* none recorded — evidence is the PR #14 CI gate and the owner-signed merge itself.

- [ ] **[Security] Detached daemon log safe-open (symlink/arbitrary-file-write remediation)**
  *Assignee:* `@ivin-titus (via Devin: Fable 5.2 + SWE-2 Flash)`
  *Priority:* High
  *Status:* **Implemented + independently reviewed; awaiting owner acceptance.** Security implementation committed as `bcb20c3` (`fix(cli): reject unsafe symlinked daemon log paths`); documentation follow-up (UID review + audit/tracker reconciliation) committed as `d99cfff` (`docs(workspace): record detached log safe-open UID review and commit reference`); UID-ownership UX slice committed as `71e9dec` (`fix(cli): add actionable guidance to log ownership mismatch errors`). Security implementation pushed to `origin/feature/devin`; security HEAD is `c716f03` — six commits ahead of `origin/develop` @ `f897117` (`bcb20c3`, `d99cfff`, `71e9dec`, `1e5f152`, `73a14d6`, `c716f03`; earlier SHAs retained below as implementation provenance). Pre-mortem + verification evidence recorded in `audit_report.md` §1l. Helper `internal/cli/logfile.go` uses `golang.org/x/sys/unix` `Openat`+`O_NOFOLLOW`+`Fstat` validation; shared by `daemonize()` writes, `lastStartupError()` reads, and `devtether logs` reads. 22 regression tests added in `internal/cli/logfile_test.go`. gofmt/race-tests/build/vet/lint(0 issues)/cross-compile(linux+darwin)/diff-check all pass. The `bash scripts/test.sh` module-hygiene failure noted during verification was pre-commit only (uncommitted `go.mod`); it is resolved now that `bcb20c3` is committed. Follow-up UID-ownership UX slice (actionable mismatch errors + first-run/foreign-owner/concurrency regression tests + docs) pushed as `71e9dec`; its investigation found no code defect (environmental filesystem cause) — see `audit_report.md` §1m. **Slice 0 (runtime-directory isolation + secure resolution) committed as `1e5f152`; independent read-only audit returned ACCEPT WITH CONDITIONS (no confirmed production defect); audit conditions corrected with evidence (real foreign-ownership root-gated test executed under sudo, client no-dial test, doc corrections, ELOOP probe finding — ELOOP unreachable on Linux, ENOTDIR path covered instead); Batch 3 integration re-verification (2026-11-11, recorded in §1p): `go test -race -count=1 ./internal/...` PASS, `bash scripts/test.sh` PASS (6 passed, 2 skipped — linters absent at the time), build/vet/cross-compile (linux/darwin) PASS. Independent complete-branch integration audit of `feature/devin` @ `c716f03` (2026-11-11) returned **ACCEPT WITH CONDITIONS** — no confirmed defects, `make test` 8 passed / 0 skipped, race tests/build/vet/lint(0 issues)/govulncheck/cross-compile/root-gated tests/E2E all PASS; the four conditions were documentation-only and are corrected in this pass; macOS runtime verification and owner acceptance remain outstanding** — see `audit_report.md` §1n (pre-mortem), §1o (Slice 0 implementation + verification), §1p (Slice 0 independent audit + corrections + integration re-verification), §1q (complete-branch integration audit). Security finding remains open pending owner acceptance. Pending owner steps: (1) owner local run verification of `feature/devin` @ `c716f03`; (2) owner GPG-signed squash integration into `develop` (acceptance convention in `docs/workspace/README.md` §2). Beta.8 stays open.
  *Description:* Devin Security finding (medium): `daemonize()` followed symlinks for `.logs/devtether.log` or a symlinked `settings.log_path` directory, allowing a malicious repository to redirect detached daemon stdout/stderr (including access-log lines) into an attacker-chosen file. Remediation: refuse symlinked log files/directories, require regular file + link count 1 + current-user ownership + no group/world write, fail closed before child spawn, preserve append/FD3 readiness/startup-error reporting.

- [x] **[Chore] `scripts/test.sh` executable bit**
  *Assignee:* TBD
  *Priority:* Low
  *Status:* **Fixed in the workspace-docs commit on this branch** — executable bit restored (100755); `make test` verified passing (6 passed, 2 skipped; golangci-lint/govulncheck not installed locally).

## Scope Discoveries (Deferred / Backlog)
*(If an audit or idea creates new problems mid-sprint, log it here. Do NOT derail the current release.)*
- [ ] **[Feature] IPC REST API Refactor** *(carried from beta.7)* — `/api/*` prefix migration remains deferred; live routes ship on the existing flat endpoints. The live-route endpoints cover the dynamic-routing portion of this idea.
- [ ] **[Feature] Stateless Web GUI** *(carried from beta.7)* — blocked by IPC REST API Refactor.
- [ ] **[Feature] Webhook Traffic Inspection** *(carried from beta.7)*
- [ ] **[Chore] Expire the `internal/daemon` Test Exemption** *(carried from beta.7)*
- [ ] **[Feature] Non-TTY CLI Overrides & Automation Flags** *(carried from beta.7)*

## Baseline & Provenance
- Baseline: `feature/devin` = `develop` @ `f897117` + 4 unsigned security commits (`bcb20c3`, `d99cfff`, `71e9dec`, `1e5f152` — security remediation + Slice 0; see the [Security] task) + 1 workspace-docs commit (`73a14d6`) + 1 toolchain-bump commit (`c716f03`) — 6 commits total. Branch flow: `feature/devin` → PR into `develop`; owner squash-merges; never merge.
- `origin/main` is an ancestor of `origin/develop`; `develop` = `main` + 3 owner-signed squash-merged commits not yet on main:
  - `2b6a75f` — proxy test strengthening + dep bumps (PR #14).
  - `6fac500` — on-demand route health: `internal/health` tracker, one-shot startup check, `POST /routes/check`, `routes --check` (PR #15). Live routes extend this tracker.
  - `f897117` — live route management (squash merge of beta.8 Slices 1–4).

## Canonical references
- [Architecture](../../architecture.md) · [ADR index](../../adr/README.md) · [Engineering standards](../../engineering-standards.md) — current technical truth; verify workspace claims against them and the source tree.
