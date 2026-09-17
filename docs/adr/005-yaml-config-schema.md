# ADR-005: Unified YAML Config Schema

**Status:** Accepted
**Date:** 2026-05-30

## Context

DevTether's internal architecture relies on 4 independent engines (conceptually grouped into Three Layers), each with different configuration needs. We needed to decide between:

1. **Separate config files per engine** (e.g., `devtether-routes.yaml`, `devtether-orchestrate.yaml`, `devtether-tunnel.yaml`)
2. **A single unified `devtether.yaml`** with top-level keys for each engine

## Decision

Use a **single `devtether.yaml`** with distinct top-level keys for each engine.

> [!NOTE]
> **Beta Scope & TLD Simplification**
> The schema below illustrates the unified structure for all planned engines. However, in the current Beta (Engine 1 only), only the `routes:`, `settings:`, `proxy:`, and `dns:` sections are active. Furthermore, to simplify configuration and prevent route bypasses, the `dns.tld` setting was removed in favor of hardcoding `.localhost`. `.internal` is reserved for the future Engine 3.

### Currently Supported (Engine 1)

> **TLD Restriction**: In the current Engine 1 (Beta), the system strictly enforces the `.localhost` TLD. The configuration does **not** support expanding or modifying the DNS TLD (e.g. attempting to add `.internal` or custom TLDs will not work). This simplification prevents route bypasses and OS resolver complexity.

```yaml
# Engine 1 (Layer 1): Static Routes
routes:
  portfolio.localhost: 3222
  job-flow.localhost: 3223

# Global Settings
settings:
  daemon: false
  verbose: false
  log_path: .logs/devtether.log

proxy:
  port: 80
  timeouts:
    idle: 120s
    read_header: 10s

dns:
  bind: "127.0.0.1:53"
  # Note: `tld` array is NOT supported in Engine 1. It is hardcoded to `.localhost`.
```

### Planned for Future (Engines 2-4)

> **Future TLD Expansion**: When Engine 3 (Tunnel/LAN) is released, DevTether will officially support expanding the DNS scope to `.internal` and potentially other TLDs for LAN and WAN sharing.

```yaml
# Engine 2 (Layer 2): Orchestrated Services
orchestrate:
  api:
    domain: api.localhost
    command: uvicorn main:app --port $PORT
    cwd: ./services/api
    restart: on-failure

# Engine 3 (Layer 3): Tunnel Config
tunnel:
  relay: dev.yourcompany.com
  lan: true

# Engine 4 (Layer 3): Access Control
access:
  enabled: true
  require_token: ["api.*", "admin.*"]
  public: ["app.*", "docs.*"]
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
