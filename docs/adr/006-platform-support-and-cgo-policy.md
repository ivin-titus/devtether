# ADR-006: Platform Support & CGo Policy

**Status:** Accepted
**Date:** 2026-09-02

## Context

DevTether is distributed as a single pre-compiled Go binary via GitHub Releases. Cross-compilation is handled by GoReleaser using Go's native `GOOS`/`GOARCH` mechanism. The release build itself runs on an Ubuntu runner; repository CI additionally tests on macOS.

This cross-compilation model breaks if any dependency uses CGo (the Go-to-C bridge), because CGo requires platform-specific C compilers and linkers for each target OS/architecture. This would force either per-platform CI runners or complex cross-compiler toolchains (e.g., zig), both of which significantly increase build complexity and maintenance burden.

Additionally, the codebase uses several Unix-specific constructs:
- Unix domain sockets for IPC (`net.Listen("unix", ...)`).
- POSIX signals (`syscall.SIGTERM`) for graceful shutdown.
- Linux kernel capabilities (`setcap cap_net_bind_service`) for binding to privileged ports.
- Future engines (e.g., Engine 2 powering Layer 2) plan to use `Setpgid`, `sh -c`, and POSIX signal escalation (`SIGTERM` → `SIGKILL`).

These constructs work on Linux and macOS but not on Windows.

## Decision

### 1. Supported Platforms

| Tier | Platforms | Commitment |
|------|-----------|------------|
| **Tier 1** | `linux/amd64`, `linux/arm64` | Fully tested. Official binaries shipped. |
| **Tier 2** | `darwin/amd64`, `darwin/arm64` | Supported and shipped; less platform-specific coverage than Linux. |
| **Tier 3** | `windows/*` | Not shipped. Not supported. Future consideration. |

Native Windows support is currently out of scope. It may be reconsidered later based on demonstrated demand; no current release commitment is implied.

### 2. CGo Policy: Pure Go Only

**No CGo dependencies are permitted in the DevTether binary.** All dependencies must be pure Go to preserve single-command cross-compilation.

If a feature requires functionality typically provided by a C library, a pure-Go alternative must be used:

| Need | CGo Option (Banned) | Pure-Go Alternative |
|------|---------------------|---------------------|
| SQLite | `mattn/go-sqlite3` | `modernc.org/sqlite` or `go.etcd.io/bbolt` |
| mDNS | System `avahi`/`dns-sd` | `hashicorp/mdns` |
| TLS/ACME | — | `golang.org/x/crypto/acme` (already pure Go) |
| CLI TTY / Colors | `fatih/color` (Heavy/OS-bound) | `golang.org/x/term` (Standard lib) |

### 3. Windows Consideration (Future)

If Windows support is ever pursued, the approach would be platform build tags (`_linux.go`, `_darwin.go`, `_windows.go`) to provide OS-specific implementations:
- IPC: Unix sockets → Named pipes.
- Signals: `SIGTERM` → `GenerateConsoleCtrlEvent`.
- Process management: `Setpgid` + `sh -c` → Job Objects + `cmd.exe /C`.

Engines that cannot be made cross-platform (e.g., Engine 2's process supervisor for Layer 2) would be excluded on Windows via build tags, with a clear runtime error: "Layer 2 (Orchestration) is not supported on Windows."

## Consequences

### Positive

- Cross-compilation works out of the box with `go build` and GoReleaser.
- The release build runs on `ubuntu-latest`; repository CI also runs tests on macOS.
- Binary size stays small (no C runtime linking).
- The dependency policy is simple to enforce: `CGO_ENABLED=0` in all builds.

### Negative

- Windows users cannot use DevTether until Tier 3 support is implemented.
- Pure-Go alternatives for some libraries (e.g., `modernc.org/sqlite`) may have lower performance than their CGo counterparts. For DevTether's use case (local dev tool, not a database), this tradeoff is acceptable.
- macOS is Tier 2 (community-tested), meaning bugs on macOS may take longer to surface and fix.
