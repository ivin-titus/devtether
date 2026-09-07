# DevTether

> **Single binary. Named domains. Zero hassle.**

DevTether is a modular, self-hosted developer networking toolkit for Unix environments (Linux, macOS). It replaces port memorization, reverse proxy configs, and ngrok subscriptions with clean named domains — all from a single Go binary.

```text
Before:                          After:
localhost:3222                   web.localhost
localhost:3223                   api.localhost
localhost:8042                   db.localhost
```

> [!WARNING]
> **Project Status: Beta**
> DevTether is currently in Beta. Engine 1 (Static Routing) is complete — its configuration schema, CLI API, and core architecture are stable. Breaking changes to these will only occur with strong consensus and will be logged in the [CHANGELOG](CHANGELOG.md). Engines 2–4 are pending implementation.

## Quick Start

```bash
# 1. Install (see Installation below)
# 2. Create a config file
devtether init

# 3. Edit your routes
#    routes:
#      web.localhost: 3222
#      api.localhost: 8042

# 4. Start routing
devtether up
```

## Installation

### Option 1: Quick Install (Recommended)

```bash
curl -sSfL https://raw.githubusercontent.com/ivin-titus/devtether/main/scripts/install.sh | sh
```

The script auto-detects your OS and architecture, downloads the latest release, and verifies the checksum.

### Option 2: Download Binary

Download the latest release for your platform from [GitHub Releases](https://github.com/ivin-titus/devtether/releases):

```bash
# Example for Linux amd64 (replace version with latest)
tar -xzf devtether_2.0.0-beta.2_linux_amd64.tar.gz
chmod +x devtether
mkdir -p ~/.local/bin
mv devtether ~/.local/bin/
```

### Option 3: Build from Source

Requires Go 1.27.1+.

```bash
git clone https://github.com/ivin-titus/devtether.git
cd devtether
make build
make install
```

### Post-Install Setup

Depending on your operating system, there are a few final steps to configure DevTether for a seamless experience.

<details open>
<summary><b>🐧 Linux Setup</b></summary>

**1. Network Capabilities**

DevTether is secure-by-default and strictly binds to loopback (`127.0.0.1:80` and `127.0.0.1:53`) to prevent accidental LAN exposure. To avoid running as `root` while binding these privileged ports, grant the binary capabilities:

```bash
sudo setcap cap_net_bind_service=+ep ~/.local/bin/devtether
```
> *If you skip this, DevTether gracefully falls back to unprivileged ports (8080 for HTTP, 5353 for DNS).*

**2. DNS Configuration**

Configure your system resolver to route `*.localhost` to DevTether.

**systemd-resolved (Ubuntu, Fedora, Arch Linux):**
```bash
sudo mkdir -p /etc/systemd/resolved.conf.d/
echo -e "[Resolve]\nDNS=127.0.0.1:53\nDomains=~internal ~localhost" | sudo tee /etc/systemd/resolved.conf.d/devtether.conf
sudo systemctl restart systemd-resolved
```

**dnsmasq (Non-systemd / Alpine):**
```bash
echo "server=/localhost/127.0.0.1#53" | sudo tee /etc/dnsmasq.d/devtether.conf
sudo systemctl restart dnsmasq
```
</details>

<details open>
<summary><b>🍎 macOS Setup</b></summary>

**1. Port Binding**

macOS does not support capabilities like Linux. To use the privileged loopback ports (`127.0.0.1:80` and `127.0.0.1:53`), run `devtether` with `sudo`, or simply let it fall back to the unprivileged ports (`8080` and `5353`).

**2. DNS Configuration**

macOS has native support for domain-specific resolvers via `/etc/resolver/`:
```bash
sudo mkdir -p /etc/resolver
echo "nameserver 127.0.0.1" | sudo tee /etc/resolver/localhost
echo "nameserver 127.0.0.1" | sudo tee /etc/resolver/internal
```
</details>

## Usage

### Configuration (`devtether.yaml`)

Map your already-running services to clean domains:

```yaml
routes:
  web.localhost: 3222
  api.localhost: 3223
  db.localhost: 8042
```

See [examples/](examples/) for more configuration patterns, including proxy timeouts and DNS settings.

### Commands

```bash
devtether up                        # Start the routing daemon
devtether up -c /path/to/config     # Use a specific config file
devtether routes                    # Show active routes (live from daemon, or from config)
devtether init                      # Create a starter devtether.yaml
devtether version                   # Print version, commit, and build date
```

## Platform Support

| Platform | Status |
|----------|--------|
| Linux (amd64, arm64) | ✅ Fully supported |
| macOS (amd64, arm64) | ✅ Supported |
| Windows | 🔲 Not yet supported ([ADR-006](docs/adr/006-platform-support-and-cgo-policy.md)) |

## Architecture

DevTether is evolving from a simple reverse proxy into a comprehensive Developer Platform, structured into **Three Main Layers** inside a single Go binary. *(Internally, these layers are powered by **4 independent modular engines** — see [ADR-001](docs/adr/001-modular-engine-architecture.md)).*

| Layer | Purpose | Status |
|-------|---------|--------|
| **1. Networking Layer** | Reverse Proxy, DNS, Smart CORS, Traffic Inspection, IP Cycling | ✅ Foundation Complete |
| **2. Process Orchestrator** | Process Groups (PGID), ephemeral ports, unified logging | 🔲 Planned |
| **3. Access Controls** | Centralized IAM, RBAC, cross-network collaboration tokens | 🔲 Planned |

DevTether provides a **Unified Interface**: both the CLI and the stateless, lazy-loaded Web GUI (`devtether.localhost`) communicate via the exact same internal IPC Daemon API. What you can do in the GUI, you can do in the CLI.

*For detailed architecture, sequence diagrams, and the request flow, see [docs/architecture.md](docs/architecture.md).*

## Documentation

- [Architecture Overview](docs/architecture.md) — Deep dive into engines, request flows, and infrastructure
- [Product Requirements](docs/PRD.md) — Goals, competitive landscape, and implementation roadmap
- [Architectural Decision Records](docs/adr/README.md) — Why things are built the way they are

## Contributing

We welcome contributions! Before submitting a PR:

1. Read our [Engineering Standards](docs/engineering-standards.md) — the code quality bar for all contributions
2. Follow the [Contributing Guide](CONTRIBUTING.md) — setup, workflow, and PR checklist
3. Run `./scripts/test.sh` before pushing — if it passes locally, CI will pass

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community behavior policies.

## License

DevTether is licensed under the AGPL-3.0 License. See [LICENSE](LICENSE) for the full text.
