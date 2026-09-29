# ADR-003: Security Model — Secure by Default, Permissive by Opt-in

**Status:** Accepted  
**Date:** 2026-05-30  
**Reconciled:** 2026-09-28

## Context

DevTether owns local DNS, proxy, and IPC boundaries. The current beta is deliberately local-only; future engines may add process execution and network exposure.

## Decision

Features that expand the attack surface require explicit opt-in.

### Current beta security boundary

| Surface | Current posture |
|---|---|
| IPC | Per-user runtime directory, owner-only socket, instance `flock`, startup nonce |
| DNS | `127.0.0.1:5335` by default, route-aware, non-recursive |
| Reverse proxy | Loopback-only listeners and backend targets; strict Host validation |
| HTTP timing | Explicit header/idle controls, with the streaming exception defined by ADR-008 |
| Configuration | Strict YAML field decoding and route validation |

The current beta does **not** expose LAN/WAN sharing, process execution, RBAC, or a browser management GUI.

### Runtime ownership

The daemon validates runtime directories before filesystem mutation, holds an exclusive `flock` for its lifetime, and treats the PID file as secondary diagnostic/fallback state.

When running with elevated privileges, the daemon uses a root-owned runtime location rather than placing privileged sockets in an unprivileged user's runtime directory.

### Network binding

Current listeners bind explicitly to loopback addresses. Blank host bindings such as `":port"` are forbidden.

Future Engine 3 LAN sharing may deliberately broaden network exposure, but only as an explicit opt-in with separate threat modeling.

## Future security surfaces

### Engine 2 — Orchestration

Future process execution must constrain command execution, process groups, environment inheritance, and privilege boundaries.

### Engine 3 — Tunneling

Future LAN/WAN exposure must add explicit sharing controls, tunnel authentication, and relay security.

### Engine 4 — Access Control

Future shared services must enforce scoped access policies and tokens before exposure.

### Future Web GUI

A future browser management interface must validate Origin/Host and related browser request metadata before exposing management data.

## Non-Negotiable Invariant

> A network-reachable endpoint must never gain arbitrary shell-command execution without explicit user consent.

## Consequences

The current beta keeps its network surface narrow and local while leaving future exposure mechanisms subject to explicit security design before implementation.
