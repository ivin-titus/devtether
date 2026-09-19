# v2.0.0-beta.7 (Phase 1 & Beyond) - Discussion Space
*(This document is append-only. It hosts ongoing brainstorms for multiple issues within a release cycle.)*

*Note: Decisions 1 through 17 pertained to the v2.0.0-beta.2 release (CLI UX, Bug Fixes, CI Hardening) and have been fully implemented and merged into the main repository. They have been removed from this document to reduce noise.*

---

> **Note:** All discussions regarding the OS Service module and Phase 3 integrations have been completely deferred to future releases due to complexities. See `.ideas/service_mode/artifacts/discussion_space.md` for historical context.

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

---

## Discussion: Phase 1 Pre-Mortem Anomalies (Resolved)

1. **`settings.verbose` vs `settings.log_level` Overlap**
   - **Resolution:** Deferred. Ship only `daemon`, `verbose`, and `log_path`. `log_level` is YAGNI. (No ADR logged yet; will be formalized when fine-grained logging is needed).
2. **`settings.dns.fallback_port` Duplicates Top-Level `dns:` Key**
   - **Resolution:** Keep DNS config strictly under the top-level `dns:` key (DRY violation avoided).
3. **Logger Writes to `os.Stdout` (Not `os.Stderr`)**
   - **Resolution:** Properly deferred to a future refactor to split structured output from log output.

---

## Discussion: Documentation Debt from Phase 1 (Daemon Mode)

- **Issue:** Phase 1 shipped `-d` mode, but documentation (help text, README, `init` template) lagged behind.
- **Resolution:** Fixed in a single batch commit before beta.7 release (Audit findings 4-10).

---

## Discussion: Where Does `devtether down` Belong?

- **Resolution:** Implemented in Subphase 2.1 alongside `status`. Both share the same IPC plumbing (`POST /shutdown`).

---

## Discussion: `sudo ./devtether up -d` Creates an Unreachable Daemon (Finding 11)

- **Issue:** `sudo` leaked root-owned sockets into the user's `$XDG_RUNTIME_DIR`.
- **Resolution:** Fixed in Subphase 2.7/2.9. The daemon now securely isolates to `/tmp/devtether-0` when run as root, safely allowing macOS users to bind port 80 via `sudo`.

---

## Discussion: `devtether doctor` UX Improvements (Findings 13, 16)

- **Resolution:** Introduced a `⚠` (warning) tier for non-fatal states (e.g. stale sockets, port 80 unavailable). Suppressed irrelevant Linux `setcap` warnings on macOS. (Implemented in Subphase 2.4).

---

## Discussion: `devtether status` Memory Label (Finding 12)

- **Resolution:** Renamed "Memory" to "Heap" to accurately reflect `runtime.MemStats.Alloc`. (Implemented in Subphase 2.4).

---

## Discussion: `devtether logs -f` Follow Flag (Finding 14)

- **Resolution:** Added `-f`/`--follow` as an explicit flag (alias for `--lines 0`). (Implemented in Subphase 2.4).

---

## Discussion: Should Foreground Mode Write to the Log File?

- **Decision:** No. Foreground mode outputs to the terminal. `devtether logs` strictly tails daemon-mode logs (`.logs/devtether.log`). (Implemented in Subphase 2.4).

---

## Design Note: Doctor Setcap + Sudo Messaging

- **Resolution:** `doctor` passive diagnostics print the raw `setcap` command; `init` active wizard applies it. (Implemented in Subphase 3.3).

---

## Discussion: Daemon Lifecycle UX Confusions & Fixes (Post-Phase 2)

Critical lifecycle race conditions were resolved by replacing aggressive polling with event-driven kernel primitives:
- **`logs -f` Zombie Trap:** Fixed via blocking `flock(LOCK_EX)` instead of polling.
- **`down` 100ms Race & Ghost TCP Binds:** Fixed by holding the IPC HTTP connection open until the server fully shuts down (EOF).
- **Silent Fallbacks:** Fixed by using an anonymous pipe to pass a JSON payload from the detached child to the parent.

---

## Discussion: Subphase 3.3 Remediation Design Decisions

The second-opinion audit (`audit_report_cline.md`) enforced the following fixes:
- **Exit-Code Discipline:** Replaced `os.Exit(1)` inside libraries with typed errors returned to Cobra. (Watchdog is an authorized exception).
- **Proxy Timeouts:** Enforced `ReadHeaderTimeout` and `IdleTimeout`. (Authorized exemption: strict `Read/Write` timeouts are banned to protect ADR-008 streaming).
- **Service Manager:** Hardcoded `-d` flags and `DEVTETHER_FOREGROUND=1` beat `settings.daemon` to prevent unsupervised orphans.
- **Readiness Ordering:** IPC listener binds synchronously *before* readiness signals.
- **PID File Fallback:** `down` uses the PID file + `SIGTERM` if the socket is permission-denied.
- **`fsnotify` Dependency:** Authorized as a 100% pure-Go standard to replace heavy polling loops.

---

## Discussion: TLD Defaults and Single-Label Domains

- **Resolution:** Simplified to strictly use `.localhost`. `.internal` is reserved for future Engine 3 LAN sharing (partially documented in ADR 002). `validateRoutes()` explicitly enforces `.localhost` validation to fail-fast.

---

## Discussion: False-Positive Root Daemon Detection

- **Resolution:** `RootDaemonMayBeRunning()` updated to dial the socket or check `kill -0` instead of blindly returning true on `os.ErrPermission`. `devtether down` now proactively cleans up orphaned sockets (`ECONNREFUSED`).

---

## Discussion: Resolving DX-1 (Deleting the `examples/` Directory)

- **Resolution:** Deleted `examples/` directory to prevent schema drift. `devtether init` now serves as the single source of truth by generating a rich, commented `devtether.yaml` template.

---

## Discussion: Route-Reload Semantics (DX-8)

- **Resolution:** Documentation updated to state that route changes require `devtether down && devtether up`. Actual IPC hot-reload deferred to the REST API refactor.

---

## Discussion: Subphase 3.6 UX & Uninstallation Feedback (Manual Sandbox)

Manual user testing revealed a few DX oversights:
1. **DNS Auto-Configuration:** `devtether init` correctly applies `setcap` on Linux, but stops short of configuring DNS. It should smartly detect the OS (macOS vs systemd-resolved vs DNSMasq) and prompt the user to apply the corresponding DNS resolver setup. This will significantly simplify the installation section in `README.md`.
2. **Help Text Duplication (Cobra Anti-Patterns):** 
   - The `devtether --help` command prints a manually maintained cheat-sheet (in `rootCmd.Long`) *and* the auto-generated Cobra `Available Commands` list. This is overwhelming and violates DRY. We must delete the manual cheat-sheet and rely entirely on Cobra's native generation.
   - Similarly, `initCmd.Long` manually lists its flags, which Cobra then immediately prints again underneath. We must delete the manual flag documentation in `Long`.
3. **Help Clutter:** Cobra auto-generates a `completion` command that appears in the `devtether --help` output. This is noise for a normal user and must be hidden (`CompletionOptions.DisableDefaultCmd = true`).
4. **Uninstall Completeness:** `scripts/uninstall.sh` successfully removes the binary. Since `setcap` is an extended file attribute, deleting the binary natively destroys the capability (no explicit reversal is needed). However, once the `init` wizard is upgraded to configure system-level DNS (Point 1), `uninstall.sh` must be expanded to cleanly reverse those DNS integrations.
5. **Strict TLD Enforcement (QA-2 Verified):** Manual testing with `_devtether.yaml` successfully loaded the `job-flow.internal` route. This actively reproduced the `QA-2` finding (missing `.localhost` enforcement in `validateRoutes`). We must implement a strict regex check (`^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)*\.localhost$`) to fail-fast on unsupported domains, block wildcards, and enforce valid alphanumeric characters.

---

## Phase 4.5 Orchestration & CLI Architecture Discussions

We discovered several complex UX/CLI orchestration bugs (DX-23 through DX-30) that require architectural decisions. Let's discuss the proposed solutions:

### 1. Log Tailing Memory Spike (DX-27)
**The Problem**: Tailing a huge log file backwards via `readBackwardChunk` uses `buf = append(chunk, buf...)`, resulting in an $O(N^2)$ memory leak.
**Decision (Industry Standard GNU `tail` algorithm)**: We will allocate a single, static 4KB buffer and traverse backwards purely to count newline characters (`\n`). Once we find the target line count, we record the byte offset, use `f.Seek()`, and stream the file directly to standard output. This keeps memory usage at a flat ~4KB ($O(1)$) regardless of the number of lines requested.

### 2. File Descriptor Desync on Log Rotation (DX-28)
**The Problem**: When `logs -f` is running and the daemon is restarted, `daemonize()` currently truncates the file (`os.O_TRUNC`). The active `logs -f` file descriptor is left pointing beyond the new EOF, causing it to spin infinitely.
**Decision (Seamless GNU `tail -F` Emulation)**: Before blocking on `fsnotify`, we will compare `f.Stat().Size()` against our current byte offset. If the size shrinks, we immediately know the log was rotated (truncated). We seamlessly `f.Seek(0, io.SeekStart)` and continue tailing, printing a `--- Log Truncated / Daemon Restarted ---` banner.

### 3. `fsnotify` Event Dropping Race Condition (DX-29)
**The Problem**: In `logs.go`, `fsnotify.Events` are read in a blocking `select` only *after* `f.Read()` returns `EOF`. Events fired *during* the read block are buffered and can be dropped by `fsnotify` if the channel fills up.
**Decision (Decoupled Producer/Consumer)**: We will spawn a dedicated background goroutine whose *only* job is to instantly drain `watcher.Events` in an infinite loop. It will forward a non-blocking trigger signal to the main file reader. This guarantees `fsnotify` is never blocked by file I/O, entirely eliminating the risk of dropped events or OS-level buffer overflows.

### 4. DNS Wizard Hardcodes Port 53 (DX-30)
**The Problem**: The `devtether init` wizard hardcodes `:53` into OS configs. If DevTether falls back to `5353` (due to missing `setcap`), resolution breaks.
**Decision (Dynamic Port Injection)**: The wizard will dynamically calculate if port 53 is inaccessible based on the user's answers (e.g., if they decline `setcap`) and automatically provision the OS configuration with `127.0.0.1:5353` instead, or parse `cfg.DNS.Bind` directly if configuring an existing setup.

---

## Phase 4.6 Architectural Check & Remediation Discussions

The final independent audit discovered four architectural/DX regressions that must be addressed before the release.

### 1. `WaitForExit` Polling Battery Drain (Medium)
**The Problem**: In `logs.go`, the detached `logs -f` uses a `50ms` loop to poll the internal `flock` lock file to detect when the daemon shuts down. This wakes the CPU 20 times a second, which drains battery and violates our anti-polling standard.
**Decision (Event-Driven Fallback)**: We should remove the tight `50ms` polling loop.
*Option A*: Use `fsnotify` to watch the `.pid` or `.lock` file for deletion events.
*Option B*: Simply back off the polling interval to `1000ms`. Since exiting a log tail isn't latency-critical, a 1-second delay is perfectly acceptable and drops CPU wakes by 95%.
*Recommendation:* Option B is the "Lazy Senior Dev" approach. No new `fsnotify` watchers needed for a simple exit check.

### 2. Incomplete Fix for DX-30 (High)
**The Problem**: The `devtether init` wizard configures `systemd-resolved` based on the port active *at that exact moment* (e.g. `53`). But if a user later runs `devtether up` without `setcap`, the proxy dynamically falls back to `5353`, leaving `systemd-resolved` pointing to a dead port. This breaks all `.localhost` routing. Furthermore, the `5353` fallback port is notoriously problematic because it is the standard mDNS port (often blocked by `avahi-daemon` on Linux and `mDNSResponder` on macOS).
**Decision (Fail-Fast Architecture & Safe Unprivileged Port)**: 
1. **No Silent Fallbacks:** Remove the silent fallback logic in `internal/dns/server.go`. If `devtether.yaml` configures port 53 and it fails (EACCES or EADDRINUSE), the daemon crashes instantly and cleanly tells the user how to fix it (e.g., use `sudo`, apply `setcap`, or edit the config). This ensures the daemon and the OS config are never out of sync.
2. **Safe Unprivileged Default:** Update the `init` wizard so that if the user declines `setcap` (or is on an OS without it), it defaults the DNS port to `5335` rather than the conflict-prone `5353`.

### 3. Route Regex Relaxation (Low)
**The Problem**: The domain validation regex permits trailing hyphens (e.g. `app-.localhost`), violating RFC 1035.
**Decision**: Tighten the regex in `validateRoutes()` to strictly enforce alphanumeric boundaries for labels: `^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*\.localhost$`

### 4. Daemonized Health Check Polling (Low)
**The Problem**: The `up -d` daemon starts a 1.5-second polling loop that repeatedly `net.Dial`s every backend route forever to check if it's online/offline. This generates unnecessary CPU load and network noise for a detached service.
**Decision (Deferred)**: Refactoring `devtether routes` to actively ping services on-demand was deemed too risky for the stabilization phase of `beta.7`. This has been deferred to a future release (see `docs/workspace/.ideas/on_demand_health_checks.md`). As an interim mitigation, the polling loop delay was increased from 1.5 seconds to 3.0 seconds to reduce background CPU wakeups.
