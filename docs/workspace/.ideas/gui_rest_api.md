# Stateless Web GUI & IPC REST API Refactor

**Status:** Deferred to a future release (Engine 2 or later).

## Context & Vision
Currently, the DevTether daemon exposes basic IPC endpoints (`/status`, `/shutdown`) via HTTP over a Unix socket, and the CLI acts as a simple client. However, as DevTether grows, users will need a richer interface to manage routes dynamically without restarting the daemon.

We envision a **Stateless Web GUI** served directly from the daemon on a special domain like `devtether.localhost`. It will be built using Vanilla JS and embedded into the Go binary using `//go:embed`.

### Prerequisite: IPC REST API Refactor
Before building the GUI, the IPC layer must be refactored into a standard REST API.
- **Current:** Ad-hoc endpoints (`/status`, `/shutdown`).
- **Target:** `/api/*` prefix.
  - `GET /api/status`
  - `GET /api/config/routes`
  - `POST /api/config/routes`
  - `DELETE /api/config/routes/{domain}`

### Security Considerations (ADR-009)
The Web GUI will be subject to strict security constraints (ADR-009 §5). Exposing a management GUI on `devtether.localhost` introduces CSRF and DNS Rebinding risks.
- **Origin Validation:** The API must strictly validate the `Origin` and `Host` headers to ensure requests only come from the explicit Web GUI domain.
- **IPC Nonce:** The existing IPC Session Nonce (negotiated at startup) must be integrated into the GUI session for authentication.

## Why Deferred?
Per the "No AI Slop / Engine 1 Focus" mandate (`docs/engineering-standards.md`), we are intentionally limiting the scope of the current release to the core networking layer (Engine 1). The Web GUI and REST API represent a significant architectural leap and will be executed only when Engine 1 is battle-tested.
