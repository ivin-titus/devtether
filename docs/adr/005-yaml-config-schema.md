# ADR-005: Unified YAML Config Schema

**Status:** Accepted  
**Date:** 2026-05-30  
**Reconciled:** 2026-11-10

## Context

DevTether uses one `devtether.yaml` so current local routing stays simple while future engines can have their own configuration sections.

## Current beta schema

The current binary accepts:

```yaml
routes:
  web.localhost: 3222
  api.localhost: 8042

settings:
  daemon: false
  verbose: false
  log_path: "./.logs"

proxy:
  port: 80
  timeouts:
    idle: 120s

dns:
  bind: "127.0.0.1:5335"
```

The YAML decoder uses strict field checking, so unknown keys are rejected rather than silently ignored.

### Current constraints

- Route names must be valid `.localhost` names.
- `dns.bind` is the exact DNS listener address.
- `proxy.timeouts.read` and `proxy.timeouts.write` are not supported.
- There is no `dns.tld` configuration.
- `.internal` is not a current namespace.
- `orchestrate:`, `tunnel:`, and `access:` are future-engine sections and are currently rejected by validation.

## Future schema

The long-term design may introduce:

```yaml
orchestrate:
  api:
    domain: api.localhost
    command: uvicorn main:app --port $PORT

tunnel:
  relay: dev.example.com
  lan: true

access:
  enabled: true
  require_token: ["api.*"]
  public: ["docs.*"]
```

These are **planning examples only**.

## Design principles

1. Keep the current schema small and explicit.
2. Keep future engine configuration isolated by top-level section.
3. Reject unknown fields so documentation drift fails loudly.
4. Do not add YAML live-reload or runtime persistence until there is a concrete need. Since beta.8, route creation and removal are supported at runtime through the daemon IPC (`POST /routes`, `DELETE /routes/{domain}?inc=N`) without editing or reloading `devtether.yaml`; this runtime-only mutation is not configuration reload and never writes the file. Editing `routes:` in `devtether.yaml` still requires a daemon restart.

## Consequences

One configuration file remains easy to version while strict decoding protects the current binary from silently accepting stale documentation examples.

Reconciled 2026-11-10: runtime route mutation (beta.8) is memory-only and deliberately outside this schema's persistence/reload boundary; the file remains the sole source of static routes.
