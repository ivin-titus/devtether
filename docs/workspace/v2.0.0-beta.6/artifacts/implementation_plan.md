# Release v2.0.0-beta.6 - Implementation Plan

## Pre-Flight Checklist
- [ ] Codebase audited for related logic and edge cases.
- [ ] Changes verified against `docs/engineering-standards.md`.
- [ ] Architectural decisions comply with `docs/adr/`.

## Phase 1: Reverse Proxy & UI Overlays
### Subphase 1.A: Error Page Theming & Branding
- **Step 1:** Convert `.assets/devtether_logo.png` to a base64 string. Embed it as a proper `<link rel="icon">` favicon in the `<head>`, and place it as a tiny, well-integrated logo inside the error-showing container in `internal/proxy/error_page.html` without disrupting the UI/UX.
- **Step 2:** Rewrite the hardcoded dark-mode CSS variables in `internal/proxy/error_page.html` to use `@media (prefers-color-scheme: dark)` and `light` media queries, allowing the overlay to gracefully adapt to the user's system theme.
- **Step 3:** Trigger a 404/502 in the browser and verify the logo is visible and the page theme matches the OS.

### Subphase 1.B: Edge Normalization (ADR-007 Compliant)
- **Step 1:** Create or update a centralized function `NormalizeHost` in `internal/netutil`.
- **Step 2:** Implement the ADR-007 strictly mandated sequence: *First*, strip IPv6 `[` and `]` literals. *Then*, use `net.SplitHostPort` (or equivalent robust splitting) to strip the port.
- **Step 3:** Update `internal/proxy/handler.go` to completely remove its local `sanitizeHost` and use `netutil.NormalizeHost` instead.

### Post Phase 1 Resolution
- **Bug Fix 1 (ADR-007 Logic):** Update `netutil.NormalizeHost` to strip `[` and `]` **only** when both are present. When using `net.SplitHostPort`, fall back to the original string **only** if the error is explicitly a `*net.AddrError` (missing port).
- **Bug Fix 2 (Architectural Migration):** Ensure `internal/dns/server.go` and `internal/router/router.go` are completely migrated to use `netutil.NormalizeHost` for trailing dot and lowercasing logic, centralizing all edge normalization.
- **Bug Fix 3 (Test Coverage):** Create `internal/netutil/netutil_test.go` with a table-driven test covering all edge cases (empty strings, bare IPv6, IPv6 with ports, trailing dots, mixed casing) as strictly mandated by ADR-007.
- **Bug Fix 4 (Overengineering & Logic Flaw):** Refactor `net.SplitHostPort` error handling in `NormalizeHost` to strictly use a clean `errors.As` check. It must ONLY fall back to the original string if the error is exactly `*net.AddrError` with `.Err == "missing port in address"`. Remove all verbose boilerplate/AI slop.

### Subphase 1.C: UI/UX Polish & Template Refactoring
- **Step 1 (DRY & Clean HTML):** Remove the hardcoded, truncated base64 strings from `internal/proxy/error_page.html`. Replace them with a native parse-time placeholder `{{LOGO_BASE64}}`.
- **Step 2 (Go Embed):** Copy `.assets/devtether_logo.png` to `internal/proxy/logo.png`. In `internal/proxy/templates.go`, use `//go:embed logo.png` to read the image as `[]byte`. Encode it to a Base64 string at initialization time.
- **Step 3 (Zero-Pass Injection):** Optimize template performance by removing `LogoBase64` from `ErrorPageData` entirely. Instead, use `strings.ReplaceAll` in `templates.go` to inject the Base64 string directly into the raw HTML string *before* the template is compiled, burning the image directly into memory at boot time.
- **Step 4 (Accessibility):** Fix the `.hint` CSS in `error_page.html`. Replace the hardcoded `#bae6fd` text and `rgba(56,189,248,0.1)` background with CSS variables (e.g., `--hint-bg` and `--hint-text`) that adapt safely in both light and dark modes to resolve the contrast issues.
- **Step 5 (CSP Whitelist):** Add `img-src 'self' data:;` to the `<meta http-equiv="Content-Security-Policy">` tag to prevent the browser from strictly blocking the embedded `data:` image URI.
- **Step 6 (Terminology Cleanup):** Scrub all code comments, godocs, and configuration examples (`full-config.yaml`, `config.go`, `router.go`, `middleware.go`) of any ephemeral release terminology like "Phase X". Enforce this in `engineering-standards.md`.

## Phase 1.5: AI Workforce Standardization
### Subphase 1.5.A: Bulletproof Agent Execution Context
- **Step 1:** Create `docs/workspace/agent-skills/ponytail/SKILL.md` to define the "Lazy Senior Dev" (YAGNI, Native Tools, Standard Library) mindset.
- **Step 2:** Link the `ponytail` skill globally across `AGENTS.md`, `devtether-change`, and `devtether-audit` to enforce minimal step sizes and ban AI slop.
- **Step 3:** Introduce the **Anti-Guessing Rule** to `AGENTS.md` and mandate strict compliance with `docs/workspace/README.md` artifact templates across all subagents.

## Phase 2: CLI UX & Daemon Fallback Logic
### Subphase 2.A: Initialization Configurations
- **Step 1:** Change the commented default generated configuration in `internal/cli/init.go` from `api.localhost: 8080` to `api.localhost: 3000`. This prevents an infinite proxy loop when a user runs the daemon unprivileged and it falls back to port `8080`.
- **Step 2:** Run `devtether init` and ensure the default port is `3000`.

### Subphase 2.B: IPC Client Output & Error Masking
- **Step 1:** Inspect the `err` from `client.ListRoutes()` in `internal/cli/routes.go` (`runRoutes`). Only fall back to reading `devtether.yaml` if the error explicitly indicates the daemon isn't reachable (e.g. `ECONNREFUSED` or `os.ErrNotExist`). Otherwise, print the real failure to the user.
- **Step 2:** Check if `cmd.Flags().Changed("config")` is true in `internal/cli/routes.go` (`runRoutes`). If it is, completely bypass the IPC daemon check and read the requested config file directly.
- **Step 3:** Sort the generated `configRoutes` array alphabetically by `Domain` in `internal/cli/routes.go` (`runRoutes`) before passing it to `printRoutes()` to guarantee deterministic terminal output on every invocation.

### Subphase 2.C: Graceful Daemon Shutdown
- **Step 1:** Call `signal.Stop(sigCh)` immediately after trapping the first `SIGTERM`/`SIGINT` in `internal/cli/up.go` (Signal Goroutine). This ensures subsequent `Ctrl+C` inputs invoke default OS termination instead of hanging if the shutdown sequence stalls.
- **Step 2:** Run `devtether up` and mash `Ctrl+C` during startup/shutdown to verify the zombie process is cleanly killable.

## Phase 3: Core Network Engines (DNS & Logging)
### Subphase 3.A: DNS Performance Optimization
- **Step 1:** Replace the slow string-parsing `dns.NewRR` logic on the hot path in `internal/dns/server.go` (`handleRequest`) with direct, zero-allocation struct instantiation: `&dns.A{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET}, A: net.ParseIP("127.0.0.1")}`.

### Subphase 3.B: Standard Output Logging & ANSI Discipline
- **Step 1:** Import `golang.org/x/term` in `internal/logger/logger.go` to cleanly detect if the logger is running in a TTY via `term.IsTerminal(int(os.Stdout.Fd()))`. Apply `\033[90m` around structured attributes only if the check passes.
- **Step 2:** Explicitly read `r.Level` inside `devHandler.Handle`. Prepend `ERROR:` or `WARN:` to the raw message if it meets or exceeds `slog.LevelWarn` to guarantee failure visibility when running in standard (non-verbose) mode.

### Subphase 3.C: Dynamic UI ANSI Bypass (Standards Compliant)
- **Step 1:** In `internal/cli/up.go`, define local variables for the ANSI codes (`dim`, `green`, `reset`, `yellow`) at the top of the startup rendering blocks.
- **Step 2:** Import `golang.org/x/term` and execute `if !term.IsTerminal(int(os.Stdout.Fd()))`. If the check succeeds (it is NOT a terminal), reassign all color variables to `""`.
- **Step 3:** Use the variables in the `fmt.Printf` format strings instead of hardcoded ANSI escapes.
- **Step 4:** Pipe `devtether up` into a file and verify no ANSI escape codes are present in the startup banner.
