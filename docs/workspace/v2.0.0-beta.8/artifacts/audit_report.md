# Audit Report — v2.0.0-beta.8

Append-only verification record per `devtether-audit` conventions.

## §1 Pre-mortem — Live Route Management design (rev 5)

Independent review by an isolated Devin session
(`devin-642d54ff3e2b40a18d84e21aa6fcb2bb`, 2026-11-01) per devtether-change §2
and devtether-audit §1–§5. Reviewer traced every claimed interleaving against
actual `router.Engine`, `health.Tracker`, proxy ctx plumbing, and daemon
handlers on `develop`.

### Verdict: **Sound to implement**

The incarnation/tombstone mechanism is correct under every interleaving the
reviewer constructed and is not overengineered — the naive simpler versions
(forget-on-mutate, per-domain gen-floor) demonstrably fail.

### Confirmed defects / mandatory gaps

- **PM-1 (medium):** ADR-005 design principle 4 ("route changes require a
  restart") and the `routes` command `Long` text ("live reload via IPC is not
  available in this release") directly contradict this feature. Doc/ADR updates
  are MANDATORY, not optional — plan §1 step 7 amended accordingly.
- **PM-2 (high if missed, trivial fix):** `health.Target.Incarnation` must be
  plumbed at BOTH `health.Target` construction sites — `daemon.routeTargets()`
  AND `cli/up.go startupTargets()` — or every startup-check/`CheckAll`
  `Observe` is rejected by `inc==0` and all routes silently stay `unknown`.
  Made an explicit requirement in plan §3.
- **PM-3 (medium) → RESOLVED by owner ruling D14 (supersedes D13):** DELETE
  applies to static AND live routes, runtime-only, never writes YAML.
  Confirmation required for EVERY remove (warning tailored to type);
  `-y/--yes` bypasses; non-TTY without `-y` fails safe. Residual
  race found inside the confirmation window itself (confirm inc10 →
  concurrent re-add inc11 → unconditional DELETE would kill the replacement):
  closed by D15 — incarnation-aware delete is REQUIRED (`?inc=` mandatory,
  missing → 400; no unconditional path), backed by an opaque `incarnation`
  field in `GET /routes`.

### Credible risks (accepted, documented)

- **PM-4 (low):** `hi` is an unbounded tombstone map (never evicted). Accepted:
  bounded by user-driven route churn over a daemon's lifetime; documented in
  plan §3 rather than adding eviction machinery (Ponytail).
- **PM-5 (low, inherent):** observation-drop window between engine mutation
  and tracker op — a request resolving a new route before `Register` gets its
  `Observe` rejected; status heals on next traffic. Inherent to two-call-site
  ordering; a code comment, not a fix.
- **PM-6 (low):** call-site coupling — `AddRoute`/`RemoveRoute` must be paired
  with `Register`/`Unregister` everywhere (incl. ~15 test call sites and
  `up.go`) or that route's health goes permanently silent. Mechanical; covered
  by signature-change compilation + tests.

### Verified clean (insufficient-evidence items resolved)

- Interleavings: add→delete→re-add, delete-racing-register, unregister-racing-
  register both ways, concurrent same-domain POSTs/DELETEs — all clean. The
  `hi` tombstone is load-bearing: add→remove-before-register→late-Register is
  exactly what it blocks.
- `inc==0` unreachable in production once callers pair `Register`.
- Proxy plumbing confirmed: `ModifyResponse`/`ErrorHandler` already read
  `targetCtxKey`; `target.Incarnation` is available at both call sites.
- Boot race unfounded: IPC `Serve` starts after static registration.
- DNS picks up live routes automatically (per-query resolve → correct
  NXDOMAIN→A transition). Nonce/strict-JSON/`ValidateRoute` reuse consistent
  with ADR-003 parity.

### Post-design consistency audit (2026-11-01, final pass)

Re-audit after owner rulings D14–D15 (final form). Checks run: semantics
tables identical in meaning across tracker/plan/discussion; confirmation
required for every remove in all three (plan file list, test matrix,
tracker description); `incarnation` exposure justified only by D15 (no
other user-facing surface); D15 leaves no unconditional DELETE path —
missing `inc` → 400, and the engine exposes a single
`RemoveRoute(domain, expectedInc)` signature; D13 marked superseded, not
deleted (history preserved); conditional delete composes with the
tombstone model (`Unregister(domain, removedInc)` runs regardless, so a
delayed `Register` cannot resurrect a removed incarnation). No
contradictions found. **Workspace declared ready for Slice 1; awaiting
owner approval.**

## §1b Slice 1 independent verification (2026-11-01)

Isolated session `96ff0c8c1999428a9c8d56df58a760a0` audited the Slice-1 diff
(config + router) against `implementation_plan.md` §4/§6 and the skills.
Verdict: **sound — no confirmed defects**; ready for owner review. Build and
focused `-race` tests re-run green in the audit checkout.

## §1c Slice 1 corrective review (2026-11-01)

Owner-commissioned focused review of Slice 1 at `6540532`. Method: full
upstream/downstream trace (`LoadConfig` callers `up`/`routes`/`doctor`/
`logs`; consumers DNS/proxy/daemon IPC), ADR-007 reconciliation, skills per
`devtether-audit`. Three findings: two fixed in the working tree
(uncommitted), one resolved as no-change.

### Findings

- **S1-C1 (medium, confirmed defect):** case-variant duplicate route domains
  (e.g. `App.localhost` + `app.localhost`, any ports) passed `LoadConfig` —
  `ValidateRoute` matches its regex on the lowercased form and yaml.v3 map
  keys are case-sensitive — but `up.go`'s `AddRoute` loop then failed on the
  second entry with `ErrRouteExists`, killing startup after config validation
  had passed. Pre-Slice-1 this was a silent overwrite; Slice 1 made it a loud
  but late failure.
  **Fix:** `validateRoutes` rejects duplicate normalized identities using
  `netutil.NormalizeHost(domain)` (ADR-007 centralized normalizer, no
  hand-rolled logic); the error names both raw forms and the normalized
  identity. Applies through `LoadConfig`, covering all callers. Strict
  semantics unchanged: `ValidateRoute` still validates literally (trailing
  dot, whitespace, wrong TLD, wildcards still rejected, error text
  byte-identical), and validated domains normalize to lowercase only, so the
  acceptance set is unchanged.
- **S1-C2 (low, test gap):** no direct test for create → remove → recreate →
  stale-incarnation removal. **Fix:**
  `TestEngine_StaleIncarnationCannotRemoveReplacement` proves the inc2
  replacement survives a stale inc1 removal via both `Resolve` and
  `GetAllRoutes`.
- **S1-C3 (resolved, no change):** ADR-007 question — `ValidateRoute` does
  not (and must not) call `NormalizeHost`: it validates literal config
  input, and normalizing first would accept trailing-dot domains that
  `TestRouteDomainPolicy` pins as rejected. ADR-007's producer/consumer
  boundary rule is satisfied by `AddRoute`/`RemoveRoute`/DNS/proxy; the
  config boundary now uses the centralized normalizer for duplicate identity.
  No ADR amendment needed; options "normalize keys at load" (changes strict
  semantics) and "ADR exemption" rejected.

### Verification evidence

- `go build ./...` clean; focused config/router tests pass.
- `go test -race -count=1` green: config, router, cli, daemon, dns, proxy.
- `bash scripts/test.sh`: all checks passed (govulncheck skipped — not
  installed locally, pre-existing).
- Independent verification session `a91220db08bb48d3848d975b3bd92fe8` (diff
  applied to a clean `6540532` checkout): verdict **sound — no confirmed
  defects**; all three scope items met; explicit checks (a)–(i) pass;
  `-count=20` reruns of the new tests pass.

### Remaining risks / observations (from independent verification)

- Duplicate diagnostics with 3+ normalized-equivalent domains name only the
  first colliding pair (Go map iteration order); the config is still
  rejected and the error always carries the normalized identity plus both
  raw forms. Low impact; no fix planned (deterministic ordering would add
  sorting machinery for a cosmetic gain — Ponytail).
- `devtether logs` ignores `LoadConfig` errors and falls back to the default
  log path (`resolveLogFile`), so a config newly rejected for duplicates can
  make `logs` ignore `settings.log_path`. Pre-existing invalid-config
  fallback pattern; such a config cannot run the daemon anyway. Not a
  defect.

## §1d Slice 2 review and closure pass (2026-11-07)

Slice 2 (health incarnation integration) implemented and pushed at `babaaf4`.

### Independent reviews

- **Devin read-only audit** of `babaaf4` vs `20ef459` (fresh review, not
  relying on the implementation-time audit): verdict **READY WITH CONDITIONS**
  — no confirmed code defects; the condition was workspace status
  reconciliation. Verification independently re-run in a clean checkout:
  `go build ./...`, `go vet`, `go test -race -count=1` across
  health/proxy/daemon/cli/dns/router/config, and `bash scripts/test.sh`
  (6 passed, 2 skipped — linters not installed locally, pre-existing); new
  tests additionally rerun repeatedly (`-count=5`/`-count=10`, `-race`).
- **Owner-commissioned external review** of `babaaf42`: production
  implementation sound, no confirmed code defects. Closure items raised:
  concurrent-`Register` test coverage, offline `ErrorHandler` stale-
  incarnation end-to-end test, boot-registration integration-test
  assessment, and workspace status reconciliation — all addressed below.

### Closure items (this change, uncommitted)

- `TestConcurrentRegisterUnregisterObserve` renamed to
  `TestConcurrentUnregisterObserve` — it raced observations against an
  unregister but never exercised concurrent `Register` calls. New
  `TestConcurrentRegisterHighestIncarnationWins` releases concurrent
  `Register` calls for incarnations 1–8 plus a duplicate together and proves
  the highest incarnation wins regardless of arrival order, confirmed at the
  observation level (incarnation 7 rejected, 8 recorded).
- New `TestHealthStaleIncarnationOfflineObservationRejected` (proxy): a
  channel-gated transport blocks mid-dial; the route is swapped inc1→inc2
  while the request is in flight; the old incarnation's dial failure is
  rejected by the tracker (status stays `unknown`, 502 returned), and
  a post-swap request records `online` against inc2. Closes the offline
  `ErrorHandler` stale-incarnation coverage gap.
- **Boot-registration integration test: not added (accepted).** Rationale:
  `runUp` has no existing test harness and a test would need to bind ports,
  the daemon socket, and the readiness pipe — fragile and a new abstraction
  for marginal gain. The `Register` loop is a four-line block immediately
  after `daemon.NewServer` and before binds/workers (verified by inspection);
  incarnation propagation is pinned by
  `TestStartupTargetsCarryIncarnation`/`TestRouteTargetsCarryIncarnation`;
  `Register`/`Observe` semantics are fully unit-tested. Sufficient for this
  slice; revisit if `up.go` startup ordering grows more complex.

### Verification evidence (closure pass, 2026-11-07)

- `go test -race -count=10 ./internal/health/ -run 'TestConcurrentRegisterHighestIncarnationWins|TestConcurrentUnregisterObserve'` — ok.
- `go test -race -count=5 ./internal/proxy/ -run 'TestHealthStaleIncarnation'` — ok (both online and offline stale-incarnation tests).
- `go test -race -count=1` across health/proxy/daemon/cli/dns/router/config — all ok.
- `bash scripts/test.sh` — all checks passed (6 passed, 2 skipped: golangci-lint/govulncheck not installed locally, pre-existing).
- No production-code changes in this pass; beta.7 assertions untouched.

### Remaining test gaps / risks (accepted, not defects)

- Interleaving tests A/B are tracker-level scripted incarnations; handler-level
  remove→re-add convergence tests are Slice 3 scope (plan §7).
- `hi` tombstone map is unbounded (PM-4, accepted in §1).
- Observation-drop window between engine mutation and tracker op (PM-5,
  accepted in §1).

## §1e Slice 3 pre-implementation audit (2026-11-10)

Slice 3 (IPC + CLI) audited read-only against `feature/devin` @ `7c25ff5`
before any code was written. No Slice 3 code exists; the verdict covers
the approved design only.

### Verdict: READY WITH CONDITIONS — no confirmed defects

All proposed mechanics compose with existing primitives: the nonce pattern
(`handleShutdown`), `http.MaxBytesReader` bounding, strict JSON decoding,
`config.ValidateRoute`, atomic `router.AddRoute`/`RemoveRoute`, and
incarnation-scoped `tracker.Register`/`Unregister`. `cli/tty.go isTTY` has
no production consumer until Slice 3 — expected, not a defect.

### Conditions C1–C7 (owner-accepted) + clarifications

- C1: POST port range 1–65535 enforced server-side via `ValidateRoute` →
  400 (`router.AddRoute` does not range-check).
- C2: DELETE order pinned — `RemoveRoute(domain, inc)` first; only on
  `ok=false` does `Resolve` split 409 (exists, wrong inc) from 404
  (absent).
- C3: GET /routes two-snapshot contract documented as accepted limitation;
  `incarnation` is engine-snapshot-authoritative for conditional DELETE;
  tracker API not expanded.
- C4: handler tests must set a non-empty `s.nonce` explicitly.
- C5: `/routes/check` ServeMux precedence over `/routes/{domain}`
  documented and tested.
- C6: CLI remove is daemon-only — `ListRoutes` supplies type +
  incarnation, conditional DELETE, no config fallback; 409 → "route
  changed — re-confirm", never silent re-fetch + retry.
- C7: confirmation reads `cmd.InOrStdin()`; decline/EOF abort; non-TTY
  without `-y` fails safe; `-y` skips confirmation only; TTY accept/
  decline/EOF recorded as human verification.
- Owner clarifications folded in: strict `inc` grammar `[1-9][0-9]*`
  (canonical positive decimal; zero/negative/empty/malformed/overflow/
  duplicate/leading-zero → 400); strict POST JSON — exactly one object,
  exactly `domain`+`port` keys, exact casing, duplicates/case-variants/
  unknown/null/non-object/trailing → 400, stdlib-only
  `json.Decoder.Token()` stream parse (no new dependency); CLI
  subcommands in `internal/cli/routes.go` (no `routes_mutate.go`).

### Probe-verified platform facts (Go 1.27.3, scratch module, deleted after use)

- `?inc=1&inc=2` yields two values; `Query().Get` would silently take the
  first — the handler must require exactly one value.
- `strconv.ParseUint(v,10,64)` rejects empty/negative/`+1`/whitespace/hex/
  float/exponent/overflow but accepts `0` and leading zeros — hence the
  explicit `[1-9][0-9]*` grammar check before parsing.
- ServeMux: `DELETE /routes/check` routes to the check handler (→405);
  `/routes/`, `/routes/check/`, and deeper paths → 404; `{domain}` arrives
  percent-decoded and un-normalized → `ValidateRoute` before engine calls.
- Struct-based `encoding/json` decoding cannot express the approved
  strictness (case-insensitive keys, duplicate last-wins, `null` → zero
  struct), so the Token()-stream approach in plan §5 is required, not
  optional.

### File scope (approved)

Production: `internal/daemon/daemon.go`, `internal/daemon/client.go`,
`internal/cli/routes.go`. Tests: `internal/daemon/daemon_test.go`,
`internal/daemon/stress_test.go`, `internal/cli/routes_test.go`.
(Superseded by §1f F1: `internal/config/config.go` +
`internal/config/config_test.go` added.)

### Remaining implementation questions

None blocking. TTY prompt accept/decline/EOF is human verification, not a
unit test. The two-snapshot GET /routes contract is documented, not fixed.

## §1f Slice 3 documentation correction pass (2026-11-10)

Owner review of the published docs commit `7ff7e67` (against its parent,
the discussion record, and the actual source) accepted the Slice 3
design — no architecture reopened — and raised three documentation
findings, all resolved in `implementation_plan.md`. File lists reconciled
across tracker, plan, and this report.

### Findings and resolutions

- **F1 (contract error in plan text):** the DELETE handler step cited
  `config.ValidateRoute` for the path domain, but the actual signature is
  `ValidateRoute(domain string, port int) error` and DELETE has no port.
  Resolution (owner-approved): Slice 3 adds exported
  `config.ValidateRouteDomain(domain string) error`; `ValidateRoute` is
  refactored to reuse it and then apply its existing port check; POST
  keeps `ValidateRoute(domain, port)`; DELETE uses
  `ValidateRouteDomain(domain)`. Domain policy and error behavior are
  exactly preserved; no `ValidateRoute(domain, 1)` workaround. Slice 3
  file list gains `internal/config/config.go` and
  `internal/config/config_test.go`, with focused shared-helper and
  invalid-DELETE-domain tests.
- **F2 (underspecified conversion):** POST `json.Number` → integer
  conversion is now pinned: digits-only form enforced by the strict
  decode; overflow-checked parse (`strconv.Atoi`); conversion failure or
  port outside 1–65535 → 400; malformed/overflowing client input is never
  500. Test matrix extended with overflow/float/exponent cases.
- **F3 (incomplete status map):** POST step 6 specified 500 for
  unexpected `AddRoute` errors but the consolidated status map omitted
  it. The map now lists 500 as an internal-error response, distinct from
  validation (400), authentication (403), and duplicate-route (409)
  errors.

### Verification

- Revised plan re-checked against actual signatures:
  `config.ValidateRoute(domain string, port int) error`,
  `router.AddRoute(domain, serviceName string, port int, routeType RouteType) (uint64, error)`,
  `router.RemoveRoute(domain string, expectedInc uint64) (uint64, bool)`.
- Only the four beta.8 artifacts touched; `git diff --check` clean.

## §1g Slice 3 implementation and closure pass (2026-11-10)

Slice 3 (IPC + CLI) implemented per the owner-approved design
(`implementation_plan.md` §5–§7) and published at `1dd37bb`
("feat(daemon): live route management over IPC with CLI subcommands",
8 files, +1550/−12). A focused closure pass followed; its corrections
are uncommitted on `feature/devin`.

### Confirmed findings and fixes (closure pass)

- **CLI literal-domain validation missing:** `runRoutesRemove`
  normalized the argument before validation, so `NormalizeHost` silently
  repaired `app.localhost:80` (port stripped) and `app.localhost.`
  (trailing dot stripped) into valid lookups. Fixed:
  `config.ValidateRouteDomain(args[0])` runs before normalization and
  before any IPC; regression tests prove both forms are rejected and the
  engine is untouched. `NormalizeHost` itself unchanged; uppercase domains
  still work.
- **Incarnation leaked into CLI output:** the `routes add` success
  message printed the incarnation number. Removed; the API field and
  conditional-delete behavior are unchanged, and a CLI test asserts the
  token stays opaque.
- **Stale `routes` help:** the Long text still claimed route changes
  require a daemon restart and live reload is unavailable. Replaced with
  the ephemeral-live vs runtime-only-static distinction. Wider Slice 4
  doc updates (ADR-005, architecture.md, CHANGELOG, `noRoutesHint`)
  remain out of scope.

### Implementation deviations from plan §5 (accepted)

- POST reads the bounded body fully before Token()-stream decoding, so
  any >8KB body is 413 regardless of where a strictness violation
  appears (plan described stream decoding; same status contract, stricter
  ordering guarantee).
- DELETE validates the literal path domain, then keys engine/tracker by
  `netutil.NormalizeHost(pathDomain)` — `Engine.Resolve` is
  case-sensitive, so without this a mixed-case path domain returned a
  spurious 404.
- `newStack` test harness waits for `Serve` cleanup before test exit
  (test-only; prevents cross-test nonce-file deletion).

### Accepted limitations

- Two-snapshot GET /routes contract unchanged (C3): `incarnation` is
  authoritative for conditional DELETE; health fields observational.
- HTTP 500 from `AddRoute` is unreachable from validated client input;
  no dedicated test.
- `routes --check` with zero routes exits 0 (pre-existing behavior).
- Full list-then-delete stale-confirmation race is covered at handler
  and client level; the CLI-level interleaving needs a TTY or a seam and
  is covered by the 409-mapping unit test instead.

### Verification evidence (actual, 2026-11-10)

- `go build ./...`, `go vet ./...`: pass.
- `go test -race -count=1 ./internal/...`: all 8 packages pass; new
  daemon stress tests re-run `-race -count=5`.
- `bash scripts/test.sh`: 6 passed, 2 skipped (golangci-lint and
  govulncheck not installed locally — pre-existing).
- Real-socket smoke test: add → success + ephemeral notice; duplicate →
  409; non-integer port → local error; non-TTY remove → refused exit 1;
  `-y` remove works for live AND static; `devtether.yaml` byte-identical;
  live route gone after restart, static route returns; `down`/`status`
  exit codes unchanged.
- Beta.7 assertions untouched (only removed test line is the replaced
  `newStack` Serve launch).

### Outstanding human verification (TTY prompt)

- Interactive `y` accepts and removes.
- `n` or empty input aborts without mutation.
- EOF aborts without mutation.
- Non-TTY without `-y` fails safely (unit-covered; confirm interactively).
- `-y` skips the prompt but still performs the normal lookup and
  conditional DELETE.

## §1h Slice 4 Batch 1 documentation consistency (2026-11-10)

Owner reviewed the Slice 4 read-only audit (READY WITH CONDITIONS) and
authorized Batch 1 only: documentation corrections. No Go source, tests,
CLI messages, proxy pages, CHANGELOG entry, or release-status changes.
ADR-005 was clarified (Reconciled 2026-11-10) rather than superseded,
following the repository's accepted-ADR reconciliation convention.

### Corrections applied

- `README.md`: stale restart claim replaced; Core Commands extended with
  `routes add`/`remove`/`--check`; new "Live routes" subsection covering
  ephemeral vs static-removal semantics and the `UNKNOWN` health note.
- `docs/PRD.md`: restart claim corrected; §5 runtime-route clarification;
  current-commands list extended.
- `docs/architecture.md`: §5 (static vs runtime mutation, incarnation),
  §6 (POST `/routes` and DELETE `/routes/{domain}?inc=N` with status
  codes; configuration-file writes and YAML hot reload explicitly
  unsupported), §7 (live-route CLI operations), §9 (incarnation-scoped
  observations, no periodic polling, startup check covers static routes
  only, live routes start `unknown`, `unknown` ≠ `offline`, not every
  proxied request is conclusive).
- `docs/adr/005-yaml-config-schema.md`: principle 4 amended to separate
  unsupported YAML live-reload/persistence from supported runtime-only
  route mutation; consequences note added; historical decision preserved.

### Verification

- Every edited section re-checked against the implementation at
  `a0c257f`: endpoint methods/paths/status codes, nonce requirements,
  strict JSON shape, incarnation semantics, health observation sources,
  and startup-check scope.
- Repo-wide search for remaining "route changes require a restart" style
  claims: only YAML-scoped statements remain (accurate).
- `git diff --check` clean; Markdown tables and links verified.

### Outstanding

- Batch 2 (CLI/HTTP messaging: proxy 404 hint, `noRoutesHint`, `routes`
  health legend) — not started, awaiting owner approval.
- Probe-on-add behavior — deferred by owner.
- CHANGELOG beta.8 entry — deferred to release gate.
- TTY human verification from §1g — still pending.

## §1i Slice 4 Batch 2 CLI/proxy UX (2026-11-10)

Owner approved Batch 2 scope only: proxy 404 guidance, `routes` health
clarity, `routes add` feedback, add-side validation and duplicate error,
help/exit-code accuracy. Health-probe behavior, IPC contracts and status
codes unchanged. No GUI work.

### Corrections applied

- `internal/proxy/handler.go` (404 page): guidance now distinguishes static
  routes (devtether.yaml; runtime-removed static routes return on restart)
  from live routes (`devtether routes add`); no longer YAML-only.
- `internal/cli/routes.go`:
  - `printRoutes`: one-line UNKNOWN footnote, only when a route lacks a
    conclusive observation; no per-row noise.
  - `routes` Long help: observational health semantics (UNKNOWN = "not yet
    checked", not OFFLINE; registration/DNS separate from backend health;
    live routes skip the startup check; observations from proxied traffic
    or `--check`); exit codes scoped to `--check`.
  - `routes add` Long help: initial UNKNOWN health; adding a route does not
    verify the backend.
  - `runRoutesAdd`: literal domain validation via `config.ValidateRouteDomain`
    before normalization/IPC (parity with `routes remove`); duplicate 409
    mapped to a clear domain-identifying error; success output states
    daemon-only scope, ephemerality, no YAML persistence, initial UNKNOWN.
- Tests added/updated: `TestRouteNotFoundPage`,
  `TestPrintRoutesUnknownFootnote`, `TestRoutesAddRejectsNonCanonicalDomain`,
  `TestRoutesAddAcceptsUppercaseHostname`; `TestRoutesAddCreatesLiveRoute`
  extended.

### Verification

- `go test -race -count=1 ./internal/...` — pass.
- `go build ./...`, `go vet ./...` — pass.
- `bash scripts/test.sh` — all checks passed (6 passed, 2 skipped).
- `git diff --check` — clean. gofmt clean on touched files.
- Self-audit: footnote wording tightened to "Further observations" so the
  startup check is not contradicted; uppercase hostname behavior pinned by
  test; incarnation identifiers remain opaque in CLI output.

### Outstanding / out of scope

- `up`'s `noRoutesHint` (audit C7) still recommends editing YAML + restart
  only — flagged as outside the approved Batch 2 scope; awaits owner
  decision.
- Probe-on-add, CHANGELOG beta.8 entry, release status — deferred.
- TTY human verification from §1g — still pending.

## §1j Slice 4 Batch 2.1 UX accuracy corrections (2026-11-10)

Owner reviewed pushed Batch 2 (`71c2160`) and authorized two narrow
corrections: health-observation wording precision and the `up` no-routes
hint. No other UX, IPC, health, or startup-behavior changes.

### Corrections applied

- Health wording (internal/cli/routes.go): `routes add` success output,
  `routes add` help, `routes` help, and the `printRoutes` UNKNOWN footnote
  now state that a conclusive observation may come from proxied traffic or
  `devtether routes --check`, and that not every proxied request produces a
  conclusive observation. UNKNOWN still means "not yet checked" and is not
  OFFLINE; adding a route still does not verify the backend.
- `noRoutesHint()` (internal/cli/up.go): now presents both workflows —
  static routes in `devtether.yaml` (requires a daemon restart) and
  ephemeral live routes via `devtether routes add <domain> <port>` on the
  running daemon. Empty configuration remains non-fatal; daemon startup
  behavior unchanged.

### Tests

- Updated: `TestRoutesAddCreatesLiveRoute` (conclusive-observation wording,
  rejects "first proxied request" implication),
  `TestPrintRoutesUnknownFootnote` (pins "not every proxied request is
  conclusive"), `TestNoRoutesHint` (both workflows, obsolete YAML-only
  restart instruction absent).
- Added: `TestRoutesHelpHealthWording` (help text precision).

### Verification

- Focused CLI tests pass; `go test -race -count=1 ./internal/...` pass;
  `go build ./...` and `go vet ./...` pass; `bash scripts/test.sh` all
  checks passed (6 passed, 2 skipped); `git diff --check` clean; gofmt
  clean on touched files.
- Self-audit: complete diff reviewed against the two approved corrections —
  no unrelated changes; no health implementation, IPC, HTTP, GUI, or
  dependency changes.

### Outstanding

- Probe-on-add, CHANGELOG beta.8 entry, release status, pending security
  finding — all deferred/awaiting owner. Slice 4 and beta.8 remain open.
- TTY human verification from §1g — still pending.

## §1k Independent audit of Slice 4 Batch 2 + Batch 2.1 (2026-11-10)

Read-only audit of the combined Batch 2 (`71c2160`) and Batch 2.1
(`1bedca7`) changes on `feature/devin`, against actual source and tests.

### Baseline

- HEAD `1bedca71be828953c482d64f126462b1ef1eccfb`, clean tree.
- Combined diff vs `c84acbd`: 9 files, +359/−18; only CLI/proxy message,
  validation, help, and test changes plus workspace records. No daemon,
  health, config, router, DNS, or IPC changes.

### Verdict: ACCEPT WITH CONDITIONS

No confirmed code defects. Proxy 404 guidance, health-observation wording,
live-route creation feedback, add-side validation and duplicate error,
startup no-routes guidance, and tests all verified against source.
Regression surfaces (removal confirmation, non-TTY fail-safe,
stale-incarnation protection, static-config immutability, health tracker
semantics, CLI fallback) untouched.

### Verification (executed during the audit, not reported by the
implementation session)

- `go test -race -count=1 ./internal/...` — pass.
- `bash scripts/test.sh` — all checks passed (6 passed, 2 skipped).
- `go build ./...`, `go vet ./...`, `git diff --check` — pass/clean.

### Conditions and observations

1. Tracker status reconciliation (this update): Batch 2.1 was recorded as
   uncommitted/pending review before the push; now recorded as pushed at
   `1bedca7` and reviewed.
2. Owner-side TTY verification (from §1g) remains pending.
3. The `printRoutes` UNKNOWN footnote also appears in config-fallback mode
   (daemon unavailable), where proxied traffic and `--check` cannot
   currently produce observations. The wording ("may come from") is not
   false; recorded as optional future UX polish, not a defect and not a
   requirement for this batch.

Slice 4 and beta.8 remain open; no CHANGELOG entry; the separately pending
security finding is untouched.

## §2 Findings log

| ID | Severity | Location | Finding | Status |
|---|---|---|---|---|
| B8-1 | Low | `scripts/test.sh` | Committed mode 100644; `make test` cannot exec it. Local runs use `bash scripts/test.sh`; CI unaffected (calls `go test` directly). One-line `chmod +x` fix — tracked in tracker as separate hygiene item. | Open |
| B8-2 | Medium | `internal/config/config.go` `validateRoutes` | Case-variant duplicate route domains passed `LoadConfig` but failed daemon startup with `ErrRouteExists` | Fixed and pushed at `20ef459` — normalized-duplicate guard via `netutil.NormalizeHost` + regression tests; see §1c |
| B8-3 | Low | `internal/router/router_test.go` | Missing direct test for create→remove→recreate→stale-incarnation removal survival | Fixed and pushed at `20ef459` — `TestEngine_StaleIncarnationCannotRemoveReplacement`; see §1c |
