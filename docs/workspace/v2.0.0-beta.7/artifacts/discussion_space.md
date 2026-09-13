# v2.0.0-beta.7 (Phase 1 & Beyond) - Discussion Space
*(This document is append-only. It hosts ongoing brainstorms for multiple issues within a release cycle.)*

*Note: Decisions 1 through 17 pertained to the v2.0.0-beta.2 release (CLI UX, Bug Fixes, CI Hardening) and have been fully implemented and merged into the main repository. They have been removed from this document to reduce noise.*

---

## Discussion: The Unified CLI/GUI Interface (Deferred)

### 1. Lazy-Loaded, Zero-Bloat GUI
The `devtether.localhost` dashboard is **not** a heavy background process. By default, the GUI process layer is *not even started*, saving memory. It is purely lazy-loaded. If a user modifies the config while running, the service layer gracefully stops/reloads specific routes without tearing down the entire daemon.

### 2. Shared Call Interface (DRY / SoC)
The GUI is entirely stateless. Both the CLI (e.g., `devtether stop api`) and the Web GUI call the exact same **IPC Daemon REST API**. 
- The IPC Daemon is the sole mutator of the `devtether.yaml` config file. 
- This guarantees zero DRY or SoC violations (`docs/engineering-standards.md`). Everything possible in the GUI is equally possible via CLI.

### 3. Deep System Visibility (`devtether status`)
The `status` (or `ps`) command will provide a robust, unified table showing:
- IPC Daemon health.
- Static network routes (Layer 1).
- Orchestrated processes with PID, CPU, and Memory usage (Layer 2).
- Active LAN/WAN sharing tunnels and RBAC locks (Layer 3).

### 4. System-Level Startup (Daemon Wake)
To match XAMPP, Nginx, or Cloudflared, DevTether includes a `service install` command. It auto-generates the correct daemon file (`systemd` for standard Linux, `OpenRC/runit` for Arch/Alpine, `launchd` for macOS), allowing it to wake silently on system boot.

### Resolution
GUI and full IPC REST API are explicitly deferred to a post-daemon release. `devtether status` and `service install` are scoped into beta.7. See `implementation_plan.md` Phases 2.1 and 3.2.

---

## Discussion: Phase 1 System Integration & IPC (Engine 1 Focus)

Phase 1 focuses on elevating Engine 1 (Core Networking) into a true system-level utility via a detached daemon and an IPC API. We are intentionally deferring Engine 2 (Process Orchestrator) until a concrete need arises, adhering strictly to the *No Overengineering* mandate (`docs/engineering-standards.md`).

### 1. The Tri-Layered Daemon Architecture
Running `devtether up` currently blocks the terminal. To achieve a seamless developer experience, DevTether must support background execution via a single, unified IPC foundation across three paradigms:

1. **Flag-based (`-d`):** `devtether up -d` spawns a new independent child process of itself using `exec.Command` with `SysProcAttr` set to detach the PGID, then the CLI parent exits immediately.
2. **Config-based (`daemon: true`):** Setting `daemon: true` in `devtether.yaml` ensures that a standard `devtether up` automatically triggers the detached behavior without needing the flag.
3. **OS-Native Services:** True system-level daemons managed by `systemd` (Linux), `launchd` (macOS), or `OpenRC/runit` (Alpine/Arch).

#### Privilege Escalation & Port 80 (Daemon mode)
When running as a true OS-Native daemon, the service manager can easily grant `CAP_NET_BIND_SERVICE` (or run as a privileged user). This guarantees DevTether can bind to port `80` and `443`, allowing users to type `portfolio.localhost` in the browser instead of `portfolio.localhost:8080`.
- **Conflict Mitigation:** The daemon must intelligently detect if port 80 is already hijacked by legacy stacks (XAMPP, Apache, Nginx). It should gracefully fallback and clearly warn the user to stop the conflicting service.

### 2. Log Routing & Storage
When running detached, logs need a predictable, scalable home.
- **Default Path:** Logs will default to a `.logs/` folder located in the exact same directory as the active `devtether.yaml` config file.
- **Configurable Path:** The log output directory can be explicitly overridden in the global config.
- **System Daemon Path:** If running as an installed system service, logs will automatically route to standard OS log paths (e.g., `/var/log/devtether/` or `journalctl`).

### 3. CLI Command Surface (Status vs. Doctor SoC)
To preserve Separation of Concerns (SoC), we must strictly separate passive reporting from active diagnostics:

- **`devtether status` (Passive Reporting):** Its only job is to query the IPC socket (`/status`) and display current state (uptime, memory usage, active routes, PID). It does *not* test the network.
- **`devtether doctor` (Active Diagnostics):** A proactive environment scanner. It tests for XAMPP/Nginx conflicts on port 80, validates `setcap` privileges, pings upstream DNS servers for timeouts, checks for stale `.sock` files, and validates `devtether.yaml` syntax.
- **`devtether logs`**: Tails the daemon's log streams.

### 4. Centralized Engine Configuration (`devtether.yaml`)
To prevent polluting the root namespace of the YAML file, we will introduce a `settings:` (or `devtether:`) block. This ensures high configurability for the core engine, applying whether you run detached (`up -d`) or block the terminal (`up`).
- **Centralized Defaults:** All default configurations must be defined in a single, centralized Go package (`internal/config`).
- **Example Schema:**
  ```yaml
  settings:
    daemon: true              # Run in background by default
    verbose: true             # Enables debug-level internal logging
    log_level: "info"         # Standard log level
    log_path: "./.logs"       # Custom log directory
    dns:
      fallback_port: 5353     
  routes:
    portfolio.localhost: 3222
  ```

### 5. Deferred Engine 2 (Process Supervisor)
*Note: The following requirements are documented for the future. Engine 2 is deferred until specifically requested.*
- **PGID Enforcement:** Orchestrated applications must be spawned in their own Process Group (PGID) to ensure `SIGTERM` cleans up entire process trees (e.g., `npm run dev` and its children).
- **Dynamic Port Injection:** The proxy would automatically bind an available port and inject it via the `$PORT` environment variable.

### Resolution
All items in sections 1–4 are scoped into beta.7 (see `implementation_plan.md` Phases 1–3). Engine 2 remains deferred.

---
## Discussion: DevTether's Future & Architectural Philosophy (Sprint Direction)

* **Engine 1 as the Foundation:** Do not rush into building Engine 2 or Engine 3 yet. Engine 1 is intentionally small because the current phase is about making the core network foundation extremely stable. Treat Engine 1 as infrastructure that future engines will depend on. Stability and clean boundaries now are more valuable than adding another engine quickly.
* **The Long-Term Vision:** DevTether is not just a reverse proxy. It will gradually become a **single-binary development networking ecosystem**, built brick-by-brick.
  1. **Engine 1:** Local networking (embedded DNS + routing + proxy).
  2. **Engine 3:** LAN/service sharing (reducing reliance on external tunneling tools).
  3. **Access Control (Engine 4):** Secure the sharing layer before exposing capabilities.
  4. **Engine 2:** Process orchestration and dynamic port injection.
  5. *Future Ecosystem Capabilities.*
* **Anti-Slop Architecture:** Do not implement future engines prematurely or create abstractions solely for hypothetical future requirements. Keep boundaries clean and add abstractions only when an actual engine needs them (No AI-slop). Small changes, explicit reasoning, and no accumulating complexity.
* **The Simplicity Mandate:** Ideally: download one binary → `devtether init` → `devtether up`. No Python service, separate DNS containers, or Redis stacks.
* **Intelligent `devtether init` (The Setup Wizard):** Similar to `create-next-app`, `init` should be an interactive setup wizard rather than just dumping a dummy `devtether.yaml` file. It should:
  1. Ask: *Do you want to setup DevTether as a system daemon or a standalone/portable binary?*
  2. Detect the OS and active DNS resolver (e.g., `systemd-resolved`), then guide the user to configure the DNS layer with smart defaults.
  3. Ask: *Apply `setcap` to allow DevTether to bind to ports 80/443 without root? (y/N)*, while explicitly stating the security consequences of this action.
  4. Inform the user of the final `devtether.yaml` location, mentioning they can edit it later and apply changes via `devtether restart`.
  5. Include a config validation phase (similar to `nginx -t`) to ensure the generated configuration is syntactically sound.
  *The core philosophy is Least Privilege by Default. Never make `init` a dangerous "configure everything silently" command. It must remain a secure, explicit local integration tool.*
* **Secure-by-Default:** Features that expand network exposure (LAN/WAN/tunnels) must remain explicit opt-ins. Convenience must never weaken security.
* **Diagnostics & Actionable Errors:** 
  - **`devtether doctor`:** Modeled after `flutter doctor`, this command will automatically scan the system for missing dependencies, port conflicts, DNS resolution failures, and privilege issues, outputting a clean checklist of the system's health.
  - **Actionable Errors:** CLI errors should follow the pattern: `what failed → why → what DevTether detected → what the user can do`.
* **The True Success Metric:** How little does a developer have to think about DevTether to get a working development network?

### Resolution
These principles are standing directives. They apply to all current and future sprints. No further action required — they are enforced via `docs/engineering-standards.md` and the ponytail skill.
