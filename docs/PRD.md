# PRD — DevTether: The Local Development Networking Protocol

> *Version 2.0 — Revised 2026-05-30*
>
> *"The missing networking layer for local development."*

---

## 1. Overview

**DevTether** is a modular, self-hosted developer networking toolkit that replaces the fragmented mess of port memorization, reverse proxy configs, ngrok subscriptions, and ad-hoc LAN sharing scripts with a single, lightweight Go binary.

Where Docker networking solves container-to-container communication, DevTether solves **developer-to-developer** and **developer-to-service** communication on bare metal. It is the networking layer that should have existed between `localhost` and production.

DevTether is built around **4 independent engines** that coexist inside one binary. Developers opt-in to the engines they need — they never pay cognitive load for features they don't use.

---

## 2. Goals

### Primary Goals

1. **Eliminate port memorization** — Replace `localhost:PORT` with clean named domains (e.g., `web.localhost`).
2. **Zero-config static routing** — Map existing services running on fixed ports to named domains with one command.
3. **Optional process orchestration** — For microservice-heavy setups, auto-allocate ports and manage process lifecycles.
4. **Self-hosted environment sharing** — Enable LAN sharing (same WiFi/VPN) and WAN tunneling (self-hosted relay) without third-party SaaS.
5. **Access control** — Provide token-based RBAC so developers can control who accesses which services.
6. **Security by default** — Every feature that expands the attack surface requires explicit opt-in.

### Non-Goals

- Replace Kubernetes or Docker Compose for production orchestration.
- Act as a production reverse proxy (Nginx, Caddy, Traefik).
- Provide full identity management or SSO (future evolution, not MVP).
- Manage containers or VMs.

---

## 3. Target Users

### Primary

- **Solo developers** running 2–5 local services who are tired of port numbers.
- **Small team leads** at startups who want teammates to access their local backend without Ngrok.
- **DevOps engineers** looking for a lightweight, self-hosted alternative to tunnel SaaS.

### Secondary

- **Open-source contributors** who need a fast way to spin up multi-service dev environments.
- **Students** building microservice projects who want portfolio-worthy infrastructure.
- **Enterprise dev teams** (mid-scale) who need access-controlled environment sharing on corporate VPNs.

### Typical Stacks

- Next.js, Vite, Nuxt, Astro (frontend)
- FastAPI, Django, Express, Go APIs (backend)
- PostgreSQL, Redis, Elasticsearch (databases — static port routing)

---

## 4. Core Problem

### The Port Problem

```text
frontend   → localhost:3000
backend    → localhost:8000
admin      → localhost:8080
monitor    → localhost:3001
worker     → localhost:3223
db         → localhost:8042
```

Developers must constantly remember these mappings. Port conflicts (`EADDRINUSE`) derail flow state. Sharing with teammates requires Ngrok subscriptions or manual IP+port sharing.

### The Solution

```text
frontend   → web.localhost
backend    → api.localhost
admin      → admin.localhost
monitor    → monitor.localhost
worker     → worker.localhost
db         → db.worker.localhost
```

One YAML file. One binary. One command: `devtether up`.

---

## 5. The Three-Layer Architecture

DevTether is conceptually built around three independent layers that coexist inside one binary *(internally powered by 4 highly modular engines — see [ADR-001](adr/001-modular-engine-architecture.md))*. Developers opt-in to the layers they need — they never pay cognitive load for features they don't use.

### Layer 1: The Networking Layer
**The zero-friction entry point.** Handles all traffic, routing, and local network topologies.
- **Static Routing:** Maps pre-existing services on fixed ports to named `.localhost` domains.
- **Intelligent IP Cycling:** Dynamically binds to `127.0.0.x` loopback addresses, avoiding port conflicts by actively scanning for `0.0.0.0` bindings (highly optimized O(1) checks).
- **Traffic Inspection:** Buffers network payloads via `sync.Pool` (zero-bloat) and streams them via IPC for 1-click webhook replays in the GUI.
- **Smart Project-Boundary CORS:** Automatically injects CORS headers for intra-project traffic (e.g., `web.localhost` to `api.web.localhost`) while blocking cross-project local access to establish a base layer of local security.
- **Rich Error Pages:** Serves ultra-lightweight Cloudflare-style HTML error pages if a backend is down, functioning perfectly even if the GUI process is offline.

### Layer 2: The Process Orchestrator Layer
**The Vercel DevTether competitor.** Manages the lifecycle of developer applications (Node, Go, Python).
- **Process Groups:** Orchestrates apps into isolated Process Groups (PGIDs) for clean shutdown (`devtether stop <group>`).
- **Dynamic Ports:** Allocates ephemeral `$PORT` environment variables.
- **Unified Logging:** Captures stdout/stderr and prefixes them (e.g., `[app | api]`) to clearly separate them from network access logs (`[proxy | web]`).

### Layer 3: The Access Controls Layer
**The enterprise signal.** Secures cross-network and cross-org collaboration.
- **Centralized RBAC & IAM:** A self-hosted identity layer controlling who can access which local services when exposed over LAN (mDNS) or WAN (relay tunnels).
- **Tokens & Groups:** Ensures that a frontend teammate can access the `api` service, but not the local `admin` database. WAN tunnels force RBAC on by default.

---

## 6. Competitive Landscape

| Tool | What it does | Gap we fill |
|------|-------------|-------------|
| **Vercel DevTether** | Named `.localhost`, dynamic ports, monorepo support | No static routing. No tunneling. No RBAC. Node.js only. |
| **frp** | TCP/UDP reverse proxy and tunneling | Complex config. Not dev-focused. No DNS. |
| **Ngrok** | Instant public tunnels | SaaS with strict limits. Not self-hosted. |
| **Caddy / Nginx** | Production reverse proxying | Manual config. No DNS. No process awareness. |
| **Cloudflare Tunnel** | Secure outbound tunneling + Zero Trust | Vendor lock-in. Complex ACL setup. |

**Our unique position:** No single tool combines local DNS + reverse proxy + process orchestration + self-hosted tunneling + RBAC into one developer-first binary.

---

## 7. Technology Stack

**Language:** Go

**Rationale:**
- Single static binary — zero runtime dependencies (no Node.js, no Python)
- Excellent networking primitives (`net/http`, `net`, `crypto/tls`)
- Native concurrency (`goroutines`, `errgroup`)
- Cross-compilation for Linux, macOS, Windows

**Key Dependencies:**

| Package | Purpose |
|---------|---------|
| `miekg/dns` | Embedded DNS resolver |
| `spf13/cobra` | CLI framework |
| `gopkg.in/yaml.v3` | YAML config parsing |
| `golang.org/x/sync/errgroup` | Concurrent server lifecycle |
| `gorilla/websocket` | Tunnel WebSocket transport (Future) |
| `golang.org/x/crypto/acme` | Let's Encrypt on relay (Future) |

---

## 8. Performance Targets

| Metric | Target |
|--------|--------|
| Memory | < 15 MB idle, < 50 MB under load |
| CPU | Near idle (event-driven, not polling) |
| Binary size | < 20 MB |
| Proxy latency | < 1ms added per request |
| DNS response | < 0.5ms for cached queries |

---

## 9. Security Model

**Guiding principle:** Secure by default, permissive by opt-in.

See [ADR-003: Security Model](adr/003-security-model.md) for the complete threat model covering 6 attack surfaces:

1. IPC Socket — `0600` permissions, XDG_RUNTIME_DIR, session nonce auth
2. DNS Engine — Loopback-only by default, strict TLD filtering, no upstream forwarding
3. Reverse Proxy — Host validation, loopback-only targets, connection timeouts, header sanitization
4. Process Orchestrator — YAML-only commands, selective env passing, `no_new_privs`
5. WAN Tunnel — mTLS, scoped registration tokens, RBAC forced-on
6. Configuration — Env var interpolation, secret pattern warnings, file permission checks

---

## 10. Success Metrics

| Metric | Target |
|--------|--------|
| GitHub stars (6 months) | 500+ |
| Setup time for new user | < 2 minutes |
| Zero-config static routing | Works on first try |
| Graceful shutdown | < 2 seconds, zero orphan processes |
| Security audit | Zero critical vulnerabilities in default config |

---

## 11. Implementation Roadmap

| Stage | Name | Deliverable |
|-------|------|-------------|
| **1** | **Core Networking Engine** | Local proxy, DNS embedded resolver, static routing via `devtether.yaml` |
| **2** | **Orchestration & IPC** | Unix socket daemon, dynamic CLI commands (`devtether link`) |
| **3** | **LAN Sharing** | Bind to `0.0.0.0`, simple token auth, Web UI dashboard |
| **4** | **WAN Tunnels** | Public URL routing via cloud relay, Let's Encrypt integration |
| **5** | **Zero Trust** | Cloudflare Access integration, strict RBAC, Audit Logs |

Detailed task breakdowns are tracked per-stage in the project's issue tracker.

---

## 12. Project Status

| Component | Status |
|-----------|--------|
| DNS Engine | ✅ Production-ready (loopback-only, route-aware, port fallback) |
| Reverse Proxy | ✅ Production-ready (graceful shutdown, timeouts, host validation) |
| Routing Engine | ✅ Production-ready (thread-safe, hot-reloadable) |
| Config Loader | ✅ Production-ready (validation, defaults, legacy detection) |
| IPC Daemon | ✅ Production-ready (XDG socket, 0600 permissions) |
| CLI (Cobra) | ✅ Production-ready (`up`, `routes` commands) |
| Layer 1: Core Networking | ✅ Production-ready (Local Static Routing Complete) |
| Layer 2: Orchestration | 🔲 Planned |
| Layer 3: Access Control (LAN/WAN + RBAC) | 🔲 Planned |

**Legend:** ✅ Production-ready | 🔲 Planned

---

*This PRD is a living document. It will be updated as the project evolves through its implementation roadmap.*---



