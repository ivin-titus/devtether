# DevTether Project Workspace

Welcome to the DevTether Project Workspace! This directory (`docs/workspace/`) serves as the operational headquarters for both human contributors and AI agents managing the project's roadmap, backlog, and immediate execution state.

---

## 1. The Workspace vs The PRD (Source of Truth)

To maintain a clean and scalable project, we strictly enforce where information lives:

* **The Codebase (`docs/PRD.md`, `docs/adr/`) -> Technical & Strategic Truth.** 
  The PRD defines the long-term project vision. ADRs define the architecture. 
* **The Workspace (`docs/workspace/`) -> Operational Truth.** 
  This folder defines the **immediate execution steps** (what we are coding *right now*). 
* **The Changelog (`CHANGELOG.md`) -> Historical Truth.** 
  Completed releases are strictly managed by GitHub Tags and the exhaustive `CHANGELOG.md`.

---

## 2. Active Release Pointers

* **Currently Live Release:** `v2.0.0-beta.5` (See [`CHANGELOG.md` at root](../../CHANGELOG.md))
* **Active Execution Release:** `v2.0.0-beta.6`
* **Next Major Release:** `v2.0.0-beta.7`

---

## 3. AI Contribution Liability

We welcome contributions generated with the help of AI agents. However, we enforce a strict **Human Accountability Policy** mirroring the Linux DCO (see [`ADR-010`](../adr/010-ai-contribution-liability.md)).

* AI agents are *tools*, not authors.
* The human invoking the agent assumes 100% legal, functional, and security liability.
* All AI outputs must be audited against [`docs/engineering-standards.md`](../engineering-standards.md) before merging.

---

## 4. Workspace Anatomy & Templates

The workspace is organized into release folders (e.g., `v2.0.0-beta.6/`). Each release folder contains a `tracker.md` and an `artifacts/` sandbox for ephemeral planning.

We enforce exact Markdown templates for these files to maintain absolute consistency across all human and AI contributors.

### A. `tracker.md` (Execution Tracker)
**Location:** `docs/workspace/vX.X.X/tracker.md`
```markdown
# Release vX.X.X
**Status:** [Planned | In Progress | Completed]
**Target:** MM/DD/YYYY

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->
- [ ] **[Category] Task Title**
  *Assignee:* `@github-username (via Agent:Model)`
  *Priority:* [Critical | High | Medium | Low]
  *Description:* Clear description of the work to be done.

## Scope Discoveries (Deferred / Backlog)
*(If an audit or idea creates new problems mid-sprint, log it here. Do NOT derail the current release.)*
- [ ] **[Category] Task Title**
  *Description:* Description of the discovered issue.
```

### B. `discussion_space.md` (Architecture & Brainstorming)
**Location:** `docs/workspace/vX.X.X/artifacts/discussion_space.md`
```markdown
# [Release or Broad Topic] - Discussion Space
*(This document is append-only. It hosts ongoing brainstorms for multiple issues within a release cycle.)*

## Discussion: [Specific Issue/Feature 1]

### 1. Problem Statement
*What are we trying to solve? Why does this need to be addressed?*

### 2. Proposed Solutions
*Detail the different architectural approaches, weighing the trade-offs of each.*
* **Approach A:** Pros/Cons
* **Approach B:** Pros/Cons

### 3. Open Questions & Edge Cases
*Are there backward compatibility issues? What if X fails?*

### 4. Resolution
*The final decision moving forward. This leads directly into the Implementation Plan.*

---

## Discussion: [Specific Issue/Feature 2]
*(Append new discussions here)*
```

### C. `implementation_plan.md` (Execution Roadmap)
**Location:** `docs/workspace/vX.X.X/artifacts/implementation_plan.md`
```markdown
# [Feature/Bug Name] - Implementation Plan

## Pre-Flight Checklist
- [ ] Codebase audited for related logic and edge cases.
- [ ] Changes verified against `docs/engineering-standards.md`.
- [ ] Architectural decisions comply with `docs/adr/`.

## Phase 1: [Phase Name]
### Subphase 1.A: [Goal Description]
- **Step 1:** Modify `path/to/file.go` to do X.
- **Step 2:** Write unit tests for X in `path/to/file_test.go`.
- **Step 3:** Run `scripts/test.sh` to verify.

## Phase 2: [Phase Name]
### Subphase 2.A: [Goal Description]
- **Step 1:** Implement Y.
```

### D. `audit_report.md` (Security & Code Reviews)
**Location:** `docs/workspace/vX.X.X/artifacts/audit_report.md`
```markdown
# Audit Report: [Component/Scope]

**Date:** MM/DD/YYYY
**Auditor:** `@github-username (via Agent:Model)`

## 1. Executive Summary
*High-level overview of the audit findings (e.g., "Critical mutex vulnerability found in DNS resolver").*

## 2. Methodology
*How the audit was conducted (e.g., manual review, static analysis, fuzzing).*

## 3. Detailed Findings
### Finding 1: [Vulnerability/Issue Name]
- **Severity:** [Critical | High | Medium | Low]
- **Location:** `path/to/file.go:LineNumber`
- **Description:** Explanation of the flaw.
- **Remediation Plan:** Proposed fix.
```

### E. `agent_diary.md` (AI Scratchpad)
**Location:** `docs/workspace/vX.X.X/artifacts/agent_diary.md`
```markdown
# Agent Diary - Scratchpad

*(This file is append-only for the duration of the task. It is a messy workspace for diagrams, math, or raw logs.)*

## Dump: MM/DD/YYYY HH:MM
` ` `text
(Raw stack trace or compiler error goes here)
` ` `

## Diagram: Auth Flow
` ` `mermaid
sequenceDiagram
    Client->>Proxy: Request
    Proxy-->>Client: Response
` ` `
```

---

## 5. How to Contribute

To begin work:
1. Read the [Contribution Guidelines](../../CONTRIBUTING.md).
2. Navigate to the active execution tracker (e.g., [`v2.0.0-beta.6/tracker.md`](v2.0.0-beta.6/tracker.md)).
3. Claim an unassigned task by opening a Pull Request that updates the `tracker.md` to include your `@github-username (via Agent:Model)`.
