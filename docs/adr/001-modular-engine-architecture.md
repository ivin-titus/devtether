# ADR-001: Modular Engine Architecture

**Status:** Accepted  
**Date:** 2026-05-30  
**Reconciled:** 2026-09-28

## Context

The original DevTether prototype coupled process orchestration, DNS, and reverse proxying into one daemon workflow. That made simple local routing depend on concerns that did not belong to it.

The rewrite separates those concerns so the local networking core can remain small while future capabilities are added independently.

## Decision

The target architecture uses **4 independent engines**:

1. **Engine 1 — Static Routing:** embedded DNS and HTTP reverse proxy for pre-existing local services.
2. **Engine 2 — Orchestration:** process supervision and dynamic `$PORT` allocation.
3. **Engine 3 — Tunneling:** LAN sharing and WAN tunneling through a self-hosted relay.
4. **Engine 4 — Access Control:** scoped tokens and RBAC for shared services.

### Current beta boundary

Only Engine 1 is implemented in beta. The config package contains the future engine types for schema planning, but the current validator rejects `orchestrate:`, `tunnel:`, and `access:` until those engines are implemented.

The rule that “engine activation is driven by the presence of its config section” is therefore a **target architecture rule**, not a current beta capability.

## Three conceptual layers

For user-facing architecture discussions, the four engines may be grouped as:

| Layer | Engine mapping | Current status |
|---|---|---|
| Layer 1 — Local Networking | Engine 1 + shared infrastructure | Current |
| Layer 2 — Orchestration | Engine 2 | Future |
| Layer 3 — Sharing & Access | Engine 3 + Engine 4 | Future |

This keeps tunneling out of the current Layer 1 scope.

## Future GUI strategy

A GUI, if implemented later, should remain a lightweight client of the daemon rather than becoming a second runtime architecture.

The current target is:

- embedded static assets;
- no heavyweight desktop runtime;
- shared management semantics with the CLI;
- strict browser-origin validation.

The GUI is **not implemented in beta.7**.

## Consequences

- Users can adopt local routing without learning process orchestration.
- Future engines have explicit boundaries before implementation begins.
- The current codebase avoids speculative engine coupling.
- Documentation can distinguish the shipped Engine 1 implementation from the target platform architecture.
