# Agent Diary — v2.0.0-beta.8

Append-only working record. Entries newest-first.

## 2026-11-10 — Independent audit of Slice 4 Batch 2 + 2.1; workspace reconciliation (Devin)

- Owner requested a read-only independent audit of the combined Batch 2
  (`71c2160`) and Batch 2.1 (`1bedca7`) changes. Audit executed against
  actual source and tests at HEAD `1bedca7`; test suite run independently
  (`go test -race -count=1 ./internal/...`, `bash scripts/test.sh` 6
  passed/2 skipped, build/vet/diff-check clean).
- Verdict: ACCEPT WITH CONDITIONS — no confirmed code defects. Conditions:
  tracker status reconciliation (done in this update) and two recorded
  observations: owner-side TTY verification still pending; UNKNOWN footnote
  less actionable in config-fallback mode (optional future UX polish, not
  a defect).
- Tracker updated: Batch 2.1 recorded as pushed at `1bedca7` and reviewed;
  audit verdict and observations recorded. Historical diary/audit entries
  preserved unchanged. Slice 4 and beta.8 remain open; no CHANGELOG entry;
  security finding untouched.

## 2026-11-10 — Slice 4 Batch 2.1 UX accuracy corrections (Devin)

- Owner reviewed pushed Batch 2 (`71c2160`) and approved two narrow
  corrections only: precise UNKNOWN/health-observation wording, and the
  `up` no-routes hint covering both static and live workflows.
- Correction 1: `routes add` success output, `routes add` help, `routes`
  help and the `printRoutes` UNKNOWN footnote now say a conclusive
  observation may come from proxied traffic or `routes --check`, and that
  not every proxied request produces a conclusive observation (client
  cancellations and certain post-connect failures are inconclusive per
  existing proxy semantics). No health implementation, probe, tracker,
  budget or timeout changes.
- Correction 2: `noRoutesHint()` in `internal/cli/up.go` now offers both
  workflows — static routes in `devtether.yaml` (requires a daemon restart)
  or an ephemeral live route on the running daemon via `devtether routes
  add <domain> <port>`. Startup behavior and empty-config tolerance
  unchanged.
- Tests: `TestRoutesAddCreatesLiveRoute` asserts the conclusive-observation
  wording and rejects "the first proxied request" phrasing;
  `TestPrintRoutesUnknownFootnote` pins "not every proxied request is
  conclusive"; new `TestRoutesHelpHealthWording` pins the help text;
  `TestNoRoutesHint` rewritten for both workflows and absence of the
  obsolete YAML-only restart instruction.
- Verification: focused CLI tests, `go test -race -count=1 ./internal/...`,
  `go build ./...`, `go vet ./...`, `bash scripts/test.sh` (6 passed, 2
  skipped), `git diff --check` — all clean. No commit/push per stop
  conditions.

## 2026-11-10 — Slice 4 Batch 2 CLI/proxy UX (Devin)

- Owner approved Batch 2 scope: proxy 404 guidance, `routes` health-status
  clarity, `routes add` feedback, add-side domain validation and duplicate
  error mapping, CLI help/exit-code accuracy. Health-probe behavior
  explicitly deferred; no IPC contract or status-code changes.
- `internal/proxy/handler.go`: 404 page no longer assumes YAML is the only
  route source; guidance now covers static (devtether.yaml, runtime removal
  returns on restart) and live (`devtether routes add`) routes.
- `internal/cli/routes.go`: `printRoutes` prints a one-line UNKNOWN
  footnote only when a route lacks a conclusive observation; `routes` Long
  help explains observational health (UNKNOWN ≠ OFFLINE, registration/DNS
  separate from backend health, live routes skip the startup check) and
  scopes exit codes to `--check`; `routes add` Long help notes the initial
  UNKNOWN state; `runRoutesAdd` validates the literal domain via
  `config.ValidateRouteDomain` before normalization/IPC (mirroring
  `routes remove`), normalizes before IPC, maps daemon 409 to a clear
  "already registered" error naming the domain, and its success output now
  states daemon-only scope, ephemerality, no YAML write, and the initial
  UNKNOWN health.
- Tests: new `TestRouteNotFoundPage` (proxy), `TestPrintRoutesUnknownFootnote`,
  `TestRoutesAddRejectsNonCanonicalDomain`, `TestRoutesAddAcceptsUppercaseHostname`;
  `TestRoutesAddCreatesLiveRoute` extended for the richer output and the
  mapped duplicate error.
- Verification: `go test -race -count=1 ./internal/...`, `go build ./...`,
  `go vet ./...`, `bash scripts/test.sh` (6 passed, 2 skipped), and
  `git diff --check` all clean. No commit/push per stop conditions.
- Out of scope (flagged, not changed): `up`'s `noRoutesHint` still says
  "Edit %s to add routes, then restart the daemon" (audit C7) — not part of
  the approved Batch 2 scope.

## 2026-11-10 — Slice 4 Batch 1 documentation consistency (Devin)

- Owner approved the Slice 4 read-only audit direction (verdict READY
  WITH CONDITIONS) and authorized Batch 1 only: documentation
  corrections, no code, no CHANGELOG entry, no release-status claims.
- Corrected stale "route changes require a restart" claims in `README.md`
  and `docs/PRD.md`; distinguished YAML edits (restart required) from
  runtime route mutation (no restart, no YAML write).
- `README.md`: added `routes add`/`routes remove`/`routes --check` to Core
  Commands and a new "Live routes" subsection (ephemeral vs static-removal
  semantics, `UNKNOWN` ≠ `OFFLINE` health note).
- `docs/PRD.md`: fixed the restart claim, added runtime-route clarification
  in §5, extended the current-commands list.
- `docs/architecture.md`: §5 router (static vs runtime mutation,
  incarnation), §6 IPC endpoint table (POST `/routes`, DELETE
  `/routes/{domain}?inc=N` with status codes; no YAML writes/hot reload),
  §7 CLI table (live-route operations), §9 health (incarnation scoping,
  no ticker, startup check covers static routes only, live routes start
  `unknown`, not every proxied request is conclusive).
- `docs/adr/005-yaml-config-schema.md`: reconciled 2026-11-10 — principle 4
  now distinguishes unsupported YAML live-reload/persistence from
  supported runtime-only route mutation; historical decision and rationale
  preserved (no new ADR; clarification follows the repo's Reconciled-date
  convention).
- Deferred per owner: CHANGELOG beta.8 entry, Batch 2 CLI/HTTP messaging,
  probe-on-add, GUI, release status. All changes uncommitted on
  `feature/devin`.

## 2026-11-10 — Slice 3 implemented, verified, published; closure pass (Devin)

- Owner authorized Slice 3 implementation against `0c955a4` (approved
  plan §3/§5/§6 as the contract). Implemented: exported
  `config.ValidateRouteDomain` (F1), daemon `POST /routes` (strict
  `json.Decoder.Token()` JSON — exact key set/casing, digits-only port
  with overflow-checked conversion, 8KB `MaxBytesReader` bound, nonce,
  server-side port range 1–65535) and `DELETE /routes/{domain}?inc=N`
  (canonical `[1-9][0-9]*` single-value grammar, RemoveRoute-first
  404/409 disambiguation, no unconditional path), additive
  `incarnation` on `RouteResponse`, client `AddRoute`/`RemoveRoute` with
  typed `APIError{StatusCode, Body}`, and CLI `routes add` /
  `routes remove [-y|--yes]` with type-specific warnings, TTY-gated
  confirmation via `cmd.InOrStdin()`, non-TTY fail-safe, and a 409
  re-confirm path that never silently retries.
- Published as `1dd37bb` ("feat(daemon): live route management over IPC
  with CLI subcommands", 8 files, +1550/−12) on owner instruction for
  external review. Verification: `go build`/`go vet` clean,
  `go test -race` all packages pass (new stress tests ×5), `bash
  scripts/test.sh` 6 passed / 2 skipped (golangci-lint + govulncheck not
  installed locally, pre-existing), and a real-socket smoke test
  covering add/duplicate/non-TTY refuse/`-y` remove of live+static /
  YAML byte-identical / restart semantics.
- Focused closure pass (uncommitted): `runRoutesRemove` now validates
  the literal argument with `config.ValidateRouteDomain` before
  `netutil.NormalizeHost` (regression tests reject `app.localhost:80`
  and `app.localhost.` without touching the engine); the incarnation
  number was removed from the `routes add` output (API field and
  conditional DELETE unchanged); stale `routes` Long help corrected
  (live routes are ephemeral; static removal is runtime-only).
- Pending human verification (TTY prompt): interactive y/n/empty/EOF
  accept/abort behavior — recorded in `audit_report.md` §1g. Slice 4
  (ADR-005, architecture.md, CHANGELOG, `noRoutesHint`) not started.

## 2026-11-10 — Slice 3 docs correction pass F1–F3 (Devin)

- Owner reviewed published docs commit `7ff7e67` against its parent and
  the actual source: Slice 3 design accepted, no architecture reopened;
  three documentation findings.
- F1: plan's DELETE step cited `config.ValidateRoute` but its signature
  is `ValidateRoute(domain string, port int) error` and DELETE has no
  port — resolved by owner-approved `config.ValidateRouteDomain(domain)`
  helper (Slice 3 file list gains `internal/config/config.go` +
  `internal/config/config_test.go`); no `ValidateRoute(domain, 1)`
  workaround.
- F2: POST `json.Number`→int conversion pinned — overflow-checked parse,
  400 on conversion failure or out-of-range port, never 500 for malformed
  client input; test matrix extended.
- F3: consolidated status map gains 500 for unexpected internal errors,
  distinct from 400/403/404/409/413.
- Documentation-only; no source/test/ADR changes; no commit/push.
  Implementation still awaits explicit owner approval.

## 2026-11-10 — Slice 3 pre-implementation audit accepted; design recorded (Devin)

- Read-only audit of the proposed Slice 3 (IPC + CLI) design against
  `feature/devin` @ `7c25ff5`: verdict READY WITH CONDITIONS, no confirmed
  defects (no Slice 3 code exists yet). Probe-verified ServeMux precedence,
  query parsing, and JSON decoding facts recorded in `audit_report.md` §1e.
- Owner accepted the audit and all seven conditions (C1–C7) plus
  clarifications, and decided: canonical `inc` grammar `[1-9][0-9]*` (400
  on zero/negative/empty/malformed/overflow/duplicate/leading-zero);
  strict POST JSON — exactly one object with exactly `domain`+`port`,
  exact casing, stdlib `json.Decoder.Token()` stream (no new dependency);
  CLI subcommands in `internal/cli/routes.go` (no `routes_mutate.go`).
- Documentation-only update (narrow owner-authorized exception to the
  AGENTS.md Native Tool Priority rule for this task — no native file
  editor exists in this Devin environment): `implementation_plan.md`
  §1/§5/§6/§7, `tracker.md`, `audit_report.md` §1e, this diary. No source,
  test, or ADR changes; no commit/push.
- Status: Slice 3 implementation NOT started — awaiting explicit owner
  approval. Slice 4 unchanged.

## 2026-11-07 — Slice 2 implemented, verified, published (Devin)

- Slice 2 (health incarnation integration) implemented per approved
  `implementation_plan.md` §7 and conditions C1–C5: `cur`/`hi` incarnation
  model in `internal/health` (`Register`/`Unregister`/`Observe`/
  `ObserveNow`, `Target.Incarnation`); proxy `ModifyResponse` target read and
  `ErrorHandler` target-read hoist via `targetCtxKey`; boot `Register` loop
  after `daemon.NewServer`, before binds/workers; `Incarnation` plumbed at
  both production `health.Target` construction sites (grep-verified: no
  third site); test fixtures registered across health/proxy/daemon/cli/stress.
- Verification: `go build ./...` + `go vet` clean; focused `go test -race`
  green (health/proxy/daemon/cli/dns/router/config); `bash scripts/test.sh`
  all checks passed (6 passed, 2 skipped — golangci-lint/govulncheck not
  installed locally, pre-existing). Independent verifier session
  `5cb39b240f3343939aca83d4d48f8c0e`: verdict **SOUND**, no confirmed
  defects, with four non-blocking notes.
- Published as `babaaf42` on `feature/devin` (fast-forward from `20ef459`).
- Independent review of `babaaf42` vs `20ef459` (fresh Devin read-only audit
  with clean-checkout verification, plus owner-commissioned external
  review): **no confirmed code defects**. Closure pass (this change,
  uncommitted): renamed `TestConcurrentRegisterUnregisterObserve` →
  `TestConcurrentUnregisterObserve` (it never raced `Register` calls) and
  added `TestConcurrentRegisterHighestIncarnationWins` +
  `TestHealthStaleIncarnationOfflineObservationRejected`; boot-registration
  integration-test gap assessed and rationale recorded in
  `audit_report.md` §1d; tracker/audit/diary status reconciled.
- Status: Slice 1 (`20ef459`) and Slice 2 (`babaaf42`) implemented, tested,
  independently verified, and pushed — pending owner acceptance. Slices 3–4
  NOT started; awaiting explicit owner approval.

## 2026-11-07 — Slice 1 corrective changes published (Devin)

- Owner reviewed `feature/devin` @ `20ef459` against `develop`, including the
  corrective diff: verdict **no confirmed code defects**.
- Publication: corrective commit `20ef459` pushed to `feature/devin`
  (fast-forward from `6540532`, no force-push); working tree clean after push.
- Status: Slice 1 (config + router) implemented, tested, independently
  verified, and pushed — pending owner acceptance. Slice 2 (health
  incarnations) NOT started; awaiting explicit owner approval.
- Documentation-only consistency update followed (tracker status, B8-2/B8-3
  findings status, this entry). No code, tests, or dependencies changed.

## 2026-11-01 — Slice 1 corrective review changes (Devin) — awaiting owner review

- Owner-commissioned corrective review of `6540532` (audit/change/Ponytail
  skills + full upstream/downstream trace): 2 confirmed findings fixed,
  1 ADR-007 question resolved without an ADR change.
- `internal/config/config.go`: `validateRoutes` rejects duplicate normalized
  route identities via `netutil.NormalizeHost` — error names both raw domains
  and the normalized identity; applies through `LoadConfig` (all callers:
  `up`, `routes`, `doctor`, `logs`); `ValidateRoute` stays literal/strict and
  the config acceptance set is unchanged.
- `internal/config/config_test.go`: case-variant duplicates (same and
  different ports) rejected via `LoadConfig`; exact-duplicate parser behavior
  pinned ("already defined"); new
  `TestValidateRoutesRejectsNormalizedDuplicates` matrix.
- `internal/router/router_test.go`:
  `TestEngine_StaleIncarnationCannotRemoveReplacement` — stale inc1 removal
  fails, inc2 replacement survives in `Resolve` + `GetAllRoutes`.
- Verification: `go build ./...` clean; focused config/router tests pass;
  `go test -race -count=1` green across config/router/cli/daemon/dns/proxy;
  `bash scripts/test.sh` all checks passed (govulncheck skipped as usual).
- Independent verification (session `a91220db08bb48d3848d975b3bd92fe8`,
  clean `6540532` checkout + diff): verdict **sound — no confirmed defects**;
  two low observations logged in `audit_report.md` §1c.
- No commit/push. Slice 2 (health incarnations) NOT started.

## 2026-11-01 — Slice 1 implemented (Devin) — awaiting owner review

- `internal/config/config.go`: extracted exported `ValidateRoute(domain, port)`;
  `validateRoutes` delegates with a `routes: %w` wrap — error text byte-identical.
- `internal/router/router.go`: `nextInc` counter; `ErrRouteExists`; atomic
  `AddRoute` → `(uint64, error)` (exists-check + insert under one write lock);
  `RemoveRoute(domain, expectedInc)` → `(uint64, bool)` — sole delete path,
  incarnation required, 0 rejected; `Incarnation` on `Target`/`RouteView`
  (copied in `Resolve` deep-copy and `GetAllRoutes`); `RouteLive` const.
- Call-site updates only where signatures forced them: `up.go` + 5 test files.
- New tests: router — unique/non-zero incarnations, duplicate rejection
  (incl. case-normalized), re-add gets higher incarnation, wrong/zero/missing
  incarnation removal rejected, 16-way concurrent create (exactly 1 success),
  16-way concurrent remove (exactly 1 ok), add/remove/lookup churn under
  `-race`; config — `TestValidateRoute` parity table vs `validateRoutes`.
- Verification: `go build ./...`, `go vet` clean; focused `go test -race` on
  router+config green; full `bash scripts/test.sh` green (all packages,
  race, lint, cross-compile, build; govulncheck skipped as usual).
- Observation (not a deviation): `TestEngine_Concurrency` now exercises
  duplicate rejection on repeat adds of `web.localhost` — still a valid race test.
- Independent verification (session `96ff0c8c1999428a9c8d56df58a760a0`,
  isolated checkout with the diff applied): verdict **sound — no confirmed
  defects**; race safety, atomicity, `errors.Is` wrapping, incarnation
  plumbing in both `Resolve` and `GetAllRoutes`, and test determinism all
  confirmed; build + focused race tests re-run green.
- No commit/push. Slice 2 (health incarnations) NOT started.

## 2026-11-01 — Slice 1 pre-mortem (Devin)

Brief pre-mortem before Slice 1 edits, verified against `develop` baseline:

- **S1-PM-1 (mechanical):** `AddRoute` gains a `(uint64, error)` return and
  `RemoveRoute` becomes `RemoveRoute(domain, expectedInc)` — ~16 call sites
  across 6 test files + `up.go` must be updated; the compiler catches every
  miss. No production `RemoveRoute` callers exist yet (IPC DELETE is slice 3).
- **S1-PM-2 (checked):** duplicate `AddRoute` now errors (`ErrRouteExists`).
  Verified no existing test or production path relies on silent overwrite —
  DNS/proxy/daemon tests each build a fresh engine; `up.go` iterates unique
  YAML map keys.
- **S1-PM-3:** `Incarnation` must be copied in BOTH `Resolve` (deep-copy
  path) and `GetAllRoutes` — a missed copy silently breaks slice-2 plumbing.
- **S1-PM-4:** `ValidateRoute` error text must remain byte-identical after
  the `routes: %w` wrap so existing `config_test.go` expectations pass.
- **Accepted:** the engine itself does not validate domain/port (that stays
  at the config/IPC layer per the plan) — preserves existing `AddRoute`
  behavior for tests using arbitrary ports.
- **Slice-1 scope guard:** no health tracker, IPC, CLI, or docs-code changes.

## 2026-11-01 — Final consistency pass: confirmation-for-all + required-incarnation delete (Devin)

- Owner refinements: confirmation required for EVERY `routes remove` (static
  and live), warning tailored to type; D15 tightened — `?inc=` is mandatory
  (missing → 400), no unconditional DELETE path exists anywhere.
- Source-of-truth language sharpened: `devtether.yaml` authoritative for
  persistent config; daemon router holds runtime state; no write-back.
- All five artifacts reconciled; consistency audit clean. Workspace ready
  for Slice 1 — awaiting owner approval.

## 2026-11-01 — Owner finalized remove semantics (Devin)

- Owner ruling D14 (supersedes D13): `routes remove` applies to static AND
  live routes, runtime-only, never writes `devtether.yaml`. Confirmation is required for EVERY remove (warning tailored to type)
  (static: YAML unchanged, returns after restart; live: ephemeral, must be
  re-added); `-y`/`--yes` bypasses; no `--force`; non-TTY without `-y`
  fails safe.
- New race in the confirmation window closed by D15: `GET /routes` exposes
  an opaque `incarnation` (single justified D12 exception) so the CLI can
  issue `DELETE /routes/{domain}?inc=N` — mismatch → 409, concurrent
  replacement survives.
- `discussion_space.md` gained a persistent-vs-ephemeral section (semantics
  table, five confusing cases, trade-offs/rationale). Plan, tracker, audit
  report reconciled; post-design consistency audit clean.
- Awaiting owner approval for Slice 1.

## 2026-11-01 — Pre-mortem recorded (Devin)

- Independent pre-mortem completed by isolated session
  `devin-642d54ff3e2b40a18d84e21aa6fcb2bb`. Verdict: **sound to implement** —
  tombstone model held under every constructed interleaving.
- Recorded in `audit_report.md` §1: PM-1 (ADR-005 + `routes` Long doc updates
  mandatory), PM-2 (`Incarnation` plumbing at both `routeTargets` +
  `startupTargets`), PM-3 (static-DELETE gap → proposed D13), PM-4/5/6
  accepted risks, plus the verified-clean list.
- `implementation_plan.md` §7 now defines 4 slices: config+router → health
  incarnations → IPC+CLI → mandatory docs. Slice 1 awaits owner approval.
- ~~Open decision D13 (DELETE on static routes → proposed 409) needs owner
  sign-off before slice 3.~~ Resolved by owner ruling D14 (2026-11-01,
  above).

## 2026-11-01 — beta.7 closeout + beta.8 workspace creation (Devin)

- Owner confirmed completion of beta.7 Phase 4 Polish manual verification.
- beta.7 tracker: Phase 4 Polish task closed, release `Status: Complete`,
  closeout note recorded with verification date.
- `docs/workspace/README.md` §2: Current → `v2.0.0-beta.8`, Previous →
  `v2.0.0-beta.7`, Next → TBD.
- Created `docs/workspace/v2.0.0-beta.8/` per workspace anatomy §4:
  `tracker.md` + `artifacts/{implementation_plan,discussion_space,audit_report,agent_diary}.md`.
- Plan provenance: five owner-reviewed revisions of the live-route design
  (external working doc) migrated into `implementation_plan.md` and
  `discussion_space.md`. Design baseline `develop` @ `6fac500`; `develop` is
  `main` + `2b6a75f` (proxy test strengthening, PR #14) + `6fac500` (on-demand
  route health, PR #15) — both prerequisites of this feature.
- Repo-hygiene finding recorded: `scripts/test.sh` committed mode 100644;
  `make test` cannot exec it; `bash scripts/test.sh` locally.
- Next: independent pre-mortem of the design → first implementation slice
  proposal → owner approval gate before any production code.
- No commits, pushes, PRs, or release-completion claims yet.
