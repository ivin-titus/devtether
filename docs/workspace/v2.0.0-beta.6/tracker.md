# Release v2.0.0-beta.6
**Status:** In Progress
**Target:** 09/10/2026

## Active Tasks
<!-- Valid Categories: [Bug Fix], [Feature], [Chore], [Security], [Docs] -->

- [x] **[Chore] Define Open-Source AI Contribution Protocols**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Critical
  *Description:* Draft ADR-010 for AI Liability, update CONTRIBUTING.md with AI rules, expand engineering standards for mandatory auditing, and scaffold the in-repo Workspace architecture to manage multi-agent development. (Completed)

- [x] **[Chore] Bulletproof AI Agent Skills & Philosophy (Lazy Senior Dev)**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Description:* Import the 'Lazy Senior Dev / YAGNI' philosophy as a `ponytail` skill. Overhaul `AGENTS.md` and standard workflow skills to enforce minimal step sizes, ban shell scripts for I/O, mandate test coverage checks, and enforce native artifact templates.

- [x] **[Bug Fix] Proxy Loop 404 on Init Default Config**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* High 
  *Description:* Change the default generated proxy route in `init.go` to `api.localhost: 3000` to prevent an infinite loop when the daemon runs unprivileged and falls back to port `8080`. 

- [x] **[Feature] Embed Base64 Logo in Error Pages**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Medium 
  *Description:* Embed `.assets/devtether_logo.png` via Base64 into the 404/502 HTML overlays.

- [x] **[Feature] Adapt Proxy Error CSS for OS Theming**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Medium 
  *Description:* Update CSS to use `@media (prefers-color-scheme)` for dynamic OS theming.

- [x] **[Bug Fix] Fix IPv6 Bracket Normalization in Proxy Edge**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Medium 
  *Description:* Robustly strip IPv6 brackets using `strings.TrimPrefix` and `strings.TrimSuffix` in `sanitizeHost` to guarantee a normalized `::1` format for the routing engine.

- [x] **[Bug Fix] Zombie Signal Handler (Unkillable Process)**  
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`  
  *Priority:* Medium  
  *Description:* Call `signal.Stop(sigCh)` immediately after trapping the first `SIGTERM`/`SIGINT` so subsequent interrupts invoke default OS termination. 


- [x] **[Bug Fix] Hardcoded ANSI Corruption in Logging** 
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Medium 
  *Description:* Import `golang.org/x/term` and use `term.IsTerminal` to strip `\033[90m` ANSI color codes if the output is redirected to a file or pipe. 


- [x] **[Bug Fix] Silent Masking of Daemon IPC Errors**  
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Low 
  *Description:* In `runRoutes`, only fall back to reading `devtether.yaml` if the error from `client.ListRoutes()` explicitly indicates the daemon isn't reachable (e.g. `ECONNREFUSED` or `os.ErrNotExist`). 


- [x] **[Bug Fix] Non-Deterministic Output (`devtether routes`)** 
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Low 
  *Description:* Sort the `configRoutes` array alphabetically by `Domain` before printing it to guarantee deterministic output on every run. 


- [x] **[Bug Fix] Inefficient DNS Record Parser**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Low 
  *Description:* Replace the slow string-parsing `dns.NewRR` logic with direct struct instantiation `&dns.A{...}` on the hot path. 


- [x] **[Bug Fix] Loss of Error Severity Context** 
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Low 
  *Description:* In `devHandler`, check if `r.Level >= slog.LevelWarn` and explicitly prepend `ERROR: ` or `WARN: ` to the standard output message. 

- [x] **[Bug Fix] Phase 3.C: Dynamic UI ANSI Bypass** 
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Medium 
  *Description:* Define ANSI color sequences as variables in `internal/cli/up.go`. Reassign them to empty strings `""` if `os.Stdout.Stat()` detects a non-character device.


- [x] **[Bug Fix] `--config` Flag Silently Ignored** 
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Low 
  *Description:* Bypass the IPC daemon check entirely if `cmd.Flags().Changed("config")` is true. 


- [x] **[Feature] Error Page UI Polish & System Theming** 
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)` 
  *Priority:* Medium 
  *Description:* Embed `.assets/devtether_logo.png` via Base64 into the 404/502 HTML overlays and update CSS to use `@media (prefers-color-scheme)` for dynamic OS theming. 


- [x] **[Bug Fix] Complete Architectural Migration to `netutil.NormalizeHost`**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* High
  *Description:* Migrate `internal/dns/server.go` and `internal/router/router.go` to use the centralized `netutil.NormalizeHost` instead of inline normalization to comply with ADR-007.

- [x] **[Bug Fix] Enforce ADR-007 Logic & Remove AI Slop in `NormalizeHost`**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Critical
  *Description:* Update `netutil.NormalizeHost` to strip brackets only when both are present. Use a clean `errors.As` check to fall back ONLY for a `*net.AddrError` containing "missing port in address".

- [x] **[Bug Fix] Add ADR-007 Table-Driven Tests for `netutil`**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Critical
  *Description:* Create `internal/netutil/netutil_test.go` with exhaustive edge-case coverage (bare IPv6, malformed, ports, trailing dots) as explicitly mandated by ADR-007.

- [x] **[Feature] Refactor Error Page Image Embedding for DRY Compliance & CSP Fix**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Medium
  *Description:* Copy `.assets/devtether_logo.png` to `internal/proxy/logo.png` and embed it natively via `//go:embed`. Inject it into the HTML directly at compile-time to avoid runtime context passing. Whitelisted `img-src` in the CSP to prevent browser blocking.

- [x] **[Bug Fix] Fix Proxy Error Page Accessibility Contrast**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Medium
  *Description:* Update `.hint` CSS in `error_page.html` to use `--hint-bg` and `--hint-text` CSS variables so it properly adapts to light mode and fixes readability issues.

- [x] **[Chore] Scrub Ephemeral "Phase" Terminology from Codebase**
  *Assignee:* `@ivin-titus (via AntiGravity: Gemini 3.1 Pro)`
  *Priority:* Low
  *Description:* Remove workspace-scoped words like "Phase 1" and "Phase 2" from `config.go`, `router.go`, `middleware.go`, and `full-config.yaml`. Enforce this in `engineering-standards.md`.

## Scope Discoveries (Deferred / Backlog) 
*(If an audit or idea creates new problems mid-sprint, log it here. Do NOT derail the current release.)* 
- [ ] **[Category] Task Title** 
  *Description:* Description of the discovered issue. 
