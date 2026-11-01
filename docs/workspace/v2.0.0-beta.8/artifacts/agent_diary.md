# Agent Diary — v2.0.0-beta.8

Append-only working record. Entries newest-first.

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
