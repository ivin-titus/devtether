---

name: devtether-change
description: Execute a DevTether change within its release workspace using separated planning, implementation, verification, human approval, and Git-planning stages. Use for feature work, bug fixes, refactors, and other code changes.

--- 

# DevTether Change Workflow

Use this workflow for implementation work.

The release workspace is the operational context. Do not replace or duplicate its artifacts.

## 1. Establish context

1. Read `docs/workspace/README.md`.
2. Identify the active/relevant release.
3. Read that release's `tracker.md`.
4. Read the relevant `implementation_plan.md`, `discussion_space.md`, `audit_report.md`, or other artifacts.
5. Read the applicable `docs/adr/`, `docs/architecture.md`, and `docs/engineering-standards.md`.
6. Read the `ponytail/SKILL.md` to establish the "Lazy Senior Dev" mindset.
7. Read `CONTRIBUTING.md` when contribution or AI-accountability requirements are relevant.

Do not begin implementation while the requested work and release scope are ambiguous.

## 2. Pre-mortem

Before changing code, perform an independent design review.

Prefer an isolated analysis subagent when available.

The review must compare the current phase against:

* applicable ADRs;
* architecture;
* engineering standards;
* existing behavior;
* relevant dependencies and boundaries.

The reviewer should identify:

* incorrect assumptions;
* missing requirements;
* architectural conflicts;
* edge cases;
* likely regressions;
* unnecessary complexity.

Do not treat implementation intent as proof that the proposed approach is correct.

## 3. Update the plan

Before implementation, update the appropriate release artifact when the current plan is incomplete or has materially changed.

Prefer the existing:

* `tracker.md` for execution state;
* `implementation_plan.md` for implementation steps;
* `discussion_space.md` for unresolved design decisions;
* `audit_report.md` for discovered findings.

Do not create another planning document when an existing artifact already owns the information.

## 4. Implementation

Implementation must be performed as a distinct responsibility from planning and verification.

Prefer an isolated implementation subagent when available.

The implementer must:

* follow the approved plan;
* make the smallest correct change (minimal lines of code, one logical thing at a time without rushing);
* preserve existing architecture and boundaries;
* exclusively use native file-editing tools (never shell/python scripts for file I/O);
* add tests for new behavior/modules (do not skip mandated tests);
* update tests affected by changed behavior;
* avoid unrelated refactors and speculative abstractions (No AI Slop);
* never leak ephemeral release terminology (e.g., "Phase 1") into source code, comments, or permanent documentation.

Record AI identity according to the workspace convention.

## 5. Independent verification

After implementation, perform a fresh verification pass.

Prefer a separate verification subagent.

Re-read every changed file and inspect relevant upstream/downstream dependencies.

Compare the result against:

* the current implementation-plan phase;
* applicable ADRs;
* architecture;
* engineering standards.

Check explicitly for:

* missing requirements;
* plan drift;
* logical errors;
* regressions;
* architecture violations;
* unintended coupling;
* new technical debt;
* upstream/downstream dependency problems;
* incomplete or misleading tests.

Run:

```bash
make test
```

A passing test suite is necessary evidence but never the sole basis for declaring the change correct.

## 6. Human verification gate

The system owner must review the implementation before it is marked complete.

Do not mark the relevant `tracker.md` task complete merely because automated verification passes.

Provide concise manual verification steps that allow the system owner to confirm the behavior.

Do not represent human verification as completed when it has not happened.

## 7. Git planning

After verification, update the relevant Git-planning artifact if the release workflow uses one.

Before proposing commits:

```bash
git status -s
git diff
```

Inspect the complete relevant diff and Git state.

Commit planning must reflect the verified implementation rather than the original intention.

Do not create or rewrite history merely to make the workspace look complete.

## 8. Completion rule

A change is complete only when:

1. the implementation matches the approved scope;
2. tests are present and pass;
3. independent verification is complete;
4. relevant documentation remains consistent;
5. the system owner has verified the result;
6. the workspace tracker accurately reflects the real state.

When any gate is incomplete, leave the work explicitly incomplete and record what remains.
