# ADR-002: DNS Design — Non-Recursive, Loopback-Only

**Status:** Accepted
**Date:** 2026-05-30

## Context

Portless embeds a DNS server (`miekg/dns`) to resolve configured TLDs (`.localhost`, `.local`, `.test`) to the developer's machine. This eliminates the need for manual `/etc/hosts` editing.

However, running a DNS server introduces significant security risks:

1. **Open resolver abuse** — If Portless forwards unmatched queries upstream, it becomes an open DNS resolver that can be exploited for DNS amplification attacks.
2. **Network exposure** — Binding to `0.0.0.0:53` (the original default) exposes the DNS server to every device on the network, even when the developer only intends to use Portless locally.
3. **Per-query overhead** — The original implementation called `net.Dial("udp", "8.8.8.8:80")` on every DNS query to determine the local IP, adding unnecessary syscall overhead.

## Decision

1. **Non-recursive by design** — Portless will **never** forward DNS queries to upstream resolvers. It only answers queries for its own configured TLDs. All other queries receive `NXDOMAIN`. This is a permanent, non-negotiable design decision.
2. **Loopback-only by default** — DNS binds to `127.0.0.1:53` by default. It only binds to `0.0.0.0:53` when `--lan` mode is explicitly enabled.
3. **Cached LAN IP** — The local IP is resolved once on startup and cached. It is refreshed only on detected network changes (interface up/down events), not per-query.

## Consequences

### Positive

- Eliminates the entire class of DNS amplification and open resolver vulnerabilities.
- Default configuration is safe — a developer who just runs `devtether up` cannot accidentally expose their DNS to the network.
- Reduced per-query latency by eliminating the UDP dial overhead.

### Negative

- Developers must configure their OS resolver (e.g., `systemd-resolved`) to forward specific TLDs to `127.0.0.1:53`. This is a one-time setup step documented in the README.
- In LAN mode, the DNS server is exposed to the local network, which is a deliberate and accepted tradeoff for the LAN sharing feature.
