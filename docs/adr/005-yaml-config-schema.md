# ADR-005: Unified YAML Config Schema

**Status:** Accepted
**Date:** 2026-05-30

## Context

Portless supports 4 independent engines, each with different configuration needs. We needed to decide between:

1. **Separate config files per engine** (e.g., `devtether-routes.yaml`, `devtether-orchestrate.yaml`, `devtether-tunnel.yaml`)
2. **A single unified `devtether.yaml`** with top-level keys for each engine

## Decision

Use a **single `devtether.yaml`** with distinct top-level keys for each engine:

```yaml
# Engine 1: Static Routes
routes:
  portfolio.localhost: 3222
  job-flow.localhost: 3223

# Engine 2: Orchestrated Services
orchestrate:
  api:
    domain: api.localhost
    command: uvicorn main:app --port $PORT
    cwd: ./services/api
    restart: on-failure

# Engine 3: Tunnel Config
tunnel:
  relay: dev.yourcompany.com
  lan: true

# Engine 4: Access Control
access:
  enabled: true
  require_token: ["api.*", "admin.*"]
  public: ["app.*", "docs.*"]

# Global Settings
proxy:
  port: 80
  tls: false
  timeouts:
    read: 30s
    write: 60s
    idle: 120s

dns:
  tld: ["localhost", "local"]
  bind: "0.0.0.0:53"
```

### Design Principles

1. **Familiar ergonomics** — The structure mirrors `docker-compose.yaml` conventions. Developers already understand this pattern.
2. **Progressive disclosure** — A minimal config is just 2 lines (`routes:\n  portfolio.localhost: 3222`). Advanced features are opt-in via additional sections.
3. **Engine activation by presence** — If a section is absent, that engine is not loaded. No `enabled: false` boilerplate needed.
4. **Environment variable interpolation** — All string values support `${ENV_VAR}` syntax for secret management.

## Consequences

### Positive

- Single file to manage, version control, and share.
- Clear visual separation of concerns via top-level keys.
- Familiar to anyone who has used Docker Compose, GitHub Actions, or similar YAML-configured tools.
- Minimal valid config is extremely simple, reducing barrier to entry.

### Negative

- Large configs with all 4 engines active could become verbose. Mitigated by clear section headers and documentation.
- YAML parsing errors in one section could prevent all engines from loading. Mitigated by per-section validation with clear error messages indicating which section failed.
