# <img src=".assets/devtether_logo.png" height="48" align="center" alt="DevTether Logo" /> DevTether

> **Single binary. Named domains. Zero hassle.**

DevTether is a modular, self-hosted developer networking toolkit for macOS and Linux. It replaces port memorization, reverse proxy configs, and ngrok subscriptions with clean, named `.localhost` domains—all from a single, lightweight Go binary.

```text
Before:                          After:
localhost:3222        👉         web.localhost
localhost:3223        👉         api.localhost
localhost:8042        👉         db.localhost
```

> [!WARNING]
> **Project Status: Beta**  
> DevTether is currently in Beta. Engine 1 (Static Routing) is complete - its configuration schema, CLI API, and core architecture are stable. Breaking changes to these will only occur with strong consensus and will be logged in the [CHANGELOG](CHANGELOG.md).

---

## ⚡ Quick Start

```bash
# 1. Install (see Installation below)
# 2. Run the interactive setup wizard
devtether init

# 3. Add your domains to devtether.yaml
#    routes:
#      web.localhost: 3222
#      api.localhost: 8042

# 4. Start routing!
devtether up -d
```

## 🛠️ Installation

### Option 1: Quick Install (Recommended)

```bash
curl -sSfL https://raw.githubusercontent.com/ivin-titus/devtether/main/scripts/install.sh | sh
```
The script auto-detects your OS and architecture, downloads the latest release, and verifies the checksum.

### Option 2: Download Binary

Download the latest release for your platform from [GitHub Releases](https://github.com/ivin-titus/devtether/releases), extract it, and move it to your `$PATH` (e.g., `~/.local/bin/`).

### Option 3: Build from Source

Requires Go 1.27.1+.
```bash
git clone https://github.com/ivin-titus/devtether.git
cd devtether
make build && make install
```

---

## 🪄 Post-Install Setup: The Wizard

Configuring your OS to natively resolve `.localhost` domains used to involve tedious, OS-specific manual steps. We fixed that. 

Simply run:
```bash
devtether init
```

The interactive wizard will:
1. Detect your OS and DNS resolver (e.g., `systemd-resolved` or `dnsmasq` on Linux, `/etc/resolver` on macOS).
2. Generate a secure `devtether.yaml` configuration.
3. Explicitly ask for permission to apply OS-level DNS routing and port capabilities (using targeted `sudo` commands).

*(Note: If you run a custom Linux DNS setup without systemd or dnsmasq, the wizard gracefully skips the automatic system mutations and leaves you in full control, while still generating a valid config file! See our [Custom Linux DNS Guide](docs/advanced-port-configuration.md#customizing-the-dns-port) to configure your system manually.)*

**Under the Hood:** DevTether uses an embedded DNS server and a reverse proxy. The wizard safely points your OS resolver to DevTether's internal DNS. If you're curious about how this remains secure-by-default, avoids privilege escalation, and binds ports safely, read our deep-dive in [ADR-011: Init Wizard & System Mutations](docs/adr/011-init-wizard-and-system-mutations.md) and the [Architecture Overview](docs/architecture.md).

---

## 💻 Usage

### Configuration (`devtether.yaml`)
Map your already-running services to clean domains. (The `devtether init` command generates a fully commented reference for you).

```yaml
routes:
  web.localhost: 3222
  api.localhost: 3223
  db.localhost: 8042
```
> *Note: Route changes currently require a quick restart (`devtether down && devtether up`).*
>
> 💡 **Power User?** If you need to manually customize your proxy or DNS ports after initialization, see the [Advanced Port Configuration Guide](docs/advanced-port-configuration.md).

### Core Commands

```bash
devtether up                        # Start the routing daemon in foreground
devtether up -d                     # Start in the background (logs to .logs/)
devtether down                      # Stop the background daemon gracefully
devtether status                    # Show daemon status (PID, uptime, route count, heap)
devtether routes                    # Show active routes (live from daemon)
devtether logs -f                   # Tail the daemon logs
devtether doctor                    # Check system environment for common issues
```

> **Log Management:** If your log file gets too large while running in the background, you can safely clear it without restarting the daemon by running this command in your terminal:
> ```bash
> > .logs/devtether.log
> ```

---

## 🗺️ Architecture & Roadmap

DevTether is evolving from a simple reverse proxy into a comprehensive Developer Platform, structured into **Three Main Layers** inside a single Go binary. *(See [ADR-001](docs/adr/001-modular-engine-architecture.md) for details).*

| Layer | Purpose | Status |
|-------|---------|--------|
| **1. Networking Layer** | Reverse Proxy, DNS, Smart CORS, Traffic Inspection | ✅ Beta Complete |
| **2. Process Orchestrator** | Process Groups, ephemeral ports, unified logging | 🔲 Planned |
| **3. Access Controls** | Centralized IAM, RBAC, cross-network collaboration tokens | 🔲 Planned |

**Platform Support:** Fully supported on macOS and Linux (amd64, arm64). Windows is not currently supported ([ADR-006](docs/adr/006-platform-support-and-cgo-policy.md)).

---

## 📚 Documentation

- [Architecture Overview](docs/architecture.md) — Deep dive into engines, request flows, and infrastructure.
- [Advanced Port Configuration](docs/advanced-port-configuration.md) — How to manually adjust and decouple DNS/Proxy ports.
- [Product Requirements](docs/PRD.md) — Goals, competitive landscape, and implementation roadmap.
- [Architectural Decision Records](docs/adr/README.md) — The *"why"* behind our technical choices.

## 🤝 Contributing

We welcome contributions! Before submitting a PR:
1. Read our [Engineering Standards](docs/engineering-standards.md) (the code quality bar).
2. Follow the [Contributing Guide](CONTRIBUTING.md).
3. Run `make test` locally to ensure CI will pass.

## 📄 License
DevTether is licensed under the AGPL-3.0 License. See [LICENSE](LICENSE).
