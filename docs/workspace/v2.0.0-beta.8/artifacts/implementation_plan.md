# Implementation Plan — Live Route Management (beta.8)

Design rev 5, owner-approved architecturally. Baseline `develop` @ `6fac500`.
Code-verified against `internal/{router,health,daemon,proxy,cli,config}`.

## 1. What changes (minimal diffs, Ponytail-applied)

1. `internal/config/config.go` — extract exported `ValidateRoute(domain, port)`;
   `validateRoutes` calls it. Identical rules for file and IPC paths.
2. `internal/router/router.go` — `nextInc` counter; `ErrRouteExists`; atomic
   `AddRoute` → `(uint64, error)`; `RemoveRoute` → `(uint64, bool)`;
   `Incarnation` on `Target`/`RouteView`; `RouteLive`.
3. `internal/health/health.go` — `cur`/`hi` maps; `Register`/`Unregister`;
   `Observe`/`ObserveNow` gain `inc`; `Target.Incarnation`.
4. `internal/daemon/daemon.go` — `POST /routes` + `DELETE /routes/{domain}`;
   shared nonce check; strict JSON; `Register`/`Unregister(domain, inc)` with
   returned incarnations; incarnation-required delete via `?inc=` (D15 — no
   unconditional path); `incarnation` field on `RouteResponse`; boot
   registration of static routes.
5. `internal/daemon/client.go` — `AddRoute`/`RemoveRoute(domain, inc)`; nonce;
   201/204 expected; daemon body surfaced in errors.
6. `internal/cli/routes_mutate.go` — `routes add <domain> <port>` /
   `routes remove <domain> [-y|--yes]`; confirmation required for EVERY
   remove, warning tailored to route type (static: YAML unchanged, returns
   after restart; live: ephemeral, must be re-added); non-TTY without `-y`
   fails safe; no `--force`; daemon-down handling; ephemeral notice on add.
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

- Proxy: `targetCtxKey` already carries resolved `Target`; handlers read
  `target.Incarnation` alongside `genCtxKey`.
- Startup check + `routes --check`: `RouteView.Incarnation` →
  `health.Target.Incarnation` at BOTH `health.Target` construction sites —
  `daemon.routeTargets()` AND `cli/up.go startupTargets()` (PM-2; missing one
  silently keeps every route `unknown`).
- Boot: register each static route (`GetAllRoutes` → `Register`).

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

## 5. IPC contract

| Method | Path | Body | Success | Errors |
|---|---|---|---|---|
| POST | `/routes` | `{"domain":"app.localhost","port":3000}` | `201` + `RouteResponse` | 400, 403, 409, 413, 405 |
| DELETE | `/routes/{domain}` | — | `400` — `inc` required | no unconditional delete (D15) |
| DELETE | `/routes/{domain}?inc=N` | — | `204` only if stored inc still `N` | 400, 403, 404, 405, 409 on mismatch ("route changed — re-confirm") |

- Nonce required (`X-Devtether-Nonce` → 403). Bodies bounded:
  `http.MaxBytesReader(w, r.Body, 8<<10)` → 413 on `*http.MaxBytesError`.
- Strict JSON: DisallowUnknownFields → one object → second decode must EOF →
  field validation → only then mutate.
- `ValidateRoute` on the literal domain before any normalization.
- Client: 201/204 expected; other statuses carry status + trimmed body.
- `GET /routes` includes live routes (`type:"live"`, `serviceName:"live"`)
  plus an opaque `incarnation` field — the single justified D12 exception,
  consumed only by the CLI's conditional DELETE (D15).

## 6. Test matrix (deterministic, no sleeps)

- **Interleaving A:** `RemoveRoute(d,10)` → `AddRoute` → `Register(11)` → `Unregister(10)` → `cur=11`, inc-10 obs rejected.
- **Interleaving B:** `AddRoute` (no Register) → `RemoveRoute(d,10)` → `Unregister(10)` → `AddRoute` → `Register(11)` → delayed `Register(10)` → `cur=11`.
- **Tombstone resurrection:** `Unregister(d,10)` with `cur` absent → `hi=10` → `Register(d,10)` rejected.
- **Zero incarnation:** `Observe(d,0,gen,o)` false + snapshot unchanged; `Unregister(d,0)` mutates nothing.
- Concurrent creates / removes / add+remove (WaitGroup + start gate, `-race`).
- Resolved-then-removed request obs rejected (proxy ctx target inc).
- Probe completing after replacement (`checkAll` injected probe + channel).
- In-flight request across removal; new requests 404; DNS NXDOMAIN.
- Client 201/204 via real socket (`XDG_RUNTIME_DIR` → `MkdirTemp("/tmp","dt")`, macOS `sun_path`).
- JSON edges: syntax/unknown fields/wrong types/trailing data/>8KB/empty → 400/413, engine unchanged. Nonce 403s.
- `ValidateRoute` parity table (file vs IPC acceptance set).
- DELETE on static AND live domains → 204 after confirmation; engine removes
  runtime-only, YAML untouched (D14). Confirmation required for both; warning
  tailored to type.
- Stale-confirmation race (D15): CLI confirms inc10, a concurrent add re-creates
  the domain as inc11, `DELETE ?inc=10` → 409 and inc11 survives — verified in
  scripted order, no sleeps.
- `DELETE /routes/{domain}` without `?inc=` → 400; no unconditional path.
- Remove CLI: prompt accept/decline on TTY for both types; `-y/--yes` bypasses;
  non-TTY without `-y` exits nonzero with guidance; no `--force`; YAML never
  touched.
- Boot registration: startup `CheckAll` observations apply against static-route
  incarnations (both `startupTargets` and `routeTargets` carry `Incarnation`).
- Lint: `NewRequestWithContext`, `net.ListenConfig`, errcheck, govet shadow, `ReadHeaderTimeout`.

## 7. Execution slices (each independently reviewable)

Post-pre-mortem slice plan (pre-mortem complete, verdict: sound to implement):

1. **Slice 1 — config + router:** `config.ValidateRoute` extraction; `nextInc`,
   `ErrRouteExists`, atomic `AddRoute`→inc, `RemoveRoute`→(inc,ok),
   `Incarnation` fields, `RouteLive`. Tests: concurrent create/remove, parity.
2. **Slice 2 — health incarnations:** `cur`/`hi`, `Register`/`Unregister`,
   `Observe`/`ObserveNow` signatures, `health.Target.Incarnation`, both
   construction sites + proxy ctx plumbing + boot registration. Tests: both
   interleavings, tombstones, zero-incarnation, probe-mid-replacement.
3. **Slice 3 — IPC + CLI:** POST/DELETE `/routes` handlers + nonce + strict
   JSON + incarnation-required `?inc=` delete (missing → 400), `incarnation` in
   `RouteResponse`, client 201/204, `routes add/remove` with required
   confirmation (both types) + `-y/--yes` + non-TTY fail. Tests: socket-level
   client, JSON edges, stale-confirmation 409, missing-inc 400, in-flight
   request, DNS.
4. **Slice 4 — mandatory docs:** ADR-005, `architecture.md`, `routes` Long,
   `noRoutesHint`, CHANGELOG/workspace records.

Each lands as minimal diffs with focused `go test -race` on touched packages;
`bash scripts/test.sh` at the end. Workflow per slice: owner approval →
implement → independent verification → next slice. Final `make test` + owner
gate before any completion claim.
