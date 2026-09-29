# ADR-004: Relay as Separate Binary

**Status:** Accepted — Future Engine 3 Design  
**Date:** 2026-05-30  
**Reconciled:** 2026-09-28

## Context

Future WAN tunneling will need a self-hosted relay that accepts public traffic and forwards it through an outbound tunnel to a developer machine.

## Decision

When Engine 3 is implemented, the relay will be a separate `devtether-relay` binary in the same repository.

Planned structure:

```text
cmd/
├── devtether/
└── devtether-relay/
```

The current repository contains only the `devtether` binary.

## Consequences

- The developer binary stays focused on the local networking engine.
- Relay deployment gets a separate operational/security boundary.
- Tunnel protocol compatibility can be versioned independently.
- GoReleaser can build both binaries once the relay source exists.

The current `.goreleaser.yaml` defines only the `devtether` build. Multi-binary release automation is future work tied to Engine 3.
