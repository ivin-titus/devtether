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

1. **Eliminate port memorization** — Replace `localhost:PORT` with clean named domains (e.g., `portfolio.localhost`).
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
grafana    → localhost:3001
job-flow   → localhost:3223
api        → localhost:8042
```

Developers must constantly remember these mappings. Port conflicts (`EADDRINUSE`) derail flow state. Sharing with teammates requires Ngrok subscriptions or manual IP+port sharing.

### The Solution

```text
frontend   → portfolio.localhost
backend    → api.localhost
admin      → admin.localhost
grafana    → grafana.localhost
job-flow   → job-flow.localhost
api        → api.job-flow.localhost
```

One YAML file. One binary. One command: `devtether up`.

---

## 5. The 4 Engines

### Engine 1: Static Routing (`devtether route`)

**The zero-friction entry point.** Maps pre-existing services on fixed ports to named domains. No process management, no port injection. DevTether just handles DNS + proxy.

```yaml
routes:
  portfolio.localhost: 3222
  job-flow.localhost: 3223
  api.job-flow.localhost: 8042
```

**Key differentiator vs. Vercel DevTether:** Vercel's tool *cannot* route to pre-existing ports. It forces all apps through its process wrapper. Our static routing respects existing workflows.

### Engine 2: Process Orchestration (`devtether orchestrate`)

**The Vercel DevTether competitor.** Spawns processes, injects dynamic `$PORT`, manages process trees with `Setpgid`, captures logs with service-name prefixes.

```yaml
orchestrate:
  api:
    domain: api.localhost
    command: uvicorn main:app --port $PORT
    cwd: ./services/api
```

**Coexistence:** Static routes and orchestrated services share the same DNS + Proxy engines and can coexist in one `devtether.yaml`.

### Engine 3: Tunneling (`devtether tunnel`)

**The self-hosted Ngrok alternative.** Two modes:

- **LAN Mode:** Binds to `0.0.0.0`, broadcasts via mDNS. Any device on the same WiFi/VPN can access services.
- **WAN Mode:** Connects to a self-hosted `devtether-relay` on a VPS. Exposes local services at `https://app.dev.yourcompany.com` with auto-provisioned Let's Encrypt certificates.

**Truly self-hosted.** Zero SaaS dependency. You own the domain, server, and certificates.

### Engine 4: Access Control (RBAC)

**The enterprise signal.** Token-based access control at the proxy layer.

- Generate scoped tokens: `devtether access grant --name "frontend-team" --services "api.*" --expires 7d`
- Tokens are HMAC-SHA256 signed JWTs validated before forwarding.
- WAN tunnels force RBAC on by default — no opt-out.
- Future: SSO integration, policy-as-code.

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
| `gorilla/websocket` | Tunnel WebSocket transport (Phase 4) |
| `golang.org/x/crypto/acme` | Let's Encrypt on relay (Phase 4) |

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

## 11. Implementation Phases

| Phase | Name | Deliverable |
|-------|------|-------------|
| 1 | Foundation | Static routing, DNS, proxy with graceful shutdown |
| 2 | Orchestration | Process supervisor, dynamic port injection |
| 3 | LAN Sharing | mDNS broadcasting, `0.0.0.0` binding |
| 4 | WAN Tunneling | Self-hosted relay, WebSocket tunnels |
| 5 | Access Control | Token-based RBAC |
| 6 | Polish & Community | Docs, CI/CD, goreleaser, community outreach |

Detailed task breakdowns are tracked per-phase in the project's issue tracker.

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
| Static Routing (Engine 1) | ✅ Complete |
| Orchestration (Engine 2) | 🔲 Not yet implemented (Phase 2) |
| LAN Sharing (Engine 3) | 🔲 Not yet implemented (Phase 3) |
| WAN Tunneling (Engine 3) | 🔲 Not yet implemented (Phase 4) |
| RBAC (Engine 4) | 🔲 Not yet implemented (Phase 5) |

**Legend:** ✅ Production-ready | 🔲 Not started

---

*This PRD is a living document. It will be updated as the project evolves through its implementation phases.*
