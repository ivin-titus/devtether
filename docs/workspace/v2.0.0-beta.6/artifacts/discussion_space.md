# Release v2.0.0-beta.6 - Discussion Space
*(This document is append-only. It hosts ongoing brainstorms for multiple issues within a release cycle.)*

## Discussion: UI Polish & Theming

### 1. Problem Statement
The error pages (404 Route Not Found and 502 Bad Gateway) lack official branding, and they force a dark theme regardless of the user's OS/device preferences.

### 2. Proposed Solutions
* **Error Page Logos:** Embed the official `devtether_logo.png` (`~6kb`) from the `.assets/` directory into both the 404 and 502 HTML overlays.
* **System-Aware Theming:** Update the CSS to use `@media (prefers-color-scheme: dark)` and `light` media queries. Instead of forcing a dark theme, the error pages should dynamically match whatever theme the user's OS/device is currently using.

### 3. Open Questions & Edge Cases
* Should the logo be served from a local file server, or embedded directly into the HTML?

### 4. Resolution
Embed the `.assets/devtether_logo.png` via Base64 into the 404/502 HTML overlays to adhere to the Single Binary architecture. Update CSS to use `@media (prefers-color-scheme)` for dynamic theming.

---

## Discussion: Remaining Deferred Bugs (Audit Backlog)

### 1. Problem Statement
There are 9 remaining deferred flaws from the security audit across the proxy, CLI, logging, and DNS engines that need to be resolved to establish a mathematically perfect Engine 1.
1. Proxy Loop 404 on Init Default Config (HIGH)
2. Inconsistent IPv6 Bracket Normalization in Proxy (MEDIUM)
3. Zombie Signal Handler (Unkillable Process on Hang) (MEDIUM)
4. Hardcoded ANSI Corruption in Logging Pipelines (MEDIUM)
5. Silent Masking of Daemon IPC Errors (LOW)
6. Non-Deterministic Output (`devtether routes`) (LOW)
7. Inefficient DNS Record Parser on Hot Path (LOW)
8. Loss of Error Severity Context in Standard Logs (LOW)
9. `--config` Flag Silently Ignored by `devtether routes` (LOW)

### 2. Proposed Solutions
* **Proxy Loop 404:** Change the default commented port in `init.go` (currently `api.localhost: 8080`) to something DevTether doesn't use in its fallback chain (e.g., `8081` or `3000`).
* **IPv6 Bracket Normalization:** Explicitly strip `[` and `]` brackets from the host string using `strings.TrimPrefix` and `strings.TrimSuffix` during `sanitizeHost`.
* **Zombie Signal Handler:** Call `signal.Stop(sigCh)` immediately after the first signal so subsequent interrupts trigger default OS termination.
* **Hardcoded ANSI Corruption:** Import `golang.org/x/term` and check if the output is a TTY. If not, strip ANSI to prevent log parser corruption.
* **Silent Masking of IPC Errors:** Only fall back to `devtether.yaml` if the error is explicitly an `ECONNREFUSED`/Not Exist equivalent. Otherwise, fail visibly.
* **Non-Deterministic Output:** Sort the `[]RouteResponse` array alphabetically by `Domain` before printing it.
* **Inefficient DNS Record Parser:** Instantiate the `dns.A` struct directly instead of using the string parser.
* **Loss of Error Severity Context:** Prepend `ERROR:` or colorize the prefix specifically for `slog.LevelWarn` and above in the standard handler.
* **`--config` Flag Ignored:** If the `--config` flag is explicitly provided, bypass the daemon check and read the file directly (or warn the user).

### 3. Open Questions & Edge Cases
* Do any of these fixes violate existing ADRs or Engineering Standards? (See audit for restrictions on DNS efficiency, TOCTOU, and ANSI discipline).

### 4. Resolution
All proposed fixes have been reviewed and mapped strictly to the workspace implementation plan. All strict constraints (such as `term.IsTerminal` for ANSI, and `strings.TrimPrefix`/`strings.TrimSuffix` for IPv6) have been verified.

---

## Discussion: AI Agent Standardization & Project Context Alignment

### 1. Problem Statement
The current AI agent workflow occasionally leads to friction, such as overengineering, AI slop, skipping tests, or failing to adhere to strict engineering standards. While we recently added strict rules (e.g., banning ephemeral "Phase" terminology, enforcing native tools, demanding minimal step sizes), there is room to enhance the agent context by adopting battle-tested philosophies from `ivin-lab`, `Project-Yuki`, and `Hori-Z`. Furthermore, agents need to ask questions when stuck rather than guessing, and they must strictly adhere to the artifact templates defined in `docs/workspace/README.md`.

### 2. Proposed Solutions

#### A. Direct Code & Documentation Updates (Pending Execution)
- **AGENTS.md Linkage:** Add an explicit link to `docs/engineering-standards.md` in the "No Overengineering (AI Slop)" section of `AGENTS.md` for immediate context.
- **Redefining Tech Debt:** Update `devtether-audit/SKILL.md` to explicitly define "anything that's against engineering standards or ADRs" as new technical debt.
- **Anti-Guessing Rule:** Add a strict rule to `AGENTS.md` and `devtether-change`: *If stuck or ambiguous, ask necessary questions instead of implementing something randomly.*
- **Artifact Template Compliance:** Add a rule to all skills mandating that any generated artifacts (e.g., trackers, diaries, audit reports) MUST strictly conform to the templates outlined in `docs/workspace/README.md`.

#### B. Borrowed Philosophies (The "Lazy Senior Dev" & Role Specialization)
Based on `Project-Yuki/PONYTAIL_LITE.md`, `ivin-lab/AGENTS.md`, and `Hori-Z/AGENTS.md`, we should adopt the following principles into our DevTether ecosystem, avoiding overengineering the agent structure itself:
- **The "Lazy Senior Dev" Principle (YAGNI):** The best code is the code never written. Agents should climb the "ladder" before writing code: Does it need to be built? Can we reuse standard library? Can it be one line? Only then write the minimum code.
- **Prefer Deletion Over Addition:** Shorter, working diffs win. Boring over clever.
- **Specialized Roles (Not Monolithic Prompts):** Rather than one massive prompt, we can utilize subagents for specific domains (e.g., `Principal Architecture Agent` for ADR checks, `Change Safety Agent` for blast radius, `Code Editor Agent` for minimal file I/O). DevTether already uses `devtether-change` and `devtether-audit`, but we can refine these roles to be sharper.

### 3. Open Questions & Edge Cases
- Do we want to officially define specific subagent personas (like the `Change Safety Agent`) in `AGENTS.md`, or just keep the general "Implementation" and "Auditor" dichotomy to avoid overcomplicating things?
- Should the "Lazy Senior Dev" mindset be its own standalone skill file (e.g., `devtether-philosophy/SKILL.md`), or baked directly into the `AGENTS.md` root rules?

### 4. Resolution
*Pending user review of these proposed additions.*
