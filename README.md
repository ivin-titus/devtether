# DevTether

> [!WARNING]
> **Project Status: Beta**
> DevTether is currently in Beta. Phase 1 (Static Routing) is complete and its configuration schema, CLI API, and core architecture are considered stable. Breaking changes to these will only occur with strong consensus and will be explicitly logged in the CHANGELOG. Note that Engines 2, 3, and 4 are still pending implementation.

DevTether is a modular, self-hosted developer networking toolkit for Linux environments. It replaces port memorization, reverse proxy configs, and ngrok subscriptions with clean named domains — all from a single Go binary.

## Architecture Overview

DevTether is built as **4 independent engines** inside a single binary:

- **Engine 1: Static Routing** — Maps pre-existing services on fixed ports to named domains. Zero process management.
- **Engine 2: Orchestration** — Spawns processes, injects dynamic `$PORT`, manages process trees.
- **Engine 3: Tunneling** — LAN sharing via mDNS + self-hosted WAN tunneling via relay.
- **Engine 4: Access Control** — Token-based RBAC at the proxy layer.

Shared infrastructure:
- **DNS Engine**: Intercepts UDP/53 queries for configured TLDs and resolves them locally.
- **Reverse Proxy**: Routes HTTP traffic based on `Host` header to the correct backend.
- **IPC Daemon**: Unix Domain Socket API for hot-reloading routes without restart.

*For a detailed sequence diagram, see the [Architecture Docs](docs/architecture.md).*

## Requirements
- Go 1.21+
- Linux (for capability support)

## Installation & Setup

Build the project and make it executable globally:

```bash
git clone https://github.com/ivin-titus/devtether.git
cd devtether
go build -o devtether ./cmd/devtether
sudo mv devtether /usr/local/bin/
```

### Network Capabilities (`setcap`)

DevTether requires elevated permissions to bind to Port 80 and Port 53. To avoid running the daemon as root, explicitly grant the binary `cap_net_bind_service`:

```bash
sudo setcap cap_net_bind_service=+ep /usr/local/bin/devtether
```
> If capabilities are not assigned, or if the port is already in use, DevTether will gracefully fall back to port `8080`. If that is also occupied, it will bind to an OS-assigned ephemeral port (Port 0). The same fallback logic applies to the DNS engine (`53` → `5353` → `Non-Fatal`).

### DNS Configuration

To unconditionally route `*.localhost` or `*.internal` to DevTether, configure your system's resolver (e.g., `systemd-resolved`):

```bash
sudo mkdir -p /etc/systemd/resolved.conf.d/
echo -e "[Resolve]\nDNS=127.0.0.1:53\nDomains=~internal ~localhost" | sudo tee /etc/systemd/resolved.conf.d/devtether.conf
sudo systemctl restart systemd-resolved
```

## Usage

### 1. Static Routing (`devtether.yaml`)

Map your already-running services to clean domains:

```yaml
routes:
  portfolio.localhost: 3222
  job-flow.localhost: 3223
  api.job-flow.localhost: 8042
```

### 2. Orchestrated Services

Let DevTether manage process lifecycles and port allocation:

```yaml
orchestrate:
  api:
    domain: api.localhost
    command: uvicorn main:app --port $PORT
    cwd: ./services/api
```

### 3. Execution

Start the routing daemon:

```bash
devtether start
```

### 4. IPC Operations

Add, view, or remove services dynamically over the Unix socket via secondary terminal windows:

```bash
devtether list
devtether add grafana.localhost "npm run start:ui"
devtether remove grafana.localhost
```

## Documentation

- [Product Requirements](docs/PRD.md)
- [Architecture Overview](docs/architecture.md)
- [Architectural Decision Records](docs/adr/README.md)

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md) for branch naming conventions, workflow architecture, and testing guidelines. Code behavior policies are found in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## License
DevTether is licensed under the AGPL-3.0 License. See [LICENSE](LICENSE) for the full text.
