# PRD — DevTether: Local Development Networking Toolkit

> *Version 2.1 — Revised 2026-09-28*

---

## 1. Overview

DevTether is a self-hosted local development networking toolkit written in Go. Its current beta focuses on one problem: making multiple already-running local HTTP services easier to address without memorizing ports.

The current release provides named `.localhost` routing through embedded DNS and a loopback-only HTTP reverse proxy. The longer-term product direction is a four-engine platform that can add process orchestration, LAN/WAN sharing, and access control without making those concerns mandatory for users who only need local routing.

---

## 2. Product Goals

### Current beta goals

1. Replace `localhost:PORT` with named `.localhost` domains for existing HTTP services.
2. Keep the routing layer small, predictable, and secure by default.
3. Provide a single Go binary with no external runtime services.
4. Make DNS setup and daemon lifecycle understandable through the CLI.

### Future goals

5. Optionally manage application processes and dynamically allocate ports.
6. Optionally share development services over a LAN or self-hosted WAN relay.
7. Add scoped access control for shared services.
8. Preserve explicit opt-in for features that expand the attack surface.

### Non-Goals

- Replace Kubernetes or Docker Compose for production orchestration.
- Act as a production reverse proxy.
- Provide generic TCP/database proxying in the current networking engine.
- Provide full identity management or SSO.
- Manage containers or VMs.

---

## 3. Target Users

### Current beta

- Solo developers running multiple local HTTP services.
- Open-source contributors who want readable local domains.
- Students and developers building multi-service projects.

### Future expansion

- Small teams that need controlled LAN access to local services.
- DevOps engineers evaluating self-hosted development tunnels.
- Teams that want process orchestration and scoped sharing from one tool.

---

## 4. Core Problem

Without a local routing layer, a project can quickly become a collection of ports:

```text
frontend → localhost:3000
backend  → localhost:8000
admin    → localhost:8080
monitor  → localhost:3001
```

The current product addresses the naming problem without taking ownership of how those applications are started.

```text
frontend → web.localhost
backend  → api.localhost
admin    → admin.localhost
monitor  → monitor.localhost
```

The current beta uses a static `devtether.yaml` route table and requires a daemon restart after route changes.

---

## 5. Current Beta Scope — Engine 1

### Static routing

Maps a named `.localhost` host to an already-running local HTTP service:

```yaml
routes:
  web.localhost: 3222
  api.localhost: 8042
```

### Embedded DNS

- Default bind: `127.0.0.1:5335`.
- Route-aware: only configured routes receive an address.
- A records for configured routes resolve to `127.0.0.1`.
- Unsupported qtypes for an existing route return NODATA.
- Unknown routes return NXDOMAIN.
- No upstream DNS forwarding.

### HTTP reverse proxy

- Default configured bind: `127.0.0.1:80`.
- On permission/address-in-use failure, beta.7 falls back to `127.0.0.1:8080`.
- There is no OS-assigned `:0` fallback.
- Backend targets are loopback-only.
- Host validation is strict.
- WebSocket and streaming-style connections are supported without fixed request read/write deadlines.

### Daemon and CLI

Current commands:

- `init`
- `up` / `start`
- `down` / `stop`
- `status`
- `routes`
- `logs`
- `doctor`
- `version`

The daemon uses a Unix-socket IPC API, instance locking, PID and nonce state, and graceful shutdown.

### Current setup integration

`devtether init` can generate the current config and, with explicit consent, configure supported Linux/macOS DNS resolver integrations. The default DNS port is always `5335`; changing it manually requires aligning the host resolver configuration.

---

## 6. Target Architecture

DevTether is designed around four independent engines.

### Engine 1 — Local Static Routing

Current:
- static route table;
- embedded DNS;
- HTTP reverse proxy;
- daemon/IPC lifecycle.

### Engine 2 — Orchestration

Planned:
- process groups;
- lifecycle supervision;
- dynamic `$PORT` allocation;
- unified process logging.

### Engine 3 — Tunneling

Planned:
- LAN sharing;
- self-hosted WAN relay;
- optional separate `devtether-relay` binary.

### Engine 4 — Access Control

Planned:
- scoped service tokens;
- RBAC;
- access policy enforcement for shared services.

The current beta does not activate the future engines through configuration; unsupported future sections are rejected.

---

## 7. Competitive Position

DevTether's current scope is intentionally narrower than its long-term architecture. The beta is primarily a local static routing tool rather than an all-in-one tunnel/orchestration platform.

The longer-term architecture is designed to combine local DNS, HTTP routing, process awareness, self-hosted sharing, and access control without requiring all of those features for local-only use.

---

## 8. Technology

### Current

- Go 1.27.1+
- `miekg/dns`
- `spf13/cobra`
- `gopkg.in/yaml.v3`
- `golang.org/x/sync/errgroup`
- `golang.org/x/term`
- `github.com/fsnotify/fsnotify`

The release build uses `CGO_ENABLED=0`.

### Future

The final dependency set for tunneling, orchestration, and access-control engines has not been committed. Future dependencies must follow ADR-006 and the engineering standards.

---

## 9. Performance Targets

These are engineering targets, not guarantees for every workload.

| Metric | Target |
|---|---|
| Idle memory | < 15 MB |
| Loaded memory | < 50 MB |
| Added proxy latency | ~1 ms or less |
| Cached DNS response | sub-millisecond target |

Binary size and resource usage may vary by platform and release configuration.

---

## 10. Security Model

### Current beta surfaces

1. IPC socket — owner-only permissions, instance locking, and authenticated shutdown.
2. DNS engine — loopback-only, route-aware, non-recursive.
3. Reverse proxy — strict host validation, loopback targets, bounded header-wait/idle behavior.
4. Configuration — strict YAML fields and route validation.

### Future surfaces

Orchestration, LAN/WAN sharing, relay transport, RBAC, and a future GUI each receive additional threat modeling before implementation. See [ADR-003](adr/003-security-model.md) and [ADR-009](adr/009-proxy-security-lessons.md).

---

## 11. Roadmap

| Engine | Current status | Next architectural area |
|---|---|---|
| Engine 1 | ✅ Beta | Stabilization and incremental DX improvements |
| Engine 2 | 🔲 Planned | Process orchestration |
| Engine 3 | 🔲 Planned | LAN/WAN tunneling |
| Engine 4 | 🔲 Planned | Access control |

Dynamic route reloading, a Web GUI, traffic inspection, and on-demand health checks remain separate future proposals and are not part of the current beta.

---

## 12. Success Measures

For the current beta, useful signals include:

- successful first-run setup;
- reliable `.localhost` routing;
- predictable daemon lifecycle;
- clear failure messages;
- no critical vulnerabilities in the default local-only configuration.

Numeric adoption targets may be tracked separately as product goals; they are not implementation requirements.

---

## 13. Project Status

The current beta delivers Engine 1. Future engines are documented here so their boundaries are explicit before implementation begins.

*This PRD is a living product document. Current behavior should be verified against the source and tests; target architecture should not be read as shipped capability.*
