# ADR-001: Modular Engine Architecture

**Status:** Accepted
**Date:** 2026-05-30

## Context

The initial version of DevTether was built as a monolithic daemon that tightly coupled process orchestration, DNS resolution, and reverse proxying into a single `devtether start` command. This created several problems:

1. **High cognitive load** — Users who only wanted simple domain-to-port mapping were forced to learn the orchestration model, understand `$PORT` injection, and rewrite their framework startup commands.
2. **All-or-nothing** — There was no way to use just the DNS+proxy layer without the process supervisor.
3. **Limited scope** — The architecture had no extension points for tunneling or access control.

During testing, we found that pointing Portless at a real Next.js app inside a pnpm monorepo required convoluted CLI commands (`pnpm --filter portfolio exec next dev --turbopack -p $PORT`) just to bypass the framework's default port configuration. This friction defeated the tool's purpose of reducing developer cognitive load.

We also identified that existing tools in the ecosystem (Vercel Portless, frp, Ngrok) each solve only one piece of the local networking puzzle. No single tool combines static routing + orchestration + tunneling + access control.

## Decision

Restructure Portless into **4 independent engines** inside a single binary:

1. **Engine 1: Static Routing** — Pure DNS + proxy. Maps domains to pre-existing ports. Zero process management.
2. **Engine 2: Orchestration** — Process supervisor with dynamic `$PORT` injection. Opt-in only.
3. **Engine 3: Tunneling** — LAN sharing (mDNS) and WAN tunneling (self-hosted relay). Opt-in only.
4. **Engine 4: Access Control** — Token-based RBAC at the proxy layer. Opt-in only.

Each engine is activated by its presence in the `devtether.yaml` config. If a section is absent, that engine is not loaded.

## Consequences

### Positive

- **Reduced friction** — Users who just want `portfolio.localhost → 3222` only need the `routes:` section. No process management knowledge required.
- **Incremental adoption** — Teams can start with static routing and progressively adopt orchestration, tunneling, and RBAC as their needs grow.
- **Competitive advantage** — No other tool in the ecosystem offers this combination in a single binary.
- **Testability** — Each engine can be unit-tested independently.

### Negative

- **Increased codebase complexity** — More packages, more interfaces, more integration points.
- **Documentation burden** — Each engine needs its own documentation, examples, and troubleshooting guides.
- **Risk of feature creep** — Must maintain discipline about what belongs inside Portless vs. what should be a separate tool.

### Mitigation

- Strict package boundaries (`internal/dns`, `internal/proxy`, `internal/orchestrator`, `internal/tunnel`, `internal/access`)
- Shared infrastructure (Router, DNS, Proxy) is engine-agnostic and doesn't import engine-specific packages
- The `devtether.yaml` schema enforces clear separation — each engine has its own top-level key
