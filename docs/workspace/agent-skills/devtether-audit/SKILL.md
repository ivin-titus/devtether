---

name: devtether-audit
description: Independently audit a DevTether change or release workspace for correctness, regressions, architectural violations, missing requirements, and documentation drift. Use after implementation or when explicitly asked to review existing work.

---

# DevTether Audit Workflow

This workflow is for independent verification, not implementation.

Do not modify source code while auditing unless the task explicitly authorizes documentation-only changes.

## 1. Establish the review scope

1. Read `docs/workspace/README.md`.
2. Identify the relevant release.
3. Read its `tracker.md`.
4. Read the relevant implementation plan and prior audit findings.
5. Read applicable ADRs, `docs/architecture.md`, and `docs/engineering-standards.md`.
6. Read the `ponytail/SKILL.md` to ensure the implementer didn't violate YAGNI or write AI slop.
7. Inspect the actual Git state and relevant diff/history.

Review the resulting implementation, not the author's intent.

## 2. Audit correctness

Check:

* requirement completeness;
* logical correctness;
* edge cases;
* state/lifecycle behavior;
* error handling;
* concurrency where applicable;
* resource ownership;
* upstream/downstream dependencies;
* compatibility;
* regressions;
* unintended coupling.

Inspect code outside the modified files when changed behavior can affect it.

## 3. Audit project consistency

Compare the result against:

* current implementation-plan phase;
* applicable ADRs;
* architecture;
* engineering standards;
* tests;
* workspace tracker.

Look specifically for:

* plan drift;
* architecture violations;
* duplicated or contradictory logic;
* new technical debt (anything against engineering standards/ADRs, AI slop, overengineering, bloated boilerplate);
* stale documentation;
* leaked ephemeral terminology (e.g., "Phase 1") in source code or permanent documentation;
* incomplete tests (or tests explicitly mandated by ADRs that were lazily skipped);
* misleading task status.

## 4. Verification

Run:

```bash
make test
```

Then perform reasoning beyond the test suite.

Green tests are evidence, not proof.

When useful, run focused tests or inspect runtime behavior for important edge cases.

## 5. Findings

Report concrete findings with:

* severity;
* location;
* evidence;
* failure mechanism;
* impact;
* confidence.

Distinguish clearly between:

* confirmed defects;
* credible risks;
* insufficient evidence.

Do not invent certainty.

## 6. Workspace updates

Write findings into the appropriate existing release audit artifact.

All generated artifacts (e.g., trackers, diaries, audit reports) MUST strictly conform to the templates outlined in `docs/workspace/README.md`.

Do not create a parallel audit system.

Do not mark work resolved unless the available evidence supports that conclusion.

The purpose of the audit is to protect the system owner from false confidence, not to make the implementation appear finished.

