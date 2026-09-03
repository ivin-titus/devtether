# DevTether

> **Single binary. Named domains. Zero hassle.**

DevTether is a modular, self-hosted developer networking toolkit for Unix environments (Linux, macOS). It replaces port memorization, reverse proxy configs, and ngrok subscriptions with clean named domains — all from a single Go binary.

```text
Before:                          After:
localhost:3222                   portfolio.localhost
localhost:3223                   job-flow.localhost
localhost:8042                   api.job-flow.localhost
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
#      portfolio.localhost: 3222
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
sudo mv devtether /usr/local/bin/
```

### Option 3: Build from Source

Requires Go 1.21+.

```bash
git clone https://github.com/ivin-titus/devtether.git
cd devtether
go build -o devtether ./cmd/devtether
sudo mv devtether /usr/local/bin/
```

### Post-Install: Network Capabilities (Linux only)

DevTether binds to Port 80 and Port 53. To avoid running as root, grant the binary `cap_net_bind_service`:

```bash
sudo setcap cap_net_bind_service=+ep $(which devtether)
```

> If capabilities are not assigned, or if the port is already in use, DevTether gracefully falls back: `80 → 8080 → OS-assigned port` for the proxy, and `53 → 5353 → skip` for DNS.

### DNS Configuration

To route `*.localhost` or `*.internal` to DevTether, configure your system resolver:

```bash
# systemd-resolved (Ubuntu, Fedora, etc.)
sudo mkdir -p /etc/systemd/resolved.conf.d/
echo -e "[Resolve]\nDNS=127.0.0.1:53\nDomains=~internal ~localhost" | sudo tee /etc/systemd/resolved.conf.d/devtether.conf
sudo systemctl restart systemd-resolved
```

## Usage

### Configuration (`devtether.yaml`)

Map your already-running services to clean domains:

```yaml
routes:
  portfolio.localhost: 3222
  job-flow.localhost: 3223
  api.job-flow.localhost: 8042
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

DevTether is built as **4 independent engines** inside a single binary:

| Engine | Purpose | Status |
|--------|---------|--------|
| **Engine 1: Static Routing** | Maps services on fixed ports to named domains | ✅ Complete |
| **Engine 2: Orchestration** | Spawns processes, injects dynamic `$PORT` | 🔲 Phase 2 |
| **Engine 3: Tunneling** | LAN sharing via mDNS + self-hosted WAN tunneling | 🔲 Phase 3–4 |
| **Engine 4: Access Control** | Token-based RBAC at the proxy layer | 🔲 Phase 5 |

*For detailed architecture, sequence diagrams, and the request flow, see [docs/architecture.md](docs/architecture.md).*

## Documentation

- [Architecture Overview](docs/architecture.md) — Deep dive into engines, request flows, and infrastructure
- [Product Requirements](docs/PRD.md) — Goals, competitive landscape, and implementation phases
- [Architectural Decision Records](docs/adr/README.md) — Why things are built the way they are

## Contributing

We welcome contributions! Before submitting a PR:

1. Read our [Engineering Standards](docs/engineering-standards.md) — the code quality bar for all contributions
2. Follow the [Contributing Guide](CONTRIBUTING.md) — setup, workflow, and PR checklist
3. Run `./scripts/test.sh` before pushing — if it passes locally, CI will pass

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community behavior policies.

## License

DevTether is licensed under the AGPL-3.0 License. See [LICENSE](LICENSE) for the full text.
