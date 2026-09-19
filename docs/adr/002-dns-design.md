# ADR-002: DNS Design — Non-Recursive, Loopback-Only

**Status:** Accepted
**Date:** 2026-05-30

## Context

DevTether embeds a DNS server (`miekg/dns`) to resolve the `.localhost` TLD (and eventually `.internal` for Engine 3) to the developer's machine. This eliminates the need for manual `/etc/hosts` editing.

However, running a DNS server introduces significant security risks:

1. **Open resolver abuse** — If DevTether forwards unmatched queries upstream, it becomes an open DNS resolver that can be exploited for DNS amplification attacks.
2. **Network exposure** — Binding to `0.0.0.0:53` (the original default) exposes the DNS server to every device on the network, even when the developer only intends to use DevTether locally.
3. **Per-query overhead** — The original implementation called `net.Dial("udp", "8.8.8.8:80")` on every DNS query to determine the local IP, adding unnecessary syscall overhead.

## Decision

1. **Non-recursive by design** — DevTether will **never** forward DNS queries to upstream resolvers. It only answers queries for the `.localhost` TLD. All other queries receive `NXDOMAIN`. This is a permanent, non-negotiable design decision.
2. **Loopback-only by default** — DNS binds to `127.0.0.1:5335` by default to remain completely unprivileged and avoid mDNS port conflicts (`5353`). It only binds to `0.0.0.0:53` when `--lan` mode is explicitly enabled with sudo.
3. **Fail-Fast Predictability** — We eliminated the legacy behavior of silently falling back to unprivileged ports if port 53 was occupied. The daemon now strictly binds to the port specified in `devtether.yaml` (defaulting to 5335) and crashes cleanly if it cannot.
4. **Cached LAN IP** — The local IP is resolved once on startup and cached. It is refreshed only on detected network changes (interface up/down events), not per-query.

## Consequences

### Positive

- Eliminates the entire class of DNS amplification and open resolver vulnerabilities.
- Default configuration is safe — a developer who just runs `devtether up` cannot accidentally expose their DNS to the network.
- Reduced per-query latency by eliminating the UDP dial overhead.

### Negative

- Developers must configure their OS resolver (e.g., `systemd-resolved`) to forward specific TLDs to `127.0.0.1:5335`. This is a one-time setup step documented in the README and automated via `devtether init`.
- In LAN mode, the DNS server is exposed to the local network, which is a deliberate and accepted tradeoff for the LAN sharing feature.

### YAGNI & Scope Limitation (Custom TLDs)
Early architectural drafts included support for `.internal` or completely customizable TLD arrays. We have explicitly dropped this in favor of hardcoding `.localhost`.
- **Why:** Supporting arbitrary TLDs significantly increases the complexity of OS integration (e.g., conflicting with mDNS `.local` or breaking corporate `.internal` resolutions) and creates a massive testing matrix across macOS and Linux resolvers.
- **YAGNI Rationale:** We adhere strictly to the "Lazy Senior Dev" mindset. Hardcoding `.localhost` solves 99% of local development routing needs perfectly. Building complex customizable TLD injection is overengineering a problem our users don't actually have yet. We will reconsider this *only* if concrete user demand arises.

## Amendment 
**Date:** 2026-09-07

The following constraints are added to fortify the DNS engine's stability and dual-stack compliance:
1. **Panic Recovery in Network Boundaries:** Because `miekg/dns` spawns a new goroutine for every incoming UDP query, any unrecovered panic in the handler acts as a trivial Denial of Service (DoS) attack, crashing the entire DevTether daemon. Therefore, **any goroutine handling DNS queries MUST wrap its logic in a `defer recover()` block.** Furthermore, any locks acquired must be released via an immediate `defer mu.Unlock()` pairing to prevent the recovery from abandoning shared state.
2. **Strict Dual-Stack Coupling:** The current loopback isolation explicitly relies on answering *only* `A` records (IPv4). If `AAAA` (IPv6) support is ever added to the DNS engine, both the DNS Server and Reverse Proxy MUST explicitly bind to `[::1]` (IPv6 loopback) in the exact same change to prevent connection-refused errors for IPv6-preferring clients.
3. **RFC 4074 Dual-Stack Compliance (NXDOMAIN vs NODATA):** The original design mandated `NXDOMAIN` for anything DevTether couldn't answer. This was architecturally flawed. Because modern OS resolvers issue `A` and `AAAA` queries in parallel, returning an authoritative `NXDOMAIN` for a configured route's `AAAA` query will poison the OS cache, randomly causing the valid `A` record lookup to fail. Therefore, the DNS engine MUST decouple route existence from record generation:
   - If the route does *not* exist in the routing table -> Return `NXDOMAIN`.
   - If the route *does* exist, but the query type is unsupported (e.g., `AAAA` or `HTTPS`) -> Return `NOERROR` with 0 answers (NODATA).
