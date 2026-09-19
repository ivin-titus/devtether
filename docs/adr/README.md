# Architectural Decision Records (ADR) Index

This directory contains the Architectural Decision Records for the DevTether project.

ADRs document the key technical decisions made during the design and implementation of DevTether. They capture the context, decision, and consequences so that future contributors understand *why* things are built the way they are.

## Index

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [001](001-modular-engine-architecture.md) | Modular Engine Architecture | ✅ Accepted | 2026-05-30 |
| [002](002-dns-design.md) | DNS Design: Non-Recursive, Loopback-Only | ✅ Accepted | 2026-05-30 |
| [003](003-security-model.md) | Security Model: Secure by Default | ✅ Accepted | 2026-05-30 |
| [004](004-relay-as-separate-binary.md) | Relay as Separate Binary | ✅ Accepted | 2026-05-30 |
| [005](005-yaml-config-schema.md) | Unified YAML Config Schema | ✅ Accepted | 2026-05-30 |
| [006](006-platform-support-and-cgo-policy.md) | Platform Support & CGo Policy | ✅ Accepted | 2026-09-02 |
| [007](007-defensive-edge-normalization.md) | Defensive Edge Normalization | ✅ Accepted | 2026-09-07 |
| [008](008-proxy-streaming-integrity.md) | Proxy Streaming Integrity | ✅ Accepted | 2026-09-07 |
| [009](009-proxy-security-lessons.md) | Proxy Security Lessons & Defense-in-Depth | ✅ Accepted | 2026-09-08 |
| [010](010-ai-contribution-liability.md) | AI Contribution Liability | ✅ Accepted | 2026-09-12 |
| [011](011-init-wizard-and-system-mutations.md) | Init Wizard & System Mutations | ✅ Accepted | 2026-09-19 |

## ADR Format

Each ADR follows this structure:

1. **Title** — Short, descriptive name
2. **Status** — Proposed / Accepted / Deprecated / Superseded
3. **Context** — What problem or decision prompted this?
4. **Decision** — What was decided?
5. **Consequences** — What are the tradeoffs?

## Related Documentation

- [Architecture Overview](../architecture.md) — system design and package responsibilities
- [Engineering Standards](../engineering-standards.md) — code quality rules for contributors
