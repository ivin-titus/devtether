# Discussion Space — Live Route Management (beta.8)

Design decisions approved by the owner across five plan revisions
(rev 5, 2026-11-01). Anything not listed here is out of scope.

## Approved decisions

| # | Decision | Ruling |
|---|---|---|
| D1 | Persistence | Ephemeral, runtime-only. No YAML writes, no file watch/reload. |
| D2 | Health identity | Route incarnations — see implementation_plan §3. |
| D3 | Provenance | `RouteLive RouteType = "live"`; `serviceName = "live"`. |
| D4 | Validation | Shared `config.ValidateRoute(domain, port)`. |
| D5 | Create semantics | Strict create → 409; check+insert atomic inside `Engine`. |
| D6 | IPC shape | Flat `/routes` endpoints; no `/api/*` migration. |
| D7 | Bulk replace | Deferred. |
| D8 | Oversized body | HTTP 413. |
| D9 | `routes add` output | Show route + explicit "ephemeral — removed on daemon restart". |
| D10 | Workspace | beta.8 created after owner's beta.7 verification (Option A). |
| D11 | `routes --check` | Keep documented snapshot semantics. |
| D12 | Incarnation exposure | Internal only — **bounded exception per D15**: the sole user-facing surface is the opaque `incarnation` field in `GET /routes` responses, required for incarnation-aware deletion. Never rendered as a meaningful number in CLI text; never exposed elsewhere. |
| ~~D13~~ | DELETE on static routes (pre-mortem PM-3) | **SUPERSEDED by D14.** Original proposal: restrict IPC DELETE to live routes, 409 on static. Owner rejected — removals must work on both kinds. |
| D14 | Remove semantics (owner, 2026-11-01) | `routes remove` is runtime-only and works on BOTH static and live routes; never edits `devtether.yaml`. EVERY removal requires confirmation by default — warning tailored to route type (static: "YAML unchanged, returns after restart"; live: "ephemeral — must be re-added"). `-y`/`--yes` skips confirmation; no `--force`. Non-TTY without `-y` fails safely. Restart restores configured routes and discards live routes. Persistent change = edit `devtether.yaml`. |
| D15 | Stale-confirmation safety | Incarnation-aware deletion is REQUIRED: `DELETE /routes/{domain}?inc=N` removes only if the stored incarnation is still `N`; mismatch → 409 "route changed — re-confirm"; missing `inc` → 400 (no unconditional DELETE path exists). The CLI supplies the incarnation it confirmed (read from `GET /routes`), so confirming removal of incarnation 10 can never delete a replacement incarnation 11 created concurrently. Justified D12 exception: `RouteResponse` gains an opaque `incarnation` field used only for this check. |

## Design evolution (why the model looks this way)

- **rev 1 → 2:** `Tracker.Forget`/generation-baseline rejected — a request that
  resolves before removal but allocates its generation after re-add slips past.
  Fix: incarnation identity carried on the resolved `Target` snapshot.
- **rev 2 → 3:** cross-handler race — tracker ops from different daemon handlers
  interleave, so `Register`/`Unregister` became conditional and monotone
  (high-water mark + incarnation-scoped unregister). No nested locks.
- **rev 3 → 4:** tombstone gap — `Unregister` returning early when `cur` was
  absent never advanced `hi`, letting a delayed `Register` resurrect a removed
  incarnation. Fix: `Unregister` advances `hi` unconditionally (for non-zero
  inc); `hi` = highest incarnation ever inserted **or** removed.
- **rev 4 → 5:** zero-incarnation hole — `cur[d]` reads 0 for absent domains,
  so `Observe(d, 0, gen, o)` would pass the equality check and write to an
  unregistered domain. Explicit `inc == 0` rejects in `Observe` and `Unregister`.

## Consistency contract (owner-approved)

- Per-domain incarnation high-water mark `hi`; live registration `cur`.
- `Register(domain, inc)` admits only `inc > hi[d]` — monotone, never demotes.
- `Unregister(domain, expectedInc)` rejects 0, tombstones `hi` unconditionally,
  clears `cur/gens/obs` only when `cur[d]==expectedInc`.
- `Observe` admits non-zero `inc == cur[d]` with newer `gen` only.
- Per-handler ordering: engine mutation → tracker op carrying its incarnation.
- `RemoveRoute(domain, expectedInc)` is the sole delete path: inc required,
  returns `(inc, ok)` so `Unregister` gets the right incarnation.

## Pre-mortem outcomes (2026-11-01, isolated audit session)

Verdict: sound to implement. Folded into this workspace:
- PM-1: ADR-005 principle 4 + `routes` Long must be amended as part of the
  feature — doc updates are mandatory gates, not follow-ups.
- PM-2: `health.Target.Incarnation` plumbing required at BOTH construction
  sites (`daemon.routeTargets`, `cli/up.go startupTargets`).
- PM-3: static-DELETE gap → resolved by owner ruling D14 (supersedes D13).
- PM-4/5/6: accepted risks — unbounded `hi` tombstones (documented), inherent
  observe-drop window (code comment), `Register`/`Unregister` pairing at every
  call site (compile-enforced + tests).

## Persistent configuration vs ephemeral runtime state

`devtether.yaml` is authoritative for persistent configured routes; the
running daemon's router holds the currently active runtime state. The two
diverge only via runtime mutations — and runtime mutations NEVER write back
to YAML, so divergence always resolves at restart, back to YAML.

| Action | Effect on daemon | Effect on `devtether.yaml` | After `down && up` |
|---|---|---|---|
| `routes add app.localhost 3000` | Inserts `type:"live"` route immediately (DNS+proxy effective next request) | Never written | Route is gone |
| `routes remove app.localhost` (live) | Deletes after confirmation; 404/NXDOMAIN | Never written | N/A (route not in YAML) |
| `routes remove app.localhost` (static) | Deletes immediately after confirmation | Never written — YAML still lists it | **Route returns** unless removed from YAML |
| daemon restart | Re-loads YAML | — | YAML routes restored; live routes discarded |
| Edit `devtether.yaml` | No live reload (out of scope) | The only persistent change | Applied on next `up` |

### Confusing cases, spelled out

1. **Remove a static route, then re-add the same domain live.** Sequence:
   `remove` (confirmed; engine drops static inc10) → `add` same domain (live
   inc11). The domain is now a *live* route. On restart the YAML static entry
   is restored — the live add is discarded. This is intended: removal and
   addition are independent runtime mutations.
2. **Add live on an existing static domain.** Rejected 409 (strict create):
   the domain is occupied; first `remove` it (runtime) or edit YAML.
3. **Remove static, restart before re-adding.** Route silently returns at
   boot — exactly what the confirmation warning says.
4. **Confirmed-then-raced removal.** Prompt shows the route being removed;
   if another client re-adds the domain before the confirmed DELETE lands,
   the incarnation mismatch yields 409 and the replacement survives (D15).
5. **Non-interactive usage.** Scripts must pass `-y`; without a TTY and
   without `-y` the remove fails safe rather than guessing.

### Trade-offs and rationale

- Allowing runtime removal of static routes is a *temporary disablement*
  feature: fast to test a domain offline without editing config. The cost —
  "it comes back after restart" — is paid explicitly via the confirmation
  warning, not hidden. Live-route removal also confirms (a mis-typed domain
  is irreversible within the running daemon), with a lighter warning.
- The unconditional DELETE was dropped entirely (D15 re-evaluation): an
  unconditioned delete is exactly the unsafe path a stale confirmation or
  concurrent client could ride. Requiring `?inc=` costs one extra field
  read in the CLI and removes the whole class of accidental-replacement
  deletes; raw API callers get a clear 400 telling them to pass the
  incarnation.
- Persistence stays config-owned: one file to edit, one restart to apply.
  The daemon never writes user config (D1), so YAML remains reviewable,
  diffable, and immune to daemon-state drift.
- ~~Conditional DELETE reuses the incarnation mechanism — the only API
  surface change is an opaque `incarnation` field in `GET /routes` and an
  optional `?inc=` parameter (unconditional delete remains valid).~~
  **Superseded wording (corrected 2026-11-01):** `?inc=N` is MANDATORY for
  every DELETE caller; a missing incarnation returns 400 and no
  unconditional deletion path exists — including for live routes and raw
  API use. The mechanism is unchanged (one opaque field + one query param),
  only its optionality was dropped.

## Boundaries (owner-stated)

No YAML persistence/watchers; no Engine 2/3/4 work; no `/api` prefix migration;
no unrelated refactors or new dependencies; preserve DNS, proxy, streaming,
WebSocket, IPC security, daemon lifecycle. Incarnation stays internal,
except the opaque `incarnation` field in `GET /routes` required for
incarnation-aware deletion (D15 boundary).
`feature/devin` → `develop` PR flow; owner squash-merges. No commit/push/merge
or tracker completion without owner approval.
