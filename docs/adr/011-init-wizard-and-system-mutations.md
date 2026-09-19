# ADR-011: Init Wizard & System Mutations

**Status:** Accepted
**Date:** 2026-09-19

## Context

Initially (as documented in ADR-002), configuring the host OS DNS resolver to forward `.localhost` traffic to DevTether's daemon was considered a strictly manual process. Users were expected to create configuration files in `/etc/systemd/resolved.conf.d/` or macOS's `/etc/resolver/` by hand.

However, during early beta testing, this proved to be a significant UX friction point. We needed a way to automate this setup without compromising our security model or silently escalating privileges without user consent. Furthermore, attempting to fall back from 53 to 5353 dynamically proved unreliable, leading to a shift to an unprivileged fail-fast default (5335).

## Decision

We introduced an interactive CLI wizard (`devtether init`) that explicitly requests permission to mutate the host OS resolver and apply `setcap` capabilities.

1. **Interactive Explicit Consent:** The wizard uses prompt-based interactions. It explicitly tells the user what command will be run (e.g., `sudo mkdir -p ...`) and requires a `y` confirmation.
2. **Dynamic Port Injection:** The wizard detects the actual port DevTether intends to bind to (which is safely defaulted to `5335`) and hardcodes this specific port into the OS resolver configuration and `devtether.yaml` `dns.bind`. This completely eliminates any race condition/drift where the OS and daemon expect different ports.
3. **Automated `sudo` Escalation:** Instead of requiring the entire `devtether init` command to be run as root (which would create root-owned config files in `$HOME`), the CLI runs as a standard user and escalates privileges *only* for the specific filesystem mutation commands using `sudo`.
4. **CI/CD Bypassing:** The wizard is strictly interactive. If the CLI detects a non-TTY environment (e.g., a CI pipeline), the interactive prompts are bypassed entirely unless specific automation flags (e.g., `--daemon`) are provided.

## Consequences

### Positive
- **Zero-Friction Onboarding:** Users can start routing `.localhost` domains in under 10 seconds without manually touching OS internals.
- **Perfect Port Synchronization:** The OS resolver and the daemon are permanently synced to the same port.
- **Secure File Ownership:** The user's `devtether.yaml` remains owned by their standard user account, not `root`.

### Negative
- **Increased Code Complexity:** The CLI must now understand the intricacies of `systemd-resolved`, `dnsmasq`, and macOS resolvers, as well as `sudo` path environments.
- **Maintenance Burden:** OS resolver paths and behaviors change across OS upgrades (e.g., macOS Sequoia vs Sonoma). We must actively maintain these integration points.
