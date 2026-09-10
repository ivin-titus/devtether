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
