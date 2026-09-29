# <img src=".assets/devtether_logo.png" height="48" align="center" alt="DevTether Logo" /> DevTether

> **Single binary. Named domains. Minimal setup.**

DevTether is a self-hosted local development networking toolkit written in Go. It maps existing local HTTP services to named `.localhost` domains through embedded DNS and a lightweight reverse proxy, with a CLI daemon for managing the local runtime.

> [!WARNING]
> **Project Status: Beta**  
> v2.0.0-beta.7 currently ships **Engine 1: Local Static Routing**. The remaining engines and sharing features are planned and are not part of the current beta.

## ⚡ Quick Start

```bash
# 1. Install (see Installation below)
# 2. Create a starter configuration and optionally configure *.localhost
devtether init

# 3. Add routes to devtether.yaml
#    routes:
#      web.localhost: 3222
#      api.localhost: 8042

# 4. Start routing
devtether up
```

Route changes currently require a restart:

```bash
devtether down
devtether up
```

## 🛠️ Installation

### Option 1: Quick Install

```bash
curl -sSfL https://raw.githubusercontent.com/ivin-titus/devtether/main/scripts/install.sh | sh
```

The script detects your OS and architecture, downloads the latest release, and verifies its checksum.

### Option 2: Download Binary

Download the latest release for your platform from [GitHub Releases](https://github.com/ivin-titus/devtether/releases), extract it, and move it to your `$PATH` (for example, `~/.local/bin/`).

### Option 3: Build from Source

Requires Go 1.27.1+.

```bash
git clone https://github.com/ivin-titus/devtether.git
cd devtether
make build && make install
```

## 🪄 Setup: `devtether init`

Run:

```bash
devtether init
```

The interactive wizard can:

1. create a valid `devtether.yaml` using the current Engine 1 schema;
2. ask whether to enable detached daemon mode;
3. on Linux, optionally apply `setcap` for privileged proxy-port binding;
4. on supported Linux/macOS resolver setups, optionally configure the host resolver so `*.localhost` can resolve through DevTether.

System-level changes are explicit and require user consent. On non-TTY input, the wizard skips interactive system changes and uses safe defaults.

For manual resolver and port changes, see [Advanced Port Configuration](docs/advanced-port-configuration.md).

## 💻 Current Usage

### Current configuration

```yaml
routes:
  web.localhost: 3222
  api.localhost: 8042

# Optional:
# settings:
#   daemon: true
#   verbose: false
#   log_path: "./.logs"

# proxy:
#   port: 80
#   timeouts:
#     idle: 120s

# dns:
#   bind: "127.0.0.1:5335"
```

The current configuration supports:

- `routes` for static HTTP routes;
- `settings.daemon`, `settings.verbose`, and `settings.log_path`;
- `proxy.port` and `proxy.timeouts.idle`;
- `dns.bind`, defaulting to `127.0.0.1:5335`.

The configuration loader rejects unsupported future sections such as `orchestrate:`, `tunnel:`, and `access:`.

### Core Commands

```text
devtether init
devtether up
devtether up -d
devtether down
devtether status
devtether routes
devtether logs
devtether logs -f
devtether doctor
devtether version
```

Foreground mode writes logs to the terminal. Detached mode writes to `devtether.log` under the configured log directory.

### Proxy port behavior

The proxy tries the configured port first. In beta.7, if that bind fails because of permission or address-in-use conditions, it falls back to `127.0.0.1:8080`. There is no OS-assigned `:0` fallback. Both configured and fallback listeners remain loopback-only.

### DNS behavior

The embedded DNS server defaults to `127.0.0.1:5335`, answers only for active `.localhost` routes, and never forwards unrelated queries upstream. An existing route with an unsupported record type returns NODATA; an unknown route returns NXDOMAIN.

## 🗺️ Architecture & Roadmap

DevTether is built around four modular engines:

| Engine | Purpose | Status |
|---|---|---|
| **Engine 1 — Local Static Routing** | Embedded DNS, static route table, HTTP reverse proxy | ✅ Current beta |
| **Engine 2 — Orchestration** | Process groups, lifecycle management, dynamic `$PORT` allocation | 🔲 Planned |
| **Engine 3 — Tunneling** | LAN sharing and self-hosted WAN relay | 🔲 Planned |
| **Engine 4 — Access Control** | Scoped tokens, RBAC and service access policy | 🔲 Planned |

The four engines may later be grouped into three conceptual user-facing layers. Engine 3 and Engine 4 are both future sharing/access capabilities. See [ADR-001](docs/adr/001-modular-engine-architecture.md).

### Platform Support

Linux and macOS are supported for current binary releases on amd64 and arm64. Linux is the primary development/test environment; macOS is shipped and exercised in CI but has less platform-specific coverage. Native Windows support is not currently provided.

## 📚 Documentation

- [Architecture Overview](docs/architecture.md) — current beta architecture and future engine boundaries.
- [Advanced Port Configuration](docs/advanced-port-configuration.md) — proxy/DNS port customization and resolver alignment.
- [Product Requirements](docs/PRD.md) — product goals, current scope, and target architecture.
- [Architectural Decision Records](docs/adr/README.md) — the decisions behind the architecture.
- [Engineering Standards](docs/engineering-standards.md) — mandatory coding and review standards.
- [Contributing Guide](CONTRIBUTING.md) — development and pull-request workflow.

## 🤝 Contributing

Before opening a PR:

1. Read the [Engineering Standards](docs/engineering-standards.md).
2. Read [CONTRIBUTING.md](CONTRIBUTING.md).
3. Run `make test` locally.
4. Remember that CI is the final release gate and runs tests on both Linux and macOS.

## 📄 License

DevTether is licensed under the AGPL-3.0 License. See [LICENSE](LICENSE).
