# ADR-009: Proxy Security Lessons & Defense-in-Depth

**Status:** Accepted  
**Date:** 2026-09-08  
**Reconciled:** 2026-09-28

## Context

This ADR records defensive rules derived from proxy and daemon audits. It distinguishes protections implemented in beta.7 from controls reserved for future engines.

## Current beta posture

### Cross-request state

The current proxy uses Go's `net/http`, request-scoped context, and no `unsafe` package. It does not currently use `sync.Pool` for request payload buffering.

The current proxy error-log throttle is bounded. It stores cooldown timestamps in a map and randomly evicts one key when the capacity limit is reached. It is **not** an LRU/TTL cache.

### Request and protocol handling

The current proxy:

- validates Host against configured routes;
- only targets loopback backends;
- centralizes host normalization;
- uses `httputil.ReverseProxy`;
- sets a response-header timeout on its transport;
- configures `ReadHeaderTimeout` and `IdleTimeout`;
- does not enable TLS/HTTP/2 in the current beta.

These controls reduce common exposure; they are not a blanket guarantee of immunity from every protocol vulnerability.

### IPC and local privilege boundaries

The daemon:

- uses owner-only Unix sockets;
- validates its runtime directory;
- holds an instance `flock`;
- authenticates shutdown requests with a startup nonce;
- isolates root-owned runtime state from normal-user runtime state.

The current beta has no network-reachable shell-command endpoint.

### Terminal output

Current structured logging is used for proxy/daemon diagnostics. Untrusted HTTP metadata is not intentionally emitted as raw terminal control sequences.

## Future defenses

### Web GUI / traffic streaming

A future management GUI or traffic stream must validate browser Origin/Host and related request metadata before exposing local management data.

### Engine 2 orchestration

Future process execution must constrain commands, environment inheritance, process groups, and privilege boundaries.

### Engine 3 tunneling

Future LAN/WAN sharing must add explicit network exposure controls, tunnel authentication, and relay security.

### Engine 4 access control

Future shared services must enforce scoped access policies and tokens before exposure.

### OS service management

System-level `systemd`/launchd` installation is currently deferred. A future service installer must enforce explicit user/group privilege boundaries, safe binary ownership, working-directory handling, and secure runtime directories.

## Decision

1. Never allow a catch-all proxy route.
2. Keep current network listeners explicitly bound to loopback.
3. Do not retain request-scoped state in long-lived shared objects without explicit ownership/lifetime rules.
4. Bound caches that index untrusted input.
5. Keep future browser, tunnel, orchestration, access-control, and service-management defenses clearly separate from the current beta contract.
6. Do not describe a protection as implemented until its corresponding source exists in the current branch.

## Consequences

The security documentation stays narrow enough to verify against the current source while preserving requirements for future network-exposure features.
