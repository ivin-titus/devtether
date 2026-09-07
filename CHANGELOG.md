# Changelog

All notable changes to DevTether are documented here.
This project adheres to [Semantic Versioning](https://semver.org).
Format follows [Keep a Changelog](https://keepachangelog.com).

## [v2.0.0-beta.4] — 2026-09-07

### Added
- **Security Posture:** Hardened proxy edge routing with strict input normalization, TOCTOU safety, and contextual binding isolation.
- **Graceful Error Pages:** Introduced beautiful, informative, and structurally resilient HTML error overlays for 400, 404, and 502 proxy errors.
- **Log Rate Limiting:** Proxy error logs are now cleanly rate-limited using a bounded, O(1) random-eviction hybrid LRU/TTL cache to prevent memory leaks and noise.
- **Infrastructure:** Introduced a universally compatible standard Unix Makefile for building, testing, and installing (`~/.local/bin/devtether`).
- **Engineering Standards:** Injected 10 strict, production-grade architectural rules covering edge normalization, boundary panic recovery, TOCTOU safety, streaming context, fail-fast resources, global mutation in tests, signal handler disarming, ANSI discipline, loopback isolation, and CLI error visibility.
- **Architectural Decision Records (ADR):** Created `ADR-007` (Defensive Edge Normalization) and `ADR-008` (Production Proxy Streaming). Amended `ADR-002` (DNS Design) and `ADR-003` (Security Model) to strictly enforce dual-stack loopback rules and boundary panic recovery.
- **Docs Generalization:** Replaced personal domain references with generic, universally understood architectural components (`web.localhost`, `api.localhost`, `db.localhost`) in `README.md` and `PRD.md`. Layer 1 is now marked as fully Production-Ready!

### Fixed
- **Daemon Lifecycle:** Pre-flight daemon checks now accurately detect permission errors (`EACCES`), protecting active socket lifecycles.
- **Concurrency Defenses:** Upgraded backend health checkers to strictly follow the `errgroup` concurrency standard, generating deterministic output and guaranteeing zero linear lag.
- **Proxy Resilience:** Reverse proxy outbound dials now feature strict 30-second header timeouts to eliminate zombie goroutine connections.
- **Proxy TOCTOU:** Fixed a massive Time-Of-Check to Time-Of-Use race condition in proxy request rewrites by passing strictly-resolved Context targets.
- **IPv6 Normalization:** Handled bare bracketed IPv6 literals safely in the proxy layer to prevent mapping cache misses.
- **Double Write Bugs:** Prevented header corruption during backend panics by implementing stream-aware `wroteHeader` tracking in the proxy middleware.
- **CLI UX:** CLI failures no longer endlessly spam `Usage` blocks to stdout, and tests properly route mock `os.Stdout` without creating catastrophic deadlocks.
- **DNS Trailing Dots:** The proxy now strictly trims trailing dots to mirror the DNS protocol layer format exactly.

## [v2.0.0-beta.3] — 2026-09-06

### Changed
- Refactored reverse proxy to output dynamic, HTML-based error pages instead of generic `http.Error` plain-text strings for unknown routes (404) and invalid host headers (400).
- Fixed structural CSS wrapping issues in the 502 Bad Gateway error template.
- Introduced proper `Context` injection for error templates to render fully qualified target contexts during proxy dial failures.

## [v2.0.0-beta.2] — 2026-09-04

### Added
- **New CLI Commands:** Added `version`, `--config`, and `init` commands for better user onboarding.
- **Install Script:** Added `scripts/install.sh` for one-liner curl installation with OS/architecture detection.
- **Local Dev Tools:** Added `scripts/test.sh` to seamlessly mirror CI checks locally.
- **CI/CD Pipeline:** Fully integrated GoReleaser and GitHub Actions workflows for automated cross-platform binary distribution.

### Fixed
- **DNS Server:** Resolved a critical data race during server shutdown.
- **DNS Server:** Fixed an NXDOMAIN contract violation where unmatched queries returned empty answers instead of standard errors.
- **Code Quality:** Resolved over 40+ strict linter warnings (errcheck, gosec, staticcheck, nilerr) uncovered during a deep codebase audit.
- **DRY Issues:** Consolidated duplicate error detection and version formatting logic.

### Changed
- **Documentation Overhaul:** Formally documented the **Three-Layer Master Architecture** (Networking, Process Orchestrator, Access Controls) conceptually grouping the internal 4 modular engines for a clearer mental model.
- **CI Hardening:** Upgraded to the Go 1.27.1 ecosystem and integrated `golangci-lint` as a strict gatekeeper.

## [v2.0.0-beta.1] — 2026-06-01

### Changed
- **Project renamed** from Portless to DevTether.
- **Complete architectural pivot** from monolithic orchestrator to modular 4-engine design (see [ADR-001](docs/adr/001-modular-engine-architecture.md)).
- **New `devtether.yaml` schema** — `routes:` for static routing (Engine 1). Old `services:` format is no longer supported.
- **DNS binds to `127.0.0.1:53`** by default (was `0.0.0.0:53`). Loopback-only per [ADR-003](docs/adr/003-security-model.md).
- **DNS is route-aware** — only answers for domains with active routes. Unknown `*.localhost` returns NXDOMAIN.
- **IPC socket** moved to `$XDG_RUNTIME_DIR/devtether/` with `0600` permissions (was `/tmp/` with `0777`).
- **Proxy** uses `http.Server` with configurable timeouts and graceful shutdown via `context.Context`.
- **Proxy** strips port from Host header and normalizes to lowercase before route lookup.
- **CLI** primary command is `devtether up` (alias: `start`). `list` renamed to `routes`.
- **Router** exposes `Resolver` interface for clean dependency injection into proxy and DNS.

### Removed
- Process supervisor (`internal/process/`) — Engine 2 concern, will return in Phase 2.
- Port manager (`internal/portman/`) — Engine 2 concern, will return in Phase 2.
- `devtether add` / `devtether remove` IPC commands — Engine 2 concern.
- `.local` TLD support — conflicts with mDNS (RFC 6762).

### Security
- IPC socket: `0777` → `0600`.
- Socket path: `/tmp/` → `$XDG_RUNTIME_DIR/devtether/`.
- DNS: `0.0.0.0` → `127.0.0.1` (loopback-only by default).
- Proxy: Added `ReadTimeout`, `WriteTimeout`, `IdleTimeout`.
- Proxy: Host header sanitization (strip port, lowercase, validation).

## [v1.0.0] (Legacy Portless Prototype) — 2026-05-30

### Added
- Initial prototype: monolithic daemon with coupled DNS, proxy, and orchestrator.
- This version was retired and rewritten for v2.0 due to architectural coupling issues.
