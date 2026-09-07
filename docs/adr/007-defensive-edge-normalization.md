# ADR 007: Defensive Edge Normalization

## Status
Accepted

## Context
During the extensive Phase 1-7 audit, we discovered multiple edge cases where the routing engine, DNS engine, and proxy engine disagreed on string normalization for network identifiers (Domains, Host headers, and IP addresses). 
Specific vulnerabilities included:
- **Trailing Dots:** A domain inserted with a trailing dot (e.g., `api.localhost.`) failed to match because `AddRoute` didn't strip it, while the proxy did.
- **IPv6 Literals:** `net.SplitHostPort("[::1]")` returns an error if no port is present, causing the proxy to retain brackets, while the router expected bare IPs (`::1`).
- **Casing:** Mixed-case `Host` headers (`App.Localhost`) were not correctly lowercased in all pathways, causing cache misses.

Because normalization logic was duplicated across `internal/router`, `internal/proxy`, and `internal/dns`, inconsistencies emerged rapidly, breaking core routing functionality.

## Decision
We mandate a **Single Source of Truth for Edge Normalization**:
Host and domain normalization MUST live in exactly one centralized function (e.g., `internal/netutil.NormalizeHost`).
This function MUST handle normalization in the following strict order:
1. Strip a `[...]` IPv6 literal **only** when both the leading `[` and trailing `]` are present. A blanket character-class trim (`strings.Trim(host, "[]")`) is strictly forbidden, as it silently mangles malformed input.
2. Strip the port via `net.SplitHostPort`, falling back to the original string **only** on a confirmed "missing port" `*net.AddrError`.
3. Strip any trailing FQDN dot (`.`).
4. Convert the string to lowercase.

Every producer (e.g., config parser, `AddRoute`) and every consumer (e.g., DNS resolver, proxy `ServeHTTP`) MUST call this exact function immediately at the system boundary. 

## Consequences
- **Positive:** Eradicates an entire class of cache-miss and routing-bypass bugs. Guarantees consistency across all engines.
- **Negative:** Slightly increases function call overhead at the ingress boundary, but this is negligible.
- **Enforcement:** A table-driven test must cover all edge cases (empty strings, bare IPv6, IPv6 with ports, trailing dots, mixed casing). Any Pull Request introducing redundant normalization logic is considered an engineering standards violation.
