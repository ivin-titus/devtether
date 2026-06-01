# Changelog

All notable changes to DevTether are documented here.
This project adheres to [Semantic Versioning](https://semver.org).
Format follows [Keep a Changelog](https://keepachangelog.com).

## [0.2.0] — 2026-06-01

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

## [0.1.0] — 2026-05-30

### Added
- Initial v0 prototype: monolithic daemon with coupled DNS, proxy, and orchestrator.
- This version was retired due to architectural coupling issues.
