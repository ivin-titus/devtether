# ADR-004: Relay as Separate Binary

**Status:** Accepted
**Date:** 2026-05-30

## Context

DevTether's WAN tunneling feature (Engine 3, part of Layer 3) requires a relay server deployed on a VPS to accept incoming HTTPS traffic and forward it through WebSocket tunnels to developers' local machines.

The question: should the relay be embedded in the main `devtether` binary, or built as a separate `devtether-relay` binary?

## Decision

Build `devtether-relay` as a **separate binary** from a **separate `cmd/` entry point** in the same repository.

```
cmd/
├── devtether/           # Main CLI binary (developer machine)
│   └── main.go
└── devtether-relay/     # Relay server binary (VPS)
    └── main.go
```

Both binaries share common packages from `internal/` (e.g., tunnel protocol definitions, token validation logic) but have distinct compilation targets.

## Consequences

### Positive

- **Smaller binary for developers** — The main `devtether` binary doesn't include relay-only dependencies (ACME/Let's Encrypt, relay routing logic). Developers who never use WAN tunneling don't carry this weight.
- **Independent deployment** — The relay can be versioned, released, and deployed independently on VPS infrastructure.
- **Security boundary** — The relay binary doesn't include orchestrator or IPC socket code. It's a pure network relay with token validation. Minimal attack surface.
- **Clear responsibility** — Contributors know exactly which binary they're working on.

### Negative

- **Shared code management** — Changes to the tunnel protocol must be compatible across both binaries. Breaking changes require coordinated releases.
- **Two build targets** — CI/CD must build and release both binaries. Goreleaser supports this natively via multiple builds.
- **Potential for version drift** — If a developer runs an old `devtether` against a new `devtether-relay`, the protocol may be incompatible.

### Mitigation for Tech Debt

- **Protocol versioning** — The tunnel WebSocket handshake includes a protocol version number. Incompatible versions are rejected with a clear error message.
- **Shared `internal/tunnel/protocol.go`** — The framing protocol, message types, and version constants live in a shared internal package. Both binaries import it. Changes to this file trigger tests for both binaries.
- **Goreleaser multi-build** — A single `goreleaser.yaml` builds both `devtether` and `devtether-relay` for all target platforms in one release.
