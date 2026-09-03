# Changelog

All notable changes to DevTether are documented here.
This project adheres to [Semantic Versioning](https://semver.org).
Format follows [Keep a Changelog](https://keepachangelog.com).

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
