# DevTether Project Workspace

This directory is the operational workspace for release execution, audits, and
architectural discussions.

## 1. Source-of-truth boundaries

- **Repository code + tests:** current implementation behavior.
- **`docs/architecture.md` + accepted ADRs:** current architecture and accepted decisions.
- **`docs/engineering-standards.md`:** engineering rules.
- **Workspace tracker:** current execution state for the active release.
- **CHANGELOG.md:** release history.
- **Workspace audit/diary/discussion files and `.ideas/`:** historical reasoning, deferred designs, and release-era evidence. These are **not authoritative current implementation sources** and may contain superseded decisions.

When an historical workspace record conflicts with the current source tree, current source and current accepted documentation take precedence.

## 2. Active release pointers

- **Current release:** `v2.0.0-beta.7` — see [tracker](v2.0.0-beta.7/tracker.md)
- **Previous release:** recorded in [CHANGELOG.md](../../CHANGELOG.md); older release workspaces may not be retained in this branch.
- **Next release:** `v2.0.0-beta.8` is a future beta line, not currently planned.

## 3. AI accountability

AI-assisted contributions remain subject to the human accountability policy in
[ADR-010](../adr/010-ai-contribution-liability.md).

- AI agents are tools, not authors.
- The human contributor remains responsible for correctness, security, licensing, architecture, and verification.
- Workspace task attribution uses the project convention for AI-assisted work.

## 4. Workspace anatomy

Active release folders contain a tracker and supporting artifacts:

- `tracker.md` — current execution state.
- `artifacts/implementation_plan.md` — planned execution steps.
- `artifacts/discussion_space.md` — design discussion and decisions.
- `artifacts/audit_report.md` — audit findings and verification history.
- `artifacts/agent_diary.md` — raw agent scratchpad/history.

Deferred ideas live under `docs/workspace/.ideas/`.

## 5. Historical-record rule

Append-only audit, diary, and discussion files preserve what was known or decided
at that point in time. A later source/documentation change does not rewrite that
history. Instead, later work should add an explicit supersession note when an
old conclusion is no longer current.

## 6. How to contribute

1. Read [CONTRIBUTING.md](../../CONTRIBUTING.md).
2. Read [Engineering Standards](../engineering-standards.md).
3. Read the relevant accepted ADRs and [Architecture](../architecture.md).
4. Use the current release tracker for execution state.
5. Verify implementation claims against the source tree before treating workspace history as current truth.
