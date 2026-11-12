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
2. Owner-side TTY verification (from §1g) remains pending. *(Superseded 2026-11-11: owner confirmed all normal interactive TTY flows pass on Zorin OS 18; abnormal-case QA deferred to final pre-release QA.)*
3. The `printRoutes` UNKNOWN footnote also appears in config-fallback mode
   (daemon unavailable), where proxied traffic and `--check` cannot
   currently produce observations. The wording ("may come from") is not
   false; recorded as optional future UX polish, not a defect and not a
   requirement for this batch.

Slice 4 and beta.8 remain open; no CHANGELOG entry; the separately pending
security finding is untouched.

## §1l Detached daemon log safe-open remediation (2026-11-10)

### Pre-mortem (design review before implementation)

Finding (Devin Security, medium): `daemonize()` in `internal/cli/up.go`
opened `<logDir>/devtether.log` with `os.OpenFile(..., O_CREATE|O_WRONLY|
O_APPEND, 0644)` after `os.MkdirAll(logDir, 0700)`. Both calls follow
symlinks, so a malicious repository could pre-place `.logs/devtether.log`
as a symlink (or `.logs` itself as a symlink) and redirect detached daemon
stdout/stderr — including access-log lines carrying request host/path/query
— into an attacker-chosen file such as a shell startup file. The same
unsafe path was reopened by `lastStartupError()` and `devtether logs`.

Confirmed against `develop` @ `f897117`: the write path, the read paths,
and the CR/LF-only access-log sanitizer (`internal/proxy/middleware.go`)
with raw `log.Println` output (`internal/logger/logger.go`) are all present
as described. The command-execution chain remains conditional (target must
be writable, daemon must emit logs, a client request must arrive, and the
target must later be interpreted by a shell); the arbitrary-file-write
primitive itself is confirmed.

Approved design (owner, this session): one CLI-local safe-open helper
using `golang.org/x/sys/unix` (promoted from indirect to direct), shared
by `daemonize()` writes, `lastStartupError()` reads, and `devtether logs`
reads. Directory and file are opened relative to an already-open directory
fd with `O_NOFOLLOW`; both fds are validated with `Fstat` after open.
Default `<config-dir>/.logs` and custom `settings.log_path` semantics
(including CWD-relative resolution) are preserved; append mode and modes
0700/0644 are preserved; FD 3 readiness and startup-error reporting are
preserved. No default-path migration, no proxy-log sanitization change, no
commit/PR.

Pre-mortem review of the design against ADR-003, ADR-006, and
`docs/engineering-standards.md`:

- **PM-1 (final-component-only boundary).** `O_NOFOLLOW` protects only the
  final directory component and the fixed `devtether.log` name; intermediate
  ancestors may still be symlinks (required for macOS `/var` →
  `/private/var` and absolute custom paths). This is deliberate and must
  not be described as stronger protection. Mitigation: document the
  boundary explicitly in code comments and the final report.
- **PM-2 (TOCTOU).** A path-based `Lstat`-then-open leaves a swap race.
  The design opens the directory with `O_DIRECTORY|O_NOFOLLOW`, then
  `Openat(dirFD, "devtether.log", ...)` with `O_NOFOLLOW`, then `Fstat`s
  both fds. No path-based check is the security boundary. Residual race:
  an attacker who can rename/replace the final directory inode between
  `MkdirAll` and `open` still cannot redirect the open, because the open
  is anchored to the fd and `O_NOFOLLOW` rejects symlink swaps of the final
  components.
- **PM-3 (hard links bypass `O_NOFOLLOW`).** Covered by requiring
  `st.Nlink == 1` on the opened file inode (post-open `Fstat`, not a
  pre-open path check).
- **PM-4 (FIFO/device hang or redirect).** Opening a FIFO `O_WRONLY`
  without `O_NONBLOCK` blocks forever. The helper opens with
  `O_NONBLOCK`, validates `S_ISREG` via `Fstat`, and rejects non-regular
  files; regular-file I/O ignores `O_NONBLOCK`, and the fd is wrapped with
  `os.NewFile` (kindNewFile → not added to the runtime poller).
- **PM-5 (umask).** `MkdirAll(dir, 0700)` and `OpenFile(..., 0644)` already
  produce owner-only dir and non-group/world-writable file modes under any
  umask that does not strip owner bits; the post-open permission check
  rejects pre-existing loose files/dirs rather than chmod'ing them
  (fail closed, no silent permission changes).
- **PM-6 (ownership strictness).** Directory and file must be owned by the
  process real UID (`os.Getuid()`), matching ADR-003's runtime-dir
  ownership check (`internal/daemon/lifecycle.go` uses the same
  `os.Getuid()`). In every supported execution mode the real and effective
  UIDs are equal (verified: `sudo` sets both to root), so this is
  behaviourally identical to an effective-UID check. Running the daemon as
  root (sudo) against a non-root-owned `.logs` will now fail closed; this
  is consistent with ADR-003 and is recorded as a known behavior change,
  not a defect. See the follow-up UID review below.
- **PM-7 (portability).** `unix.Openat`/`Fstat` exist on linux/amd64,
  linux/arm64, darwin/amd64, darwin/arm64 (verified in `x/sys@v0.49.0`
  sources and by cross-compilation). `Stat_t.Mode/Nlink/Uid` types differ
  across platforms; the helper relies on untyped `x/sys/unix` mode
  constants so `st.Mode` (uint32 on linux, uint16 on darwin) needs no
  conversion. Windows is unsupported per ADR-006.
- **PM-8 (scope creep).** Proxy log sanitization, default-path migration,
  and `daemon.RuntimeDir()` reuse are explicitly out of scope. The helper
  is CLI-local (`internal/cli`) because all three call sites live there;
  no new shared package.

### Verification evidence

Implemented on `feature/devin` (uncommitted, per owner stop condition). New
CLI-local helper `internal/cli/logfile.go`; `daemonize()` writes,
`lastStartupError()` reads, and `devtether logs` reads all route through it.
`golang.org/x/sys` promoted to a direct dependency (`go.mod`, no `go.sum`
change). 18 regression tests in `internal/cli/logfile_test.go`.

Checks run (Linux amd64, go1.27.3):

- `gofmt -l` on changed Go files — clean (no output).
- `go test -race -count=1 ./internal/...` — all packages `ok`.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `golangci-lint run` (v2.15.0, repo CI config) `./...` — `0 issues`.
- Cross-compile `GOOS/GOARCH` = linux/amd64, linux/arm64, darwin/amd64,
  darwin/arm64 `go build ./...` — all pass (confirms `unix.Openat`/`Fstat`
  and untyped mode constants compile on both supported platforms).
- `git diff --check` — clean.
- `govulncheck` — not installed on this machine; `scripts/test.sh` skips it
  when absent, so it did not run.

`bash scripts/test.sh` — module integrity (`go mod verify`) passed; the
module-hygiene step FAILED with "go.mod or go.sum has uncommitted changes
after 'go mod tidy'". This is expected and not a code defect: the slice is
intentionally uncommitted (owner stop condition), and the `go mod tidy`
promotion of `x/sys` to a direct dependency is itself an uncommitted
`go.mod` change. The script exits before its lint/vet/test steps; those were
run manually above with equivalent or stricter commands (race-enabled tests,
full lint, vet). Re-run `bash scripts/test.sh` after the owner commits.

Behavior notes discovered during implementation and locked in by tests:

- Linux returns `ENOTDIR` (not `ELOOP`) for `O_DIRECTORY|O_NOFOLLOW` on a
  symlink-to-directory; the helper maps this (and `ELOOP`) to a clear
  "is a symlink (unsafe)" error via a post-failure `Lstat` used only for the
  message, never as the security boundary.
- A trailing slash on the directory path would bypass `O_NOFOLLOW`;
  `filepath.Clean` strips it. Regression test added
  (`...RejectsDirSymlinkTrailingSlash`).
- FIFO log files are rejected without blocking via `O_NONBLOCK` + post-open
  `S_ISREG` `Fstat` check.

### Follow-up: ownership UID review + commit (2026-11-10)

Owner requested a focused read-only review of the ownership-UID wording
after the slice was committed. Findings:

- `internal/cli/logfile.go` validates ownership with `os.Getuid()`, which
  returns the **real** UID, not the effective UID. The implementation
  report's "effective uid" wording was inaccurate; corrected in PM-6 above.
- Go semantics: `os.Getuid()` = real UID (`getuid(2)`), `os.Geteuid()` =
  effective UID (`geteuid(2)`). Confirmed by a probe program (both 1000 in
  normal execution).
- Sudo behavior: `sudo id -u` and `sudo id -ru` both return `0`, so under
  `sudo devtether up -d` the real and effective UIDs are both root. In
  every supported execution mode (normal user, `sudo`, `setcap` for port
  80) real UID == effective UID, so the real-vs-effective choice has **no
  behavioral effect** on any supported flow.
- Consistency: the only other production ownership check,
  `internal/daemon/lifecycle.go` `validateDirInfo`, uses the same
  `os.Getuid()`. No production code uses `os.Geteuid()`. `logfile.go` is
  therefore consistent with the established ADR-003 interpretation of
  "current user."
- Code comments, user docs, and the tracker all say "current user" /
  "current-user ownership," which matches `os.Getuid()`; only the
  pre-mortem PM-6 said "effective uid" (now corrected).
- Root-daemon behavior: `sudo` sets both UIDs to root, so a root daemon
  requires root-owned `.logs`; a non-root-owned `.logs` fails closed. This
  matches PM-6's behavioral claim (verified) and ADR-003's root-owned
  runtime-location posture.

Recommendation (pending owner decision; no production code changed): keep
`os.Getuid()` (real UID) for consistency with `lifecycle.go` and the
fail-closed direction in theoretical real≠effective edge cases. The
alternative `os.Geteuid()` would match file-creation ownership semantics
but would diverge from the existing runtime-dir check and is not
behaviourally different in any supported mode; switching would warrant a
separate slice that also aligns `lifecycle.go`.

Commit: the security slice is committed as `bcb20c3`
(`fix(cli): reject unsafe symlinked daemon log paths`) and pushed to
`origin/feature/devin`, one commit ahead of `origin/develop` @ `f897117`.
The `scripts/test.sh` module-hygiene failure recorded above was pre-commit
only (uncommitted `go.mod`) and is resolved by the commit.

## §1m Detached daemon log UID ownership UX refinement (2026-11-10)

### Trigger

Owner-side test: after deleting the default `.logs` directory,
`sudo ./devtether up` failed with `log directory ... is owned by uid 1000,
not 0 (unsafe)` even though a fresh root-owned directory should have been
created.

### Investigation findings

- Creation order is correct: `openLogFileForWrite` calls `os.MkdirAll(logDir,
  0700)` before `openLogDir` validation (`internal/cli/logfile.go`).
- Reproduced on this host (ext4): as root, `MkdirAll`+`OpenFile` create a
  uid-0 directory/file (`getuid=0 geteuid=0`, `owner_uid=0`, mode 700/644), so
  the safe-open validation passes for a freshly created root-owned `.logs`.
  No code defect in creation or validation.
- The observed uid 1000 means `.logs` was not freshly created as root. The
  project path `/media/ivintitus/Data/...` is a secondary/mounted drive; the
  most plausible cause is a filesystem that does not honor Unix ownership
  (NTFS/exFAT/FUSE mounted with `uid=1000`), which reports every file as the
  mounting user regardless of creator. The alternative (a pre-existing
  uid-1000 `.logs`) cannot be excluded from here but was not assumed.
- The executed binary is not stale in any meaningful way: the error string
  matches the reviewed code exactly.

### Pre-mortem

- PM-1: Appending guidance to non-ownership errors would leak irrelevant
  advice. Mitigation: guidance is appended only by `logOwnershipError`, used
  solely for UID mismatches.
- PM-2: Synthetic `unix.Stat_t` tests are platform-fragile (`Mode` type
  differs linux uint32 / darwin uint16). Mitigation: tests obtain a real
  `unix.Stat_t` via `unix.Fstat` and only increment `Uid` (uint32 on both
  platforms); no root/sudo required.
- PM-3: Changing the UID error text could break existing assertions.
  Mitigation: no existing test asserts the UID message; the new message
  retains the `owned by uid`/`unsafe` prefix.
- PM-4: Recommending automatic ownership repair would weaken the safe-open.
  Mitigation: guidance names only the two supported paths (same execution
  mode, or a separate `settings.log_path`) and performs no chown/chmod/delete.

### Changes (uncommitted)

- `internal/cli/logfile.go`: added `logOwnershipGuidance` + `logOwnershipError`;
  the two UID-mismatch errors now include the path, actual owner UID, expected
  UID, and recovery guidance. No check weakened; no behavior change.
- `internal/cli/logfile_test.go`: added `statPath`/`ownerUID` helpers and four
  regression tests (first-run creation ownership/modes, foreign-owner dir/file
  rejection, concurrent creation race safety).
- `docs/architecture.md`, `docs/advanced-port-configuration.md`: documented
  the real-UID ownership model, per-mode recovery, and the non-POSIX
  filesystem limitation.

### Verification evidence

- `gofmt -l` clean; `go vet ./...` pass; `go build ./...` pass.
- `go test -race -count=1 ./internal/...` all `ok` (22 cli tests incl. 4 new).
- `golangci-lint run ./...` (v2.15.0) 0 issues.
- Cross-compile linux/darwin amd64/arm64 pass.
- `git diff --check` clean.
- `bash scripts/test.sh`: module integrity + hygiene + vet + race tests +
  cross-compile + build all pass (golangci-lint/govulncheck skipped by the
  script because they are not on its PATH; golangci-lint run manually = 0
  issues; govulncheck not installed).

### Remaining limitations

- Filesystems that do not honor Unix ownership cannot satisfy a different-UID
  ownership check; the error now points the user at a native filesystem via
  `settings.log_path`. This is environmental, not a code defect.
- Intermediate ancestors may still be symlinks (unchanged, documented).

## §1n Slice 0 pre-mortem — runtime-directory isolation (2026-11-10, Devin)

### Scope

Runtime-directory isolation + secure resolution ONLY (owner-authorized). No
log migration, no `settings.log_path` change, no state-dir work, no finding
closure, beta.8 stays open.

### Approved design (three owner read-only reviews)

- `resolveRuntimeDir(create bool)`; `create=true` secure-creates+validates the
  leaf; `create=false` requires an existing validated leaf; absent →
  `ErrNoRuntimeDir` wrapping `os.ErrNotExist`; never return an uncreated
  candidate as a connectable path.
- Canonicalize ONLY the UID-correct home; reject arbitrary symlinks in
  env-provided paths; follow only root-owned absolute system aliases into
  trusted targets.
- Descriptor-relative traversal, FD pinning, post-open `Fstat`; sticky-temp
  exception; `XDG_RUNTIME_DIR` exact-`0700`; production UID fixed to
  `os.Getuid()`; injected UID only in unexported helpers/tests.
- `RootDaemonMayBeRunning` best-effort, Lstat-only, documented heuristic.
- Owner correction: Linux returns `ENOTDIR` (not `ELOOP`) for
  `O_DIRECTORY|O_NOFOLLOW` on a symlink-to-dir; the walker must handle BOTH,
  using `Lstat` to confirm a symlink before applying the alias policy; use
  `O_NOFOLLOW` (not `O_NOFILE`).

### Pre-mortem risks

- PM-A: `ENOTDIR`/`ELOOP` mishandling → symlink-to-dir treated as a plain
  non-dir error or wrongly followed. Mitigation: `Lstat` on both, alias only
  when root-owned + absolute symlink; regression tests for `ENOTDIR`.
- PM-B: absent-leaf race → client dials an attacker-created predictable path.
  Mitigation: `create=false` never returns a candidate for an absent leaf.
- PM-C: existing `0777` runtime dir accepted (`validateDirInfo` lacked a perm
  check). Mitigation: `classifyDir` enforces not-group/world-writable.
- PM-D: API ripple (`SocketPath`/`LockPath`/`PIDPath` → error returns).
  Mitigation: update every caller; preserve not-running semantics via
  `os.ErrNotExist` wrapping; do not broadly convert unsafe-path errors into
  "not running".
- PM-E: macOS `/var` alias. Mitigation: root-owned absolute alias exception;
  on-Mac runtime verification still pending (cross-compile only).

## §1o Slice 0 implementation + verification (2026-11-11, Devin)

### Changed files

- `internal/daemon/securepath.go` (new) — `ErrNoRuntimeDir`, `classifyStat`,
  `secureOpenDir`/`secureOpenDirDepth` (descriptor-relative walk from `/` with
  `O_NOFOLLOW`, FD pinning, depth cap 40), `rootAliasTarget` (root-owned
  absolute alias only in trusted-system territory), `secureEnsureLeaf`
  (openat/`mkdirat` leaf, `ELOOP`+`ENOTDIR` handling with `Fstatat
  AT_SYMLINK_NOFOLLOW` to keep the explicit ADR-003 symlink message),
  `secureTempBase` (sticky-temp/user-owned, trusted-system for uid 0),
  `xdgRuntimeDir` (exact `0700`, ownership from stat), `tempRuntimeDir`,
  `resolveRuntimeDirUID`/`resolveRuntimeDir` (production UID fixed to
  `os.Getuid()`), `runtimeFilePath`.
- `internal/daemon/securepath_test.go` (new) — policy + resolver matrix.
- `internal/daemon/lifecycle.go` — `RuntimeDir()` delegates to
  `resolveRuntimeDir(true)`; `LockPath`/`PIDPath`/`noncePath` now return
  `(string, error)`; all lifecycle helpers propagate resolution errors;
  `DaemonRunning`/`WaitForExit` map `ErrNoRuntimeDir` to no-daemon;
  `RootDaemonMayBeRunning` rewritten as documented best-effort multi-candidate
  Lstat heuristic (never connects).
- `internal/daemon/daemon.go` — `SocketPath()` returns `(string, error)`;
  `CheckRunning` treats `ErrNoRuntimeDir` as not-running and surfaces other
  resolution errors; `NewServer` no longer resolves the socket eagerly —
  `Listen` resolves it after `RuntimeDir()` creation.
- `internal/daemon/client.go` — `socketClient` resolves `SocketPath()` on
  every dial, before any connection attempt.
- `internal/cli/doctor.go` — socket check never dials an unvalidated path;
  not-exist resolution passes, other resolution errors warn.
- `internal/daemon/lifecycle_test.go`, `internal/daemon/stress_test.go` —
  signature updates; test fixtures chmod XDG bases to exact `0700`.

### Design decisions realized during implementation

- `errInvalidRuntimeBase` sentinel: fallback to the temporary runtime
  directory happens ONLY when the XDG base itself is invalid. An absent or
  unsafe XDG leaf is authoritative and never bypassed by fallback (this
  preserves the explicit symlink/`0777` refusals instead of silently using
  `/tmp`).
- `xdgRuntimeDir` checks ownership from the stat directly rather than the
  walk class, because for uid 0 a root-owned directory classifies as
  trusted-system, not user-owned; without this, root's own `/run/user/0`
  would have been rejected.
- `secureTempBase` accepts trusted-system directories for uid 0 for the same
  reason.
- `secureEnsureLeaf` distinguishes symlink from ordinary non-directory via
  `Fstatat(AT_SYMLINK_NOFOLLOW)` so the ADR-003 "is a symlink (possible
  attack)" message is preserved on both `ELOOP` and Linux `ENOTDIR`.

### Verification evidence

- `gofmt` clean on all changed files (pre-existing gofmt drift in
  `internal/cli/status.go` and `internal/dns/server.go` left untouched —
  unrelated to this slice).
- `go test -race -count=1 ./internal/...` — all packages ok.
- `go build ./...`, `go vet ./...` — clean.
- `golangci-lint run ./internal/...` (v2.15.0) — 0 issues.
- `bash scripts/test.sh` — all checks passed (6 passed, 2 skipped:
  govulncheck/golangci-lint not on PATH; golangci-lint run manually above).
- Cross-compilation: linux/arm64, darwin/amd64, darwin/arm64 all build.
  This is cross-compilation only, NOT macOS runtime verification.
- New regression coverage: `0777` existing leaf rejected; acceptable leaf
  perms accepted; XDG non-`0700` rejected; absent leaf → `ErrNoRuntimeDir`
  wrapping `os.ErrNotExist` with no usable path from `SocketPath()`;
  foreign-owned leaf (synthetic uid) rejected; symlinked leaf rejected (both
  `ELOOP`/`ENOTDIR`); user-controlled symlinks in XDG/TMPDIR rejected;
  sticky-temp child cases (symlink/foreign/`0777`/`0700`); temp fallback
  creates isolated per-UID `0700` leaf; `RootDaemonMayBeRunning` side-effect
  free. Test coverage at that point: synthetic-UID foreign-ownership policy
  tests plus ONE root-gated test (`TestRootOwnedSystemAlias`); actual
  cross-UID leaf ownership had no committed test (manual `sudo -E` E2E only).
  Corrected in §1p after the independent audit.
- End-to-end (built binary, isolated `XDG_RUNTIME_DIR`):
  - Absent leaf: `status` → "daemon is not running" (exit 1); `down` →
    "not running"; `logs` unchanged.
  - Existing leaf, no daemon: `status` → "not running".
  - `0777` leaf: `status` → explicit unsafe-permissions error, NOT
    "not running" (unsafe-path errors are not converted to not-running).
  - `doctor`: reports the resolution problem as a warning, never dials.
  - `up -d` / `status` / `down` cycle works; socket/PID/nonce removed on
    shutdown while the leaf dir + lock persist.
  - `sudo -E` with the user's `XDG_RUNTIME_DIR` preserved: root did NOT use
    the user's XDG leaf (it stayed empty); root created
    `/tmp/devtether-0` with root-owned socket/lock/PID/nonce — the per-UID
    isolation invariant holds.

### Remaining limitations

- macOS filesystem behaviors (`O_NOFOLLOW` `ELOOP`/`ENOTDIR`, `mkdirat`,
  `os/user` home resolution, `/var` alias, `$TMPDIR` perms) are verified by
  source inspection and cross-compilation only; they require on-Mac runtime
  testing before any macOS verification claim.
- Root-gated tests (root-owned alias, actual foreign ownership) skip when
  unprivileged; synthetic-UID policy tests cover the logic.
- `RootDaemonMayBeRunning` remains a best-effort heuristic (custom root
  `XDG_RUNTIME_DIR` may be missed); it never connects.
- Slice 0 is uncommitted; independent read-only audit pending. The security
  finding remains unresolved; beta.8 stays open.

## §1p Independent audit of Slice 0 + condition corrections (2026-11-11, Devin)

### Independent read-only audit (verdict: ACCEPT WITH CONDITIONS)

Audited the working-tree diff against HEAD `71e9dec` line by line
(`securepath.go`, all integration points, tests). **No confirmed production
code defect.** Verification re-executed by the auditor: `go build`, `go vet`,
`go test -race -count=1 ./internal/...` (all ok), `bash scripts/test.sh`
(6 passed, 2 skipped), `golangci-lint run ./internal/...` (0 issues).
Conditions:

1. Add a root-gated test exercising REAL filesystem ownership (chown to a
   different UID) through the production resolver.
2. Add a client-level absent-leaf no-dial regression test.
3. Correct the §1o wording that overstated root-gated ownership coverage.
4. (Optional) Explicit ELOOP coverage if meaningful without artificial
   abstractions.

### Corrections implemented (tests/docs only; no production change)

- `TestForeignOwnedLeafRejectedReal` (root-gated, `securepath_test.go`):
  creates an isolated XDG base (0700, owned by the test process), creates the
  leaf and `chown`s it to a genuinely different UID (1000, or `nobody` when run
  as uid 1000), then asserts `resolveRuntimeDir(false)` and `SocketPath()`
  reject and return an empty path. Skips cleanly when not root; no production
  UID override. **Executed as real root** via a `-race` test binary under
  `sudo`: PASS. Fixture cleanup verified (no leftovers under `/tmp` or `/`).
- `TestSocketClientAbsentLeafNoDial` (new `internal/daemon/client_test.go`):
  with the XDG leaf absent, `socketClient`'s `DialContext` returns
  `ErrNoRuntimeDir` wrapping `os.ErrNotExist` and a nil connection. Because a
  unix dial error can never carry the `ErrNoRuntimeDir` sentinel, this proves
  the resolution error — not a dial failure — is returned, i.e. no connection
  attempt occurred. No new abstraction or seam introduced.
- `TestSymlinkToFileLeafRejected` (ELOOP condition): added, but an errno probe
  on Linux 6.8 showed `O_DIRECTORY|O_NOFOLLOW` returns **ENOTDIR** for
  symlink-to-dir, symlink-to-file, and dangling symlinks alike — the ELOOP
  branch is POSIX/BSD behavior and is NOT reachable on Linux. The test
  therefore covers the symlink/non-directory disambiguation (`Fstatat
  AT_SYMLINK_NOFOLLOW`) on the ENOTDIR path, which is the Linux-reachable
  behavior; ELOOP remains covered only by code inspection and is exercised on
  macOS runtime verification (still pending). The test comment states this
  explicitly. Not claimed as ELOOP coverage.
- Docs: §1o limitation corrected; this section added; `agent_diary.md` entry
  added; `tracker.md` security task status updated.

### Verification of corrections (2026-11-11, current working tree)

Unprivileged (`ubuntu`, uid 1000):

- `gofmt -l internal/`: only the pre-existing `internal/cli/status.go` and
  `internal/dns/server.go` drift; no file touched by Slice 0 is listed.
- `go build ./...`, `go vet ./...`: clean.
- `go test -race -count=1 ./internal/...`: all packages ok (daemon 25.8s;
  root-gated tests SKIP).
- `bash scripts/test.sh`: all checks passed (6 passed, 2 skipped).
- `golangci-lint run ./internal/...` (v2.15.0): 0 issues.
- Focused unprivileged run: `TestSocketClientAbsentLeafNoDial` PASS,
  `TestSymlinkToFileLeafRejected` PASS, `TestForeignOwnedLeafRejectedReal` SKIP
  ("requires root"), `TestRootOwnedSystemAlias` SKIP.

Root (`sudo -n`, compiled `-race` daemon test binary, scoped `-test.run`):

- `TestForeignOwnedLeafRejectedReal` PASS (real chown to uid 1000 rejected,
  empty path).
- `TestRootOwnedSystemAlias` PASS.
- All other securepath/client tests PASS.
- `TestClassifyStatPolicy` and `TestUserControlledSymlinksRejected` FAIL when
  the WHOLE package is run as uid 0. This is an artifact, not a defect: those
  tests are synthetic-UID policy tests written for a non-root process (as uid
  0, "self" and "foreign" UIDs coincide, and the uid-0 temp-base exception in
  `secureTempBase` changes the expectation). They pass unprivileged (see the
  race run above) and are not part of the root-gated set. Running the package
  as root is not a supported test mode.
- Fixture cleanup verified: no `/devtether-alias-test` or
  `/tmp/devtether-foreign-owner*` left behind.

## §1q Independent full-branch integration audit (2026-11-11, Devin)

Read-only audit of the complete pushed `feature/devin` branch reviewed as one
integrated change set against `develop` — all commits after the merge base,
not only the latest. Executed on a detached worktree at the audited HEAD; no
repository modification. (§1n–§1p above cover the Slice 0 runtime-directory
slice specifically; this section is the complete-branch integration review.)

### Scope and state

- Audited branch: `feature/devin` @ `c716f033aab656f9475b22022b58dded0828c835`
  (`c716f03`).
- Base: `develop` @ `f8971178fd96abde26a2ab24a7ab3ebf00ededab`; merge base =
  `develop` HEAD. Ahead/behind: **6 ahead, 0 behind**.
- Commits reviewed: `bcb20c3` (log safe-open), `d99cfff` (UID-review docs),
  `71e9dec` (log ownership UX), `1e5f152` (Slice 0 runtime-dir isolation),
  `73a14d6` (workspace sync + `scripts/test.sh` exec bit), `c716f03` (Go
  1.27.4 bump for GO-2026-6859).
- Scope: detached log safe-open (`internal/cli/logfile.go`, `up.go`,
  `logs.go`, `logfile_test.go`), runtime-directory isolation
  (`internal/daemon/securepath.go`, `lifecycle.go`, `daemon.go`, `client.go`,
  `cli/doctor.go` + tests), combined security boundary and all callers of
  changed signatures, non-security changes (doctor help text, `go.mod`/`go.sum`,
  `x/sys` promotion, `scripts/test.sh` mode, workspace records), and
  consistency with ADR-003 / ADR-006 / architecture / engineering standards.

### Verification executed

- `gofmt -l` on changed files — clean. Pre-existing repo-wide drift
  (`internal/cli/status.go`, `internal/dns/server.go`) unchanged by this branch.
- `git diff --check` — clean.
- `go build ./...`, `go vet ./...` — clean.
- `make test` (runs `./scripts/test.sh`; tracked mode 100755 verified) —
  **8 passed, 0 skipped**: `go mod verify`, `go mod tidy`, `golangci-lint`
  v2.15.0 (0 issues), `go vet`, `govulncheck` (0 affecting code; 7
  required-module vulnerabilities not called), `go test -race -count=1 ./...`
  (all packages ok), darwin/arm64 cross-build, linux build.
- Cross-compile: linux/arm64, darwin/amd64, darwin/arm64 — all pass
  (cross-compilation only; NOT macOS runtime verification).
- Root-gated tests under `sudo -n` (compiled `-race` daemon test binary,
  scoped `-test.run`): `TestForeignOwnedLeafRejectedReal` PASS (real `chown`
  to uid 1000 rejected), `TestRootOwnedSystemAlias` PASS, all other
  securepath/client tests PASS. `TestClassifyStatPolicy` and
  `TestUserControlledSymlinksRejected` FAIL under a whole-package uid-0 run —
  the §1p documented artifact (synthetic-UID policy tests; unsupported test
  mode), not a defect.
- Built-binary E2E: `up -d` with symlinked `.logs` → explicit
  `log directory ... is a symlink (unsafe)` rejection, exit 1, no detached
  spawn, nothing written through the link; absent leaf → `status` "not
  running" exit 1, `down` idempotent exit 0, `doctor` socket check
  pass/warn correctly; `0777` leaf → explicit unsafe-permissions error
  (not "not running"); `logs` with absent file → friendly message.
- CI status: no CI runs exist for `c716f03` — `ci.yml` triggers only on
  push/PR to `main`/`develop`. The `macos-latest` test matrix exists but has
  not exercised this branch.

### Security conclusion

- Original detached-daemon log symlink/arbitrary-file-write vulnerability:
  **remediated**. Final directory and file components are descriptor-anchored
  (`openat`/`O_NOFOLLOW`) and post-open `Fstat`-validated; the same safe-open
  boundary serves `daemonize()` writes, `lastStartupError()` reads, and
  `devtether logs` reads; the log is opened before child spawn. No realistic
  path to the original arbitrary-file-write impact remains within the
  malicious-repository threat model.
- Runtime-directory isolation: **acceptable**. Per-UID leaf isolation,
  validated bases, absent leaf never returned as a connectable candidate
  (`ErrNoRuntimeDir` → not-running), unsafe-path errors propagate rather than
  degrade to "not running", client dials resolve the validated path per dial.
- Combined boundary: consistent real-UID ownership model, compatible error
  semantics, no divergent validation of the same path, no weakened fail-closed
  fallback, no stale callers, no unnecessary abstraction.

### Findings

No confirmed defects; no security finding with a credible remaining attack
path; no blocking verification gaps. Accepted limitations: intermediate
ancestors above the final log directory may be symlinks (documented boundary);
log file mode `0644` (pre-existing choice); macOS runtime filesystem behavior
verified by source review and cross-compilation only. Documentation
inconsistencies found (stale B8-1 status, Slice-0 sections misattributed as
the full-branch review, stale HEAD/commit/test counts in the tracker, missing
diary entry) were corrected in this reconciliation pass.

### Verdict

**ACCEPT WITH CONDITIONS** — all four conditions were documentation-only and
are resolved alongside this record. Outstanding separately: macOS runtime
verification (PR-time CI will exercise the `macos-latest` matrix), owner
local-run verification, owner acceptance, owner-signed squash integration.
The security finding is NOT marked owner-accepted; beta.8 remains open.

## §2 Findings log

| ID | Severity | Location | Finding | Status |
|---|---|---|---|---|
| B8-1 | Low | `scripts/test.sh` | Committed mode 100644; `make test` cannot exec it. Local runs use `bash scripts/test.sh`; CI unaffected (calls `go test` directly). One-line `chmod +x` fix — tracked in tracker as separate hygiene item. | Resolved — tracked mode is now 100755 (commit `73a14d6`); `make test` executes the script directly; independent full-branch audit recorded 8 passed / 0 skipped (see §1q) |
| B8-2 | Medium | `internal/config/config.go` `validateRoutes` | Case-variant duplicate route domains passed `LoadConfig` but failed daemon startup with `ErrRouteExists` | Fixed and pushed at `20ef459` — normalized-duplicate guard via `netutil.NormalizeHost` + regression tests; see §1c |
| B8-3 | Low | `internal/router/router_test.go` | Missing direct test for create→remove→recreate→stale-incarnation removal survival | Fixed and pushed at `20ef459` — `TestEngine_StaleIncarnationCannotRemoveReplacement`; see §1c |
