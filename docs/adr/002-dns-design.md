# ADR-002: DNS Design — Non-Recursive, Loopback-Only

**Status:** Accepted  
**Date:** 2026-05-30  
**Reconciled:** 2026-09-28

## Context

DevTether embeds a DNS server so local HTTP routes can use named `.localhost` domains without editing `/etc/hosts`.

A DNS listener must not become an open resolver or accidentally expose local development services to other devices.

## Decision

### 1. Non-recursive

DevTether never forwards unmatched DNS queries to an upstream resolver. It only answers for managed `.localhost` routes.

### 2. Safe default bind

The current default is:

```text
127.0.0.1:5335
```

This keeps the current Engine 1 DNS listener local and unprivileged.

Port 53 is supported only as an explicit configuration when the host provides the required privileges. Earlier 53-default and 5353-fallback behavior is historical, not current.

### 3. Route-aware response contract

The response depends on route existence and query type:

| Route exists? | Query type | Response |
|---|---|---|
| No | Any | NXDOMAIN |
| Yes | A | A = 127.0.0.1 |
| Yes | Unsupported type, such as AAAA/HTTPS | NOERROR with zero answers (NODATA) |

The current route must therefore be distinguished from an unsupported record type.

### 4. Host integration

`devtether init` may configure supported Linux/macOS resolver integrations with explicit user consent. It writes the exact configured DNS port into the generated resolver configuration.

If `dns.bind` is changed manually, the host resolver must be updated to match.

### 5. TLD scope

The current engine hardcodes `.localhost`.

`.internal` and arbitrary/custom TLD configuration are **not supported in the current beta**. A future Engine 3 implementation may define a separate sharing namespace, but that is a future architectural decision.

## Consequences

- The current resolver stays local and non-recursive.
- Unmanaged DNS names are rejected rather than forwarded.
- Dual-stack clients can query unsupported types without the hostname being treated as nonexistent.
- OS resolver integration remains explicit and auditable.

## Historical note

Earlier revisions used port 53 as the default and experimented with fallback behavior. Those contracts are superseded by the current 5335 fail-fast design.
