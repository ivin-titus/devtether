# ADR-003: Security Model — Secure by Default, Permissive by Opt-in

**Status:** Accepted
**Date:** 2026-05-30

## Context

DevTether is a networking tool that proxies HTTP traffic, optionally executes shell commands, and can expose local services to the internet. This makes it a high-value attack surface. A single vulnerability in default configuration could compromise a developer's machine or expose sensitive development data.

A pre-mortem analysis of the v1.0 codebase identified several critical security issues:

1. **IPC socket set to `0777`** — Any user on the machine could send commands to the daemon.
2. **DNS bound to `0.0.0.0` by default** — Exposed the DNS server to the entire network without user intent.
3. **Child processes inherited all environment variables** — Including `AWS_SECRET_KEY`, `DATABASE_URL`, etc.
4. **No authentication on any endpoint** — IPC, proxy, and DNS were all unauthenticated.

## Decision

Adopt a **"Secure by Default, Permissive by Opt-in"** security model:

1. Every feature that expands the attack surface requires **explicit opt-in**.
2. The default configuration is the **most restrictive possible**: loopback-only binding, no tunnels, no orchestration, no LAN exposure.
3. Security hardening is phased across the implementation roadmap, with critical mitigations built into the Core Networking Engine.

### Specific Decisions

| Decision | Rationale |
|----------|-----------|
| IPC socket permissions: `0600` | Only the owner can read/write. Prevents local privilege escalation. |
| Socket location: `$XDG_RUNTIME_DIR/devtether/` | Per-user, `0700`, tmpfs-backed. Cleaner than `/tmp`. |
| DNS default bind: `127.0.0.1:53` | Loopback-only. `0.0.0.0` requires `--lan` flag. |
| Proxy targets: loopback-only | Routes can only point to `127.0.0.1:<port>`. Prevents SSRF to internal network. |
| Orchestrator commands: YAML-only by default | Ad-hoc commands via IPC require `--allow-arbitrary` flag. |
| Child process environment: allowlisted | Only `PATH`, `HOME`, `USER`, `SHELL`, `LANG`, `TERM`, `PORT` + explicit `env:` config. |
| WAN tunnel: RBAC forced-on | When a tunnel is active, all services require a token unless explicitly `public:`. |
| Config secrets: `${ENV_VAR}` interpolation | Secrets should never be hardcoded in YAML. Warn if patterns detected. |

## Consequences

### Positive

- Default installations are safe without any security knowledge from the user.
- Security-conscious teams can adopt DevTether with confidence.
- The explicit opt-in model creates a clear audit trail of what is exposed.

### Negative

- More friction for users who "just want it to work" on the LAN — they must pass `--lan`.
- Selective environment passing may break some apps that expect inherited env vars. The `env:` config key provides an escape hatch.
- IPC authentication (session nonce) adds complexity to the CLI ↔ daemon communication path.

## Threat Surface Summary

| Surface | Critical Threats | Key Mitigations |
|---------|-----------------|-----------------|
| IPC Socket | Command execution, route hijacking | `0600`, XDG path, session nonce |
| DNS Engine | Spoofing, network exposure | Loopback-only, strict TLD filter, no upstream forwarding |
| Reverse Proxy | Host injection, SSRF, DoS | Host validation, loopback targets, timeouts, header sanitization |
| Orchestrator | Shell injection, env leakage, zombies | YAML-only commands, `no_new_privs`, selective env, SIGTERM escalation |
| WAN Tunnel | Unauthorized access, MITM, credential theft | mTLS, scoped tokens, forced RBAC |
| Config File | Secrets in Git, tampering | Env interpolation, secret warnings, file permission checks |

### Non-Negotiable Invariant

> **Devtether must never allow a network-reachable endpoint to execute arbitrary shell commands without explicit user consent.**

This invariant must be validated against every feature before shipping.

## Amendment (Phase 8.3)
**Date:** 2026-09-07

The following constraints are added to reinforce the "Secure by Default" model at the network boundary:

1. **Strict Loopback Bindings & Least Privilege:** The proxy engine was previously discovered binding to `""` or `":port"` under fallback conditions, inadvertently exposing developer services to the entire local network (`0.0.0.0`). To fix this:
   - Binding to `""` or `":port"` is strictly forbidden. Network servers (DNS, Proxy, Orchestrator ports) MUST explicitly bind to `127.0.0.1:<port>` to enforce hard loopback isolation.
   - *Future Context:* When Engine 3 (Network Sharing) is implemented, binding to `0.0.0.0` will be permitted ONLY as an explicit, temporary opt-in (e.g., via the `--lan` flag). Even then, the implementation must rigorously follow the principle of least privilege (e.g., restricting broadcast interfaces where possible). Until Engine 3 arrives, `0.0.0.0` bindings are considered a security vulnerability.
2. **Explicit Network Timeouts (DoS Protection):** Go's default `http.Client`, `http.Transport`, and `net.Dialer` have infinite timeouts (`0`), which enables trivial resource exhaustion (e.g., Slowloris, hanging backends). Any client or transport constructed in the codebase MUST set explicit non-zero timeouts (dial, response-header-wait, TLS handshake) to bound resource lifecycles.
