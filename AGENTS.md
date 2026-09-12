# DevTether Agent Guide

DevTether is a human-owned project that permits AI-assisted development under strict human accountability.

This file is a routing layer. Do not duplicate project documentation here.

## Start here

Before doing project work, read:

- `docs/workspace/README.md` — operational workflow and release/workspace model.
- `docs/engineering-standards.md` — mandatory engineering quality bar.
- `docs/architecture.md` — architecture and system boundaries.
- `docs/adr/` — accepted architectural decisions relevant to the task.
- `CONTRIBUTING.md` — contribution and AI-accountability requirements.

Then identify the relevant release under:

`docs/workspace/<release>/`

Use that release's `tracker.md` and relevant artifacts as the operational context.

## Canonical sources

- **Code + architecture:** repository code, `docs/architecture.md`, `docs/adr/`
- **Engineering rules:** `docs/engineering-standards.md`
- **Execution state:** `docs/workspace/<release>/`
- **Release history:** `CHANGELOG.md` and Git tags

Do not copy canonical documentation into agent instructions.

## AI accountability

AI agents are tools, not authors.

The human contributor remains responsible for correctness, security, licensing, architectural compliance, testing, and the final submitted change. AI-generated changes must be independently reviewed before acceptance.

Do not optimize for code volume, automation, or cleverness. Prefer the smallest correct change.

Never silently bypass an ADR, engineering standard, release boundary, verification gate, or human approval.

## Tooling & Execution Discipline

When operating in this repository, agents must adhere to strict execution discipline:

- **Native Tool Priority:** Always use your native file-editing tools (e.g., `multi_replace_file_content`, `replace_file_content`, `write_to_file`) for file I/O. **Never** use shell scripts (e.g., `echo >>`, `cat`, `sed`) or Python scripts to read or write files.
- **Minimal Step Size:** Do not rush. Make minimal lines of code changes, one logical thing at a time. If a task requires touching multiple files, break it down and verify incrementally.
- **No Overengineering (AI Slop):** Do not generate bloated interfaces, premature abstractions, or massive inline string literals. If you are generating a massive block of boilerplate, you are doing it wrong. Find the native, idiomatic Go approach. (See [docs/engineering-standards.md](docs/engineering-standards.md))
- **Anti-Guessing Rule:** If you are stuck, lack context, or the requirements are ambiguous, **STOP and ask necessary questions**. Do not implement something randomly or blindly guess.

## Standard workflows

When implementing a change, use:

`docs/workspace/agent-skills/devtether-change/SKILL.md`

When independently reviewing or auditing a change, use:

`docs/workspace/agent-skills/devtether-audit/SKILL.md`

To enforce the "Lazy Senior Dev" (YAGNI/minimalism) rules across both implementation and auditing, use:

`docs/workspace/agent-skills/ponytail/SKILL.md`

## Repository checks

The normal repository test entry point is:

```bash
make test
```

A passing test suite is required evidence, not proof of correctness. Review affected behavior and dependencies independently.

## Workspace discipline

Work primarily inside the appropriate release workspace.

Keep planning, execution state, findings, and historical artifacts in their existing canonical locations.

Do not create parallel AI-specific project-management systems.

Never use ephemeral sprint/release terminology (e.g., "Phase 1", "Phase 2") outside of `docs/workspace/`. Code comments, godocs, and public documentation like ADRs or `architecture.md` must strictly use permanent architectural terms (e.g., "Engine 1", "Layer 1").