# ADR-006: Platform Support & CGo Policy

**Status:** Accepted
**Date:** 2026-09-02

## Context

DevTether is distributed as a single pre-compiled Go binary via GitHub Releases. Cross-compilation is handled by GoReleaser, which uses Go's native `GOOS`/`GOARCH` mechanism to produce binaries for multiple platforms from a single CI runner.

This cross-compilation model breaks if any dependency uses CGo (the Go-to-C bridge), because CGo requires platform-specific C compilers and linkers for each target OS/architecture. This would force either per-platform CI runners or complex cross-compiler toolchains (e.g., zig), both of which significantly increase build complexity and maintenance burden.

Additionally, the codebase uses several Unix-specific constructs:
- Unix domain sockets for IPC (`net.Listen("unix", ...)`).
- POSIX signals (`syscall.SIGTERM`) for graceful shutdown.
- Linux kernel capabilities (`setcap cap_net_bind_service`) for binding to privileged ports.
- Future engines plan to use `Setpgid`, `sh -c`, and POSIX signal escalation (SIGTERM → SIGKILL).

These constructs work on Linux and macOS but not on Windows.

## Decision

### 1. Supported Platforms

| Tier | Platforms | Commitment |
|------|-----------|------------|
| **Tier 1** | `linux/amd64`, `linux/arm64` | Fully tested. Official binaries shipped. |
| **Tier 2** | `darwin/amd64`, `darwin/arm64` | Compiled and shipped. Community-tested. |
| **Tier 3** | `windows/*` | Not shipped. Not supported. Future consideration. |

Windows support may be reconsidered when there is demonstrated demand. The path forward is documented below under "Windows Roadmap."

### 2. CGo Policy: Pure Go Only

**No CGo dependencies are permitted in the DevTether binary.** All dependencies must be pure Go to preserve single-command cross-compilation.

If a feature requires functionality typically provided by a C library, a pure-Go alternative must be used:

| Need | CGo Option (Banned) | Pure-Go Alternative |
|------|---------------------|---------------------|
| SQLite | `mattn/go-sqlite3` | `modernc.org/sqlite` or `go.etcd.io/bbolt` |
| mDNS | System `avahi`/`dns-sd` | `hashicorp/mdns` |
| TLS/ACME | — | `golang.org/x/crypto/acme` (already pure Go) |

### 3. Windows Roadmap (Future)

If Windows support is pursued, the approach is platform build tags (`_linux.go`, `_darwin.go`, `_windows.go`) to provide OS-specific implementations:
- IPC: Unix sockets → Named pipes.
- Signals: `SIGTERM` → `GenerateConsoleCtrlEvent`.
- Process management: `Setpgid` + `sh -c` → Job Objects + `cmd.exe /C`.

Engines that cannot be made cross-platform (e.g., Engine 2's process supervisor) would be excluded on Windows via build tags, with a clear runtime error: "Engine 2 (Orchestration) is not supported on Windows."

## Consequences

### Positive

- Cross-compilation works out of the box with `go build` and GoReleaser.
- CI/CD is a single `ubuntu-latest` runner — no platform matrix needed.
- Binary size stays small (no C runtime linking).
- The dependency policy is simple to enforce: `CGO_ENABLED=0` in all builds.

### Negative

- Windows users cannot use DevTether until Tier 3 support is implemented.
- Pure-Go alternatives for some libraries (e.g., `modernc.org/sqlite`) may have lower performance than their CGo counterparts. For DevTether's use case (local dev tool, not a database), this tradeoff is acceptable.
- macOS is Tier 2 (community-tested), meaning bugs on macOS may take longer to surface and fix.
