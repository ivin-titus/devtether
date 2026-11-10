# Implementation Plan — Live Route Management (beta.8)

Design rev 5, owner-approved architecturally. Baseline `develop` @ `6fac500`.
Code-verified against `internal/{router,health,daemon,proxy,cli,config}`.

## 1. What changes (minimal diffs, Ponytail-applied)

1. `internal/config/config.go` — extract exported `ValidateRoute(domain, port)`;
   `validateRoutes` calls it. Identical rules for file and IPC paths.
   Slice 3 adds exported `ValidateRouteDomain(domain)` — the domain-only
   policy extracted from `ValidateRoute`, which reuses it and then applies
   its existing port check; DELETE `/routes/{domain}` uses it (there is no
   port to pass — no `ValidateRoute(domain, 1)` workaround).
2. `internal/router/router.go` — `nextInc` counter; `ErrRouteExists`; atomic
   `AddRoute` → `(uint64, error)`; `RemoveRoute` → `(uint64, bool)`;
   `Incarnation` on `Target`/`RouteView`; `RouteLive`.
3. `internal/health/health.go` — `cur`/`hi` maps; `Register`/`Unregister`;
   `Observe`/`ObserveNow` gain `inc`; `Target.Incarnation`.
4. `internal/daemon/daemon.go` — `POST /routes` + `DELETE /routes/{domain}`;
   shared nonce check; strict JSON; `Register`/`Unregister(domain, inc)` with
   returned incarnations; incarnation-required delete via `?inc=` (D15 — no
   unconditional path); `incarnation` field on `RouteResponse`. Boot
   registration wiring lives in `cli/up.go` (slice 2), not here.
5. `internal/daemon/client.go` — `AddRoute`/`RemoveRoute(domain, inc)`; nonce;
   201/204 expected; daemon body surfaced in errors.
6. `internal/cli/routes.go` — `routes add <domain> <port>` /
   `routes remove <domain> [-y|--yes]` subcommands (bare `routes` listing
   and `--check` behavior unchanged); confirmation required for EVERY
   remove, warning tailored to route type (static: YAML unchanged, returns
   after restart; live: ephemeral, must be re-added); prompt reads
   `cmd.InOrStdin()`; non-TTY without `-y` fails safe; `-y` skips
   confirmation only; no `--force`; daemon-only (no config fallback);
   daemon-down handling; ephemeral notice on add; HTTP 409 → "route
   changed — re-confirm", never silent retry.
7. Docs — MANDATORY (pre-mortem PM-1): amend ADR-005 design principle 4
   ("route changes require a restart"), `routes` Long text ("live reload via
   IPC is not available"), `architecture.md` §5/§6/§7, `noRoutesHint` — all as
   part of the feature, not follow-ups.

## 2. Data structures

```go
type Engine struct {
    routes  map[string]*Target
    nextInc uint64        // incarnation counter; issued under mu on every insert
    mu      sync.RWMutex
}

type Tracker struct {
    mu   sync.RWMutex
    seq  uint64           // observation generation (unchanged)
    cur  map[string]uint64 // domain → live incarnation; absent = none
    hi   map[string]uint64 // domain → highest incarnation ever inserted or
                           // removed; never decreases (tombstone)
    gens map[string]uint64
    obs  map[string]Observation
}
```

`Target`/`RouteView` gain `Incarnation uint64` — internal plumbing; the single
user-facing exception is an opaque `incarnation` field on `RouteResponse`
(`GET /routes`), consumed only by the CLI's conditional DELETE (D15, owner
exception to D12). Never rendered as a meaningful number in CLI text.

## 3. Consistency model

```go
// Register admits inc only when it exceeds the high-water mark.
func (t *Tracker) Register(domain string, inc uint64) {
    t.mu.Lock(); defer t.mu.Unlock()
    if inc == 0 || inc <= t.hi[domain] { return }
    t.hi[domain] = inc
    t.cur[domain] = inc
    delete(t.gens, domain); delete(t.obs, domain)
}

// Unregister rejects 0, tombstones hi unconditionally, then clears
// cur/gens/obs only when cur still holds expectedInc.
func (t *Tracker) Unregister(domain string, expectedInc uint64) {
    t.mu.Lock(); defer t.mu.Unlock()
    if expectedInc == 0 { return }
    if expectedInc > t.hi[domain] { t.hi[domain] = expectedInc }
    if t.cur[domain] != expectedInc { return }
    delete(t.cur, domain)
    delete(t.gens, domain); delete(t.obs, domain)
}

// Observe admits non-zero inc == cur[d] with gen newer than applied.
func (t *Tracker) Observe(domain string, inc, gen uint64, o Observation) bool {
    t.mu.Lock(); defer t.mu.Unlock()
    if inc == 0 || t.cur[domain] != inc || gen <= t.gens[domain] { return false }
    t.gens[domain] = gen; t.obs[domain] = o
    return true
}
```

### Invariants

1. Engine serializes mutations on `mu`; incarnations unique, issued in insert
   order; `RemoveRoute` returns the removed incarnation.
2. Per handler: engine mutation → tracker op carrying its incarnation.
3. `hi[d]` never decreases; `Register` only admits `inc > hi[d]`.
4. `Observe` requires non-zero `inc == cur[d]` and newer `gen`. `inc==0` must
   be rejected first (absent `cur[d]` reads 0 and would otherwise pass).
5. Convergence: after handler ops settle, `cur[d]` equals the incarnation of
   the route in the engine map. Strict create means a domain's incarnation
   sequence advances only through remove→re-add, so `Unregister` only ever
   tombstones a route already gone — a delayed `Register` can never be
   wrongly rejected.

### Producers

- Proxy: `targetCtxKey` already carries the resolved `router.Target`
  (including `Incarnation`) — no new context key is introduced. Both
  observation sites read it: `ModifyResponse` must read `targetCtxKey`
  (it currently reads only host+gen), and `ErrorHandler` must HOIST its
  existing `targetCtxKey` read above the offline `Observe` (it currently
  reads the target after the observation). A missing target yields inc 0,
  which `Observe` rejects fail-closed.
- Startup check + `routes --check`: `RouteView.Incarnation` →
  `health.Target.Incarnation` at BOTH `health.Target` construction sites —
  `daemon.routeTargets()` AND `cli/up.go startupTargets()` (PM-2; missing one
  silently keeps every route `unknown`). These are the only two production
  `health.Target` literals (grep-verified 2026-11-07); re-verify during
  implementation that no additional site exists.
- Boot: register each static route (`GetAllRoutes` → `Register`) in
  `cli/up.go` immediately after server construction (after the
  `daemon.NewServer` call, before binds and workers start) — deterministic
  and single-threaded; any observation arriving before registration is
  rejected and would leave routes `unknown`.
- Slice-3 dependency (recorded now, implemented in slice 3): the DELETE
  handler must call `Unregister(domain, removedInc)` with the exact
  incarnation returned by `RemoveRoute`, preserving engine → tracker
  ordering (invariant 2). `Unregister` has no production caller until
  slice 3; slice 2 exercises it in unit tests only.

## 4. Atomic create

```go
var ErrRouteExists = errors.New("route already exists")
func (e *Engine) AddRoute(domain, serviceName string, port int, routeType RouteType) (uint64, error)
// Single remove path (D15): expectedInc required and non-zero; deletes only
// when the stored incarnation still equals it. ok=false on absence OR
// mismatch — handler disambiguates via Resolve for 404 vs 409.
// No unconditional delete exists: a stale confirmation can never remove a
// concurrent replacement.
func (e *Engine) RemoveRoute(domain string, expectedInc uint64) (inc uint64, ok bool)
```

One `mu.Lock()`: normalize, empty-check, exists→`ErrRouteExists`, else
`inc=++nextInc`, insert, return. Concurrent creates: exactly one success.
Concurrent removes: one `true` → 204, rest 404.

## 5. IPC contract (Slice 3 approved design)

Slice 3 pre-implementation audit at `7c25ff5` (2026-11-10): READY WITH
CONDITIONS, no confirmed defects. Owner accepted conditions C1–C7 plus
clarifications: canonical `inc` grammar, strict JSON key set, daemon-only
CLI remove, and the decisions below. Audit record: `audit_report.md` §1e.
Owner review of docs commit `7ff7e67` accepted the design and added
corrections F1–F3 (`audit_report.md` §1f), folded in below.

### Routing

Mux keeps `"/routes"`, `"/routes/check"`, `"/status"`, `"/shutdown"` and
adds `"/routes/{domain}"`. Go ServeMux precedence: the more specific
`/routes/check` wins for the exact path, so `DELETE /routes/check` reaches
the check handler → 405 (POST-only); `/routes/check/` and deeper paths →
404. `PathValue("domain")` is percent-decoded but not normalized (e.g.
`a%2Fb` → `a/b`) — domain validation (`ValidateRoute` for POST,
`ValidateRouteDomain` for DELETE) runs on the literal value before any
engine call.

### POST /routes — create live route

Body: exactly one JSON object with exactly the keys `domain` (string) and
`port` (integer), exact casing. Success: `201` + `RouteResponse`.

Handler order — every rejection happens before any mutation:
1. Method: non-POST → 405.
2. Nonce: `X-Devtether-Nonce` == server nonce, else 403.
3. Body bound: `http.MaxBytesReader(w, r.Body, 8<<10)`; decode failure from
   `*http.MaxBytesError` → 413, other decode failures → 400.
4. Strict JSON (stdlib only, no new dependency): `json.Decoder` with
   `UseNumber()`, consumed via `Token()`. Require `{`, then exactly the
   keys `domain` and `port` in any order, each exactly once with exact
   casing; `domain` value must be a string token; `port` value must be a
   `json.Number` matching `^[0-9]+$` (rejects fractions, exponents,
   negatives, signs); then `}`, then `io.EOF` — trailing data → 400.
   Rejects: duplicate keys, case variants (`PORT`), unknown fields,
   `null`, non-object values, wrong types, trailing JSON values.
   Port conversion (F2): the digits-only `json.Number` is converted with
   an overflow-checked parse (`strconv.Atoi`); any conversion failure →
   400, and `ValidateRoute` enforces 1–65535 → 400. Malformed or
   overflowing client input is never a 500.
5. `config.ValidateRoute(domain, port)` on the literal domain → 400.
   **Port 1–65535 enforced here server-side (C1)** — `router.AddRoute` does
   not range-check ports, so this is the only port gate for IPC.
6. `inc, err := engine.AddRoute(domain, "live", port, RouteLive)` →
   `ErrRouteExists` → 409; other error → 500.
7. `tracker.Register(netutil.NormalizeHost(domain), inc)` — tracker keys
   match `GetAllRoutes().Domain` (engine → tracker ordering, §3 inv. 2).
8. `201` + `RouteResponse`.

### DELETE /routes/{domain}?inc=N — conditional remove

`inc` contract (strict): exactly one `inc` value
(`r.URL.Query()["inc"]` length 1) matching canonical positive decimal
`[1-9][0-9]*`; then `strconv.ParseUint(v, 10, 64)`. Missing, empty
(`?inc=`), bare `?inc`, duplicated (`?inc=1&inc=2`), zero, negative,
leading-zero (`010`), `+1`, whitespace, hex, float, exponent, or overflow
(> MaxUint64) → all 400. **No unconditional delete path exists (D15).**

Handler order:
1. Method: non-DELETE → 405.
2. Nonce → 403.
3. `config.ValidateRouteDomain(domain)` on the literal path domain → 400
   (domain-only policy identical to `ValidateRoute`'s domain checks; F1).
4. Strict `inc` parse → 400.
5. `removedInc, ok := engine.RemoveRoute(domain, inc)` (engine normalizes
   internally). On `ok`: `tracker.Unregister(netutil.NormalizeHost(domain),
   removedInc)` with the exact removed incarnation → `204`, empty body.
6. On `!ok`, disambiguate with `engine.Resolve` (C2 — `RemoveRoute` is
   always attempted first, never resolve-then-remove): domain present with
   a different incarnation → 409 ("route changed — re-confirm"); absent →
   404.

Status map: 400 invalid input (JSON, domain, `inc` grammar, port
conversion/range) · 403 bad nonce · 404 absent route ·
405 wrong method · 409 incarnation mismatch · 413 oversized body (POST
only) · 500 unexpected internal error (e.g. `AddRoute` failure — never
caused by malformed client input) · 201/204 success.

### Unchanged contracts

`GET /routes`, `POST /routes/check`, `GET /status`, `POST /shutdown` keep
their beta.7 methods, shapes, and status codes. `GET /routes` gains one
additive opaque `incarnation` field per row (the single justified D12
exception, consumed only by the CLI's conditional DELETE).

**Two-snapshot contract (C3, accepted limitation):** `handleRoutes` reads
the engine snapshot (`GetAllRoutes`) and tracker snapshot (`Snapshot`) at
different instants. All fields of a row — including `incarnation` — come
from that row's engine snapshot, so `incarnation` is authoritative for the
conditional DELETE; health fields may describe a different incarnation
when a remove/re-add lands between the snapshots. Accepted: health is
observational (`unknown` is safe); the tracker API is NOT expanded for an
atomic snapshot (owner decision).

### Client (`internal/daemon/client.go`)

- `AddRoute(ctx, domain, port)` → POST `/routes` + nonce; expects `201`,
  decodes `RouteResponse`.
- `RemoveRoute(ctx, domain, inc)` → DELETE `/routes/{domain}?inc=N` + nonce;
  expects `204`.
- Non-success responses return an explicit typed error carrying HTTP
  status + trimmed body, so the CLI maps 400/403/404/409/413 without string
  matching. Existing client methods unchanged.

### CLI (`internal/cli/routes.go`)

- Bare `devtether routes` (list, daemon→config fallback) and
  `devtether routes --check` — **unchanged**.
- `routes add <domain> <port>` — requires a running daemon (mutation; no
  config fallback); `client.AddRoute`; ephemeral-route notice on success.
- `routes remove <domain> [-y|--yes]` — daemon-only, no config fallback:
  1. `client.ListRoutes` → locate domain; absent → error exit. `type` +
     `incarnation` come from the same row (engine-sourced, authoritative
     for the conditional DELETE).
  2. Type-specific warning: static → running-daemon removal only, route
     returns on restart (YAML untouched); live → ephemeral, re-add after
     restart.
  3. Confirmation prompt reads `cmd.InOrStdin()`; skipped only by
     `-y/--yes`; non-TTY without `-y` fails with guidance; decline or EOF
     aborts with no mutation. `-y` skips the prompt only — daemon check,
     route lookup, and conditional DELETE still apply.
  4. `client.RemoveRoute(domain, incarnation)`. On 409: stop and report
     "route changed — re-confirm"; **never** re-list and silently retry.
- No `--force`. Static removal never writes `devtether.yaml`.

## 6. Test matrix (deterministic, no sleeps)

- **Interleaving A:** `RemoveRoute(d,10)` → `AddRoute` → `Register(11)` → `Unregister(10)` → `cur=11`, inc-10 obs rejected.
- **Interleaving B:** `AddRoute` (no Register) → `RemoveRoute(d,10)` → `Unregister(10)` → `AddRoute` → `Register(11)` → delayed `Register(10)` → `cur=11`.
- **Tombstone resurrection:** `Unregister(d,10)` with `cur` absent → `hi=10` → `Register(d,10)` rejected.
- **Zero incarnation:** `Observe(d,0,gen,o)` false + snapshot unchanged; `Unregister(d,0)` mutates nothing.
- Concurrent creates / removes / add+remove (WaitGroup + start gate, `-race`).
- Resolved-then-removed request obs rejected (proxy ctx target inc).
- Probe completing after replacement (`checkAll` injected probe + channel).
- Slice-2 fixture registration (beta.7 assertions must NOT weaken):
  `stress_test newStack`, proxy `newTestProxyWithTracker` /
  `newProxyWithTransport`, and `daemon_test TestHandleRoutesIncludesHealth`
  must `Register` route incarnations (`ObserveNow` gains `inc`) or existing
  health assertions fail. Verified no-change fixtures:
  `TestHandleRoutesCheck` and `routes_test.go TestRunRoutesCheckExitCodes`
  (wire results are independent of `Observe` acceptance), DNS and cli tests.
- Construction-site assertions: `startupTargets` (cli) and `routeTargets`
  (daemon) return non-zero `Incarnation` matching the engine.
- Wire-compatibility lock: `CheckAll` with `Incarnation:0` targets leaves
  tracker state untouched while returned results stay correct (`routes
  --check` exit codes unchanged).
- Proxy end-to-end incarnation: channel-gated transport; a request resolved
  before remove+re-add completes after → the old-incarnation observation is
  rejected (proves the `targetCtxKey` plumbing).
- In-flight request across removal; new requests 404; DNS NXDOMAIN.
- ServeMux precedence: `DELETE /routes/check` → 405 via the check handler;
  `DELETE /routes/check/` → 404; `GET /routes/{domain}` → 405; `DELETE
  /routes/` → 404; percent-encoded domain (`a%2Fb`) → 400 via
  `ValidateRoute`.
- Nonce: handler tests construct `Server` with an explicit non-empty
  `s.nonce` (C4 — empty==empty must not pass); missing/wrong nonce → 403 on
  POST and DELETE; correct nonce proceeds.
- Client 201/204 via real socket (`XDG_RUNTIME_DIR` → `MkdirTemp("/tmp","dt")`,
  macOS `sun_path`); non-success statuses surface status + trimmed body.
- POST JSON edges → 400/413, engine unchanged: syntax error, unknown field,
  duplicate key, case-variant key (`PORT`), `null` body, non-object value,
  trailing JSON value, wrong types (port string/float/exponent), >8KB,
  empty body. Port conversion (F2): digits-only integer overflowing int
  range → 400, never 500; range 1–65535 → 400 via `ValidateRoute`.
  Strict decode = `json.Decoder.Token()` stream (§5).
- `ValidateRoute` parity table (file vs IPC acceptance set); port
  boundaries server-side (C1): 0, -1, 65536 → 400; 1, 65535 → 201.
- DELETE `inc` edges → all 400: missing, `?inc=`, bare `?inc`, `?inc=0`,
  `?inc=-1`, `?inc=+1`, whitespace, hex, float, exponent, overflow,
  duplicate `?inc=1&inc=2`, leading zero `?inc=010`; max uint64 → 404/409.
- DELETE domain edges → 400 (non-`.localhost`, whitespace, `a%2Fb`);
  valid-absent → 404; present wrong inc → 409; present right inc → 204.
  `config.ValidateRouteDomain` unit tests: domain policy identical to
  `ValidateRoute`'s domain checks (shared-helper parity); `ValidateRoute`
  port behavior unchanged.
- DELETE on static AND live domains → 204 after confirmation; engine removes
  runtime-only, YAML untouched (D14). Confirmation required for both; warning
  tailored to type.
- Stale-confirmation race (D15): CLI confirms inc10, a concurrent add
  re-creates the domain as inc11, `DELETE ?inc=10` → 409 and inc11
  survives — scripted order, no sleeps; never silently re-fetch + retry.
- Concurrency (WaitGroup + start gate, `-race`, channel-gated where order
  matters): concurrent POST same domain → exactly one 201, rest 409;
  concurrent DELETE same inc → one 204, rest 404; DELETE inc=10 racing
  re-add inc=11 → 409 with inc11 route + health surviving; POST→DELETE→POST
  incarnations strictly increase; post-settle `cur`/`obs` reflect only the
  replacement incarnation.
- Remove CLI: daemon-only (no config fallback); `routes remove` daemon-down
  → clear error; non-TTY without `-y` exits nonzero with guidance; `-y`
  skips the prompt only (lookup/validation/DELETE still apply); decline/EOF
  abort without mutation; no `--force`; YAML never touched. TTY
  accept/decline/EOF prompt behavior → human verification (§7), not
  unit-tested without a pty.
- Boot registration: startup `CheckAll` observations apply against static-route
  incarnations (both `startupTargets` and `routeTargets` carry `Incarnation`).
- Lint: `NewRequestWithContext`, `net.ListenConfig`, errcheck, govet shadow, `ReadHeaderTimeout`.

## 7. Execution slices (each independently reviewable)

Post-pre-mortem slice plan (pre-mortem complete, verdict: sound to implement).
The Slice 2 pre-implementation audit was owner-accepted (2026-11-07); its
conditions C1–C5 are folded into §3, §6, and slice 2 below. The Slice 3
pre-implementation audit (`7c25ff5`) was owner-accepted (2026-11-10) with
clarifications; conditions C1–C7 and the strict `inc`/JSON decisions are
folded into §5, §6, and slice 3 below.

1. **Slice 1 — config + router:** `config.ValidateRoute` extraction; `nextInc`,
   `ErrRouteExists`, atomic `AddRoute`→inc, `RemoveRoute`→(inc,ok),
   `Incarnation` fields, `RouteLive`. Tests: concurrent create/remove, parity.
2. **Slice 2 — health incarnations:** `cur`/`hi`, `Register`/`Unregister`,
   `Observe`/`ObserveNow` signatures, `health.Target.Incarnation`, both
   construction sites (re-verify no third production site during
   implementation) + proxy `targetCtxKey` reads (`ModifyResponse` adds one;
   `ErrorHandler` hoists its existing read above the offline `Observe`) +
   pinned boot registration in `up.go`. Tests: both interleavings, tombstones,
   zero-incarnation, probe-mid-replacement, fixture registration,
   construction-site assertions, wire-compatibility lock, proxy end-to-end
   incarnation rejection.
3. **Slice 3 — IPC + CLI:** POST/DELETE `/routes` handlers + nonce + strict
   JSON (exact key set, `json.Decoder.Token()` stream, overflow-checked
   port conversion) + canonical `?inc=[1-9][0-9]*` conditional delete
   (any violation → 400, no unconditional path), `incarnation` in
   `RouteResponse`, client 201/204 with typed status errors,
   `routes add/remove` in `internal/cli/routes.go` with required
   confirmation (both types, `cmd.InOrStdin()`, decline/EOF abort,
   non-TTY fail-safe, `-y` skips prompt only) + 409 "route changed —
   re-confirm" (never silent retry). Also: exported
   `config.ValidateRouteDomain(domain)` extracted from `ValidateRoute`
   for DELETE domain validation (F1; no `ValidateRoute(domain, 1)`
   workaround). Files: `internal/daemon/daemon.go`,
   `internal/daemon/client.go`, `internal/cli/routes.go`,
   `internal/config/config.go`; tests in `internal/daemon/daemon_test.go`,
   `internal/daemon/stress_test.go`, `internal/cli/routes_test.go`,
   `internal/config/config_test.go`. Tests: §6 Slice-3 matrix. TTY
   accept/decline/EOF → human verification.
4. **Slice 4 — mandatory docs:** ADR-005, `architecture.md`, `routes` Long,
   `noRoutesHint`, CHANGELOG/workspace records.

Each lands as minimal diffs with focused `go test -race` on touched packages;
`bash scripts/test.sh` at the end. Workflow per slice: owner approval →
implement → independent verification → next slice. Final `make test` + owner
gate before any completion claim.
