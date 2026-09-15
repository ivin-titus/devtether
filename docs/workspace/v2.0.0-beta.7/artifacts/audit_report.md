# Audit Report: v2.0.0-beta.7

**Date:** 09/15/2026
**Auditor:** `@ivintitus (via Antigravity: Claude Opus 4.6)`

## 1. Executive Summary
Pre-mortem audit (before implementation) discovered one stale configuration issue. Post-mortem audit (after Phase 1) verified all changes against ADRs, engineering standards, and architecture. No regressions. Two observations logged for tracking.

## 2. Methodology
- Manual codebase review of every changed file (`internal/config/config.go`, `internal/config/config_test.go`, `internal/cli/up.go`).
- Cross-referenced against `docs/engineering-standards.md`, `docs/architecture.md`, ADR-003 (security model), ADR-006 (no CGo), ADR-008 (proxy streaming).
- Full CI suite (`make test`): module integrity, linting, vet, govulncheck, unit tests with `-race`, cross-compile (darwin/arm64), Linux build — all 8 checks passed.

## 3. Detailed Findings

### Finding 1: Stale `ReadTimeout` / `WriteTimeout` Config Fields
- **Severity:** Low
- **Location:** `internal/config/config.go:93-96` (`DefaultReadTimeout`, `DefaultWriteTimeout`); `config.go:151-156` (`applyDefaults`); `config.go:215-220` (`validateProxy`)
- **Status:** Resolved (09/15/2026, Subphase 1.4 Step 7)
- **Description:** `v2.0.0-beta.5` removed absolute `ReadTimeout` and `WriteTimeout` from the proxy `http.Server` per ADR-008. The proxy (`internal/proxy/server.go:30-38`) only reads `cfg.Timeouts.Idle`. The config still defines, defaults, and validates `Read`/`Write` timeout fields that the proxy silently ignores.
- **Impact:** Users who set custom `proxy.timeouts.read` or `proxy.timeouts.write` receive no warning that these values are unused. Misleading configuration surface.
- **Remediation Plan:** Remove stale constants, defaults, and validation. Retain only `Idle`. Bundle with a future config cleanup pass.

### Finding 2: Duplicate `CheckRunning` in Detach + Foreground Paths (Observation — Not a Bug)
- **Severity:** Info
- **Location:** `internal/cli/up.go:68` (parent pre-flight) and `internal/cli/up.go:100` (child pre-flight)
- **Status:** Resolved (by design)
- **Description:** When `-d` is used, the parent calls `daemon.CheckRunning` at L68 for immediate user feedback. The forked child then calls it again at L100 before starting its own servers. This double-check is intentionally defensive — it catches a race where a *different* DevTether instance starts between the parent's check and the child's startup.
- **Impact:** None. Correct behavior.

### Finding 3: No Explicit `devtether down` / Stop Mechanism
- **Severity:** Low
- **Location:** `internal/cli/` (no `down.go` exists)
- **Status:** Open — Deferred to Phase 2
- **Description:** After `devtether up -d`, there is no CLI command to gracefully stop the daemon. Users must manually `kill <PID>` (printed at startup). The daemon shuts down cleanly on `SIGTERM` via the existing signal handler. A proper `devtether down` command should dial the IPC socket and request shutdown, or send `SIGTERM` to the PID.
- **Remediation Plan:** Implement alongside `devtether status` in Phase 2, which already exposes PID via the `/status` endpoint.

## 4. Phase 1 Post-Mortem: Compliance Matrix

| Standard | Status | Notes |
|---|---|---|
| ADR-003: Secure by Default | ✓ Pass | Log dir `0700`, log file `0644`, `daemon` defaults to `false`. |
| ADR-006: No CGo | ✓ Pass | Pure Go daemonization via `exec.CommandContext` + `syscall.SysProcAttr`. |
| ADR-008: Proxy Streaming | ✓ N/A | Phase 1 did not touch proxy. |
| Engineering: No AI Slop | ✓ Pass | 3 fields added to config, ~50 lines of daemon logic. No premature abstractions. |
| Engineering: SoC | ✓ Pass | Config owns schema. CLI owns process lifecycle. No leaked responsibilities. |
| Engineering: DRY | ✓ Pass | No duplicate logic. `configPath` and `verbose` reused from `root.go`. |
| Engineering: Error Handling | ✓ Pass | All errors wrapped with `fmt.Errorf("context: %w", err)`. No log-and-return. |
| Engineering: Naming | ✓ Pass | `SettingsConfig` godoc present. `daemonize` unexported, file-scoped to `up.go`. |
| Engineering: Testing | ✓ Pass | 2 new table-driven test cases. All 18 config tests pass with `-race`. |
| Engineering: Security | ✓ Pass | `nolint:gosec` annotations with explicit justifications. No hardcoded secrets. |
| Linting | ✓ Pass | 0 issues. `golangci-lint`, `go vet`, `govulncheck`, `gosec` all clean. |
| Cross-compile | ✓ Pass | `darwin/arm64` builds successfully. `Setsid` field exists on both Linux and macOS. |

---

## 5. Documentation Quality Audit

### Finding 4: Root Help Text Missing `-d` and `settings:` Documentation
- **Severity:** Medium
- **Location:** `internal/cli/root.go:19-28` (root `Long` description)
- **Status:** Resolved (09/15/2026, Subphase 1.4 Step 1)
- **Description:** The root help (`devtether --help`) displays a hardcoded command cheat-sheet:
  ```
  devtether up        Start the routing daemon
  devtether routes    Show active routes
  devtether init      Create a starter devtether.yaml
  devtether version   Print version information
  ```
  This does not mention the new `-d` flag or daemon mode. Users running `devtether --help` have no idea they can background the daemon. The `-d` flag *is* properly listed in `devtether up --help`, but discoverability from the root level is poor.
- **Remediation Plan:** Add `devtether up -d` to the root help cheat-sheet.

### Finding 5: `up` Command Long Description Doesn't Mention Daemon Mode
- **Severity:** Medium
- **Location:** `internal/cli/up.go:37-38` (upCmd `Long` description)
- **Status:** Resolved (09/15/2026, Subphase 1.4 Step 2)
- **Description:** The `up` command's Long description reads: *"Loads devtether.yaml, registers static routes, and starts the DNS resolver, reverse proxy, and IPC daemon. Press Ctrl+C for graceful shutdown."* It doesn't mention `-d` mode, `settings.daemon`, `DEVTETHER_FORKED`, or where logs go when detached. A user unfamiliar with the flags would miss the background capability entirely.
- **Remediation Plan:** Extend the Long description to briefly mention background mode (e.g., "Use -d to run in the background. Logs are written to .logs/devtether.log.").

### Finding 6: README.md Commands Section Missing `-d` Flag
- **Severity:** Low
- **Location:** `README.md:134-141` (Commands section)
- **Status:** Resolved (09/15/2026, Subphase 1.4 Step 3)
- **Description:** The README's Commands section shows:
  ```
  devtether up                        # Start the routing daemon
  devtether up -c /path/to/config     # Use a specific config file
  ```
  It does not mention `devtether up -d` for background mode. Users reading the README won't discover daemon mode.
- **Remediation Plan:** Add `devtether up -d` to the commands table.

### Finding 7: README.md Still References `beta.2` in Download Example
- **Severity:** Low
- **Location:** `README.md:50`
- **Status:** Resolved (09/15/2026, Subphase 1.4 Step 4)
- **Description:** The binary download example says `devtether_2.0.0-beta.2_linux_amd64.tar.gz`. This is stale — the latest release is `beta.6`. The version in the example should either track the latest release or use a generic placeholder.
- **Remediation Plan:** Replace with a generic example like `devtether_<VERSION>_linux_amd64.tar.gz` or update to the current release.

### Finding 8: `init` Command Still Dumps Static Template — Help Text Is Accurate But Pre-Dates Wizard Plans
- **Severity:** Info
- **Location:** `internal/cli/init.go:17-18`
- **Status:** Partially Resolved (09/15/2026, Subphase 1.4 Step 5) — `settings:` block added to template. Help text update deferred to Phase 3.1 wizard.
- **Description:** The `init` command's Short and Long descriptions accurately describe *current* behavior ("Create a starter devtether.yaml"). Once the interactive wizard is implemented (Phase 3.1), these descriptions will need updating. The generated config template also doesn't mention `settings:` as a configurable block. No action needed now, but the help text must be updated alongside the wizard implementation.

### Finding 9: No Godoc on `runUp`, `daemonize`, `printStartupSummary`, `isBackendOnline`
- **Severity:** Low
- **Location:** `internal/cli/up.go`
- **Status:** Open
- **Description:** Per engineering standards, "Godoc on all exported symbols" is mandatory. These functions are unexported (lowercase), so they're technically exempt. However, `daemonize` performs a non-trivial process lifecycle operation and has a godoc comment (good). `runUp`, `printStartupSummary`, and `isBackendOnline` do not have godoc-style comments — they have inline step-comments instead. This is acceptable for unexported CLI functions but worth noting for consistency.
- **Remediation Plan:** No immediate action. Monitor as more CLI commands are added.

### Finding 10: `root_out.go` Is an Empty File
- **Severity:** Low
- **Location:** `internal/cli/root_out.go` (2 lines, `package cli` only)
- **Status:** Resolved (09/15/2026, Subphase 1.4 Step 6) — Confirmed unused via `git log` and `grep`. Deleted with `git rm`.
- **Description:** This file contains only the package declaration. It appears to be a leftover placeholder or stub. Empty files with no purpose add noise to the codebase.
- **Remediation Plan:** Investigate origin. If unused, delete it.
