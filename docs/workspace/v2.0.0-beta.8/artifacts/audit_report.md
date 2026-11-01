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

## §2 Findings log

| ID | Severity | Location | Finding | Status |
|---|---|---|---|---|
| B8-1 | Low | `scripts/test.sh` | Committed mode 100644; `make test` cannot exec it. Local runs use `bash scripts/test.sh`; CI unaffected (calls `go test` directly). One-line `chmod +x` fix — tracked in tracker as separate hygiene item. | Open |
