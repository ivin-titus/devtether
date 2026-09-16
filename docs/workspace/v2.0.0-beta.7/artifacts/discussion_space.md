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

---

## Discussion: Phase 1 Pre-Mortem Anomalies

### 1. `settings.verbose` vs `settings.log_level` Overlap
The example schema in the discussion shows both `verbose: true` and `log_level: "info"`. These are contradictory — if verbose is true, the effective level is debug, not info. Having both fields creates ambiguity about which takes precedence.

**Resolution:** Ship only `daemon`, `verbose`, and `log_path` for now. `log_level` is YAGNI — the binary verbose/non-verbose toggle already serves the codebase. Add fine-grained levels later if a real use case demands it.

### 2. `settings.dns.fallback_port` Duplicates Top-Level `dns:` Key
The discussion's example schema nests `dns.fallback_port` under `settings:`. The top-level `dns:` key already owns DNS configuration (`tld`, `bind`). Duplicating DNS config under `settings:` is a DRY violation (`docs/engineering-standards.md`).

**Resolution:** Do not add DNS config under `settings:`. The existing top-level `dns:` key is the single owner of DNS configuration.

### 3. Logger Writes to `os.Stdout` (Not `os.Stderr`)
Both `outLog` and the `slog.TextHandler` in `internal/logger/logger.go` write to `os.Stdout`. Most daemon loggers use `os.Stderr` so stdout can carry structured output. When the daemon is forked and stdout is redirected to a log file, this works fine — but it means `fmt.Printf` startup UI output also goes to the log file.

**Resolution:** Acceptable for now. The existing `term.IsTerminal` check already strips ANSI codes when stdout is a file. A future refactor could split structured output (stdout) from log output (stderr), but that's not needed for beta.7.

---

## Discussion: Documentation Debt from Phase 1 (Daemon Mode)

Phase 1 shipped code for `-d` mode, `settings:` config, and log routing. But the user-facing documentation surface was not updated alongside the code. This creates a discoverability gap — the feature exists but users can't find it.

### 1. Affected Surfaces
- **Root help text** (`devtether --help`): Hardcoded cheat-sheet doesn't mention `up -d`.
- **`up` command Long description**: Says "Press Ctrl+C for graceful shutdown" but doesn't mention `-d` backgrounding or where logs go.
- **README.md Commands section**: Missing `devtether up -d`.
- **README.md download example**: Still says `beta.2` tarball.
- **`init` default template**: Generates a `devtether.yaml` with only `routes:` — doesn't show `settings:` as a configurable block. Users won't know `daemon`, `verbose`, or `log_path` exist.
- **`root_out.go`**: Empty placeholder file (2 lines, `package cli` only). Dead code.

### 2. Principle
Per `docs/engineering-standards.md`: "Documentation updated if CLI flags or behavior changed." We added a `-d` flag and changed behavior (daemon mode, log routing). Documentation must catch up.

### 3. Proposed Batch Fix
All of these can be fixed in a single focused commit — no architectural decisions needed:
1. Update `root.go` Long text to add `devtether up -d`.
2. Update `up.go` Long text to mention `-d` and log path.
3. Update `README.md` Commands section and download example.
4. Add `settings:` block (commented) to `init.go`'s default template.
5. Delete `root_out.go` if confirmed unused.

### Resolution
Findings logged as audit items 4–10 in `audit_report.md`. Fix deferred to a single documentation commit before beta.7 release. No code logic changes needed.

---

## Discussion: Where Does `devtether down` Belong?

Phase 1 shipped `devtether up -d` but there's no corresponding `devtether down` to gracefully stop the daemon. Users currently have to `kill <PID>` manually (the PID is printed on spawn).

### Why It Wasn't in Phase 1
Phase 1 was scoped to daemon *startup* infrastructure: config, forking, log routing. The daemon already handles `SIGTERM` gracefully via the existing signal handler in `runUp`, so `kill <PID>` works correctly. A dedicated `down` command is a UX convenience, not a correctness requirement.

### Where It Fits
**Phase 2 (Subphase 2.1)** is the natural home. Here's why:

1. **Same IPC plumbing.** `devtether status` (Subphase 2.1) already needs to dial the Unix socket and query the daemon. `down` uses the exact same client path — it just sends a shutdown request instead of a status query.
2. **Dependency.** `down` needs a `/shutdown` or `POST /down` IPC endpoint. This endpoint belongs alongside `/status` and `/routes` in `internal/daemon/daemon.go`.
3. **Minimal surface.** The implementation is ~20 lines: add a handler, create `internal/cli/down.go`, dial the socket, send the request. It's a natural sibling to `status.go`.

### Implementation Sketch
- Add `POST /shutdown` handler to the IPC daemon — calls `cancel()` on the root context, triggering the existing graceful shutdown path.
- Create `internal/cli/down.go` — dials the Unix socket, sends the request, prints confirmation.
- If daemon is not running, print a clear message and exit 0 (idempotent).

### Alternatives Considered
- **Phase 3 (service installer):** Too late. Users need `down` as soon as they have `up -d`, which is now.
- **Standalone subphase:** Overkill for ~20 lines. It slots cleanly into 2.1's IPC work.

### Resolution
Add `devtether down` as a step within Subphase 2.1 alongside `devtether status`. Both share the same IPC client infrastructure. Log in `implementation_plan.md` and `tracker.md` when Phase 2 work begins.

---

## Discussion: `sudo ./devtether up -d` Creates an Unreachable Daemon (Finding 11)

### Problem
When `sudo ./devtether up -d` runs, the child daemon inherits `root` UID but the user's `$XDG_RUNTIME_DIR`. The socket is created with `root:root 0600` at the *user's* runtime directory. The user can no longer `devtether down` (EACCES), and `sudo devtether down` only works if the sudo password is correct. If both fail, the daemon is orphaned.

### Root Cause
`SocketPath()` reads `$XDG_RUNTIME_DIR` unconditionally. When `sudo` preserves the env (which it does by default on most distros), the socket lands in the user's dir but owned by root. ADR-003 specifies socket location as `$XDG_RUNTIME_DIR/devtether/` — this is correct for the *current user*, but breaks when the process owner doesn't match the directory owner.

### Options

**Option A: Warn and block (Recommended for now)**
Print a warning and refuse to daemonize when `os.Getuid() == 0` and `-d` is set. DevTether's core use case is `setcap`-based unprivileged port binding, not running as root. This aligns with ADR-003's "secure by default" posture.

**Option B: PID file fallback**
Write a PID file alongside the socket. `devtether down` falls back to `SIGTERM` via PID file when socket dial fails with EACCES. More complex but supports mixed-privilege scenarios.

**Option C: Adjust socket ownership**
After creating the socket as root, `chown` it to `SUDO_UID`/`SUDO_GID`. Fragile — depends on sudo environment variables being set.

### Recommendation
Option A for beta.7. It's the smallest correct fix and matches the project's "no root for dev tools" philosophy. *(Implemented in Subphase 2.4)*

**Update (Subphase 2.9):** With the introduction of the secure runtime directory model in Subphase 2.7, `sudo` no longer leaks root-owned sockets into the user's `$XDG_RUNTIME_DIR`. Instead, it securely isolates the daemon to `/tmp/devtether-0`. Consequently, the `sudo` block (Option A) was **removed** in Subphase 2.9, allowing macOS users to cleanly use `sudo devtether up -d` to bind port 80.

---

## Discussion: `devtether doctor` UX Improvements (Findings 13, 16)

### Problem
1. **Stale socket → ✗**: A stale socket is benign. `devtether up` auto-cleans it. Showing ✗ makes users think they need to manually intervene.
2. **Port 80 + setcap → ✗**: Expected on most systems without `setcap`. DevTether gracefully falls back to 8080. The ✗ implies brokenness when it's actually a normal configuration.
3. **setcap on macOS → ✗**: `getcap` doesn't exist on macOS. macOS doesn't need `setcap` at all (unrestricted since Mojave 10.14). Shows "install libcap2-bin" which is meaningless on macOS.

### Proposed UX Overhaul
Introduce a third check status: `⚠` (warning/info) for conditions that are non-ideal but not broken:

```
  ✓ Config     devtether.yaml is valid
  ⚠ Port 80    unavailable (DevTether will use port 8080 fallback)
  ⚠ Setcap     not set — use setcap or run with sudo for port 80
  ✓ DNS        localhost resolves to 127.0.0.1
  ⚠ Socket     stale socket found (will be cleaned on next startup)
```

On macOS:
```
  - Setcap     not applicable (macOS does not require setcap)
```

### Implementation
- Add a `printWarn` helper alongside `printCheck`.
- Guard setcap with `runtime.GOOS == "linux"`.
- Stale socket becomes a warning, not a failure.
- Port 80 unavailable becomes a warning with fallback note.
- Only count true failures in the exit code.
*(Implemented in Subphase 2.4)*

---

## Discussion: `devtether status` Memory Label (Finding 12)

### Problem
`runtime.MemStats.Alloc` (~0.6 MB) ≠ RSS shown in system monitors (~4.7 MB). The label "Memory" is ambiguous.

### Decision
Rename to "Heap" in the tabwriter output. This is the ponytail choice — one label change, no platform-specific `/proc` reading. Users who need RSS can check their system monitor. *(Implemented in Subphase 2.4)*

---

## Discussion: `devtether logs -f` Follow Flag (Finding 14)

### Problem
Follow mode exists (`--lines 0`) but is undiscoverable. Every log viewer uses `-f`: `docker logs -f`, `tail -f`, `journalctl -f`.

### Decision
Add `-f`/`--follow` boolean flag. When set, ignore `--lines` and enter follow mode directly. This is additive — `--lines 0` remains as an undocumented alias. The help text for `devtether logs --help` must explicitly document `-f` behaviour with a clear example. *(Implemented in Subphase 2.4 & 2.5)*

---

## Discussion: Should Foreground Mode Write to the Log File?

### Context
Currently:
- **`devtether up`** (foreground): all output goes to stdout/stderr. No log file is created.
- **`devtether up -d`** (daemon): output is redirected to `.logs/devtether.log`.
- **`devtether logs`**: reads from `.logs/devtether.log`.

This means `devtether logs` only works after a daemon-mode run. If a user runs foreground, then later does `devtether logs`, they either get stale data from a previous daemon run or "no log file found."

### Options

**Option A: No (Recommended)**
Foreground mode outputs to the terminal. The terminal *is* the log. If the user wants persistent logs, they use `-d`. This is how Docker works: `docker run` goes to stdout; `docker logs` only works for detached containers. Adding tee or dual-write is overengineering for a dev tool where the user is already watching the terminal.

**Option B: Tee to both**
Write to stdout AND the log file simultaneously using `io.MultiWriter`. Lets `devtether logs` work regardless of mode. But adds complexity: should it truncate or append? What about the `.logs/` directory creation on every foreground run? Creates confusion about which run's output you're seeing.

**Option C: Optional flag**
Add `--log-file` to foreground mode. Maximum flexibility, but YAGNI — no user has asked for this.

### Decision
Option A. Foreground = terminal. Daemon = log file. `devtether logs` is a daemon-mode companion. If `devtether logs` is called with no log file, it should clearly say: "No log file found — logs are only created when running in daemon mode (`devtether up -d`)." *(Implemented in Subphase 2.4)*

---

## Design Note: Doctor Setcap + Sudo Messaging

Per user feedback, the doctor setcap message should NOT say "use sudo" — this contradicts the sudo-blocking behaviour from Finding 11. The message should recommend `setcap` only:

```
  ⚠ Setcap     not set — run: sudo setcap cap_net_bind_service=+ep <binary>
```

On macOS: hide the check entirely (no output line at all — not even a "not applicable" note). *(Implemented in Subphase 2.4)*

---

## Discussion: Daemon Lifecycle UX Confusions & Fixes (Post-Phase 2)

During post-Phase 2 verification, several critical lifecycle bugs were discovered that severely degrade the CLI/daemon UX. These all stem from race conditions during startup/shutdown and asynchronous Goroutine/context handling in `up.go` and `logs.go`.

### 1. The `logs -f` Zombie Trap
- **The Issue:** `devtether logs -f` runs a naive `f.Read()` loop that sleeps on `io.EOF`. It has no awareness of whether the daemon is actually writing to the file. If a user runs `devtether down`, the daemon successfully exits, but `logs -f` hangs indefinitely. Because the CLI binary is also named `devtether`, running `top` shows an un-killable `devtether` process, leading users to believe the daemon refuses to stop.
- **The Fix:** ~~`logs -f` should periodically ping the IPC socket.~~ **Revised:** `logs -f` should launch a background goroutine that calls `syscall.Flock(lockFd, LOCK_EX)` (blocking). This blocks in the kernel until the daemon releases the lock (any exit, including SIGKILL). When the flock returns, the tail loop exits cleanly with `Daemon exited.` Zero polling. Requires Subphase 2.7 (flock) to be implemented first.

### 2. The 100ms Shutdown Race (`down` vs `up`)
- **The Issue:** The IPC `/shutdown` endpoint flushes an HTTP 200 OK response, then spawns a goroutine with a 100ms sleep before cancelling the application context. This means `devtether down` completes *before* the daemon actually unlinks the socket. If a user rapidly runs `down` followed by `up` (e.g. in a script), `up` hits the dying socket, assumes the daemon is still healthy, and aborts with `daemon: already running`.
- **The Fix:** ~~`devtether down` needs a fast polling loop.~~ **Revised:** The `/shutdown` handler should use `http.Flusher.Flush()` to send the response immediately, then call `cancelFunc()` directly (no 100ms sleep), then block on `<-r.Context().Done()`. This keeps the HTTP connection alive during the entire graceful shutdown (including the 5s proxy drain). `devtether down` reads the response body with `io.Copy(io.Discard, resp.Body)` — this blocks until the server closes the connection (EOF), which signals that shutdown is complete. Zero polling.

### 3. The 5-Second Ghost TCP Bind
- **The Issue:** When `context.cancel()` triggers, the IPC daemon instantly unlinks the socket via `UnixListener.Close()`. However, the HTTP proxy uses a 5-second timeout for graceful shutdown to flush Keep-Alive connections. During this 5-second window, the socket is gone (so `CheckRunning` passes), but the proxy process is still holding ports 80/8080. If a user runs `devtether up` during this window, the TCP bind fails or falsely triggers the 8080 fallback logic.
- **The Fix:** ~~Verify process death via `Signal(0)`.~~ **Revised:** Fully subsumed by the connection-held-open pattern in Issue 2. Since `handleShutdown` blocks until `r.Context().Done()` fires (which only happens after `http.Server.Shutdown` completes, which only happens after the proxy's 5s drain finishes and the errgroup returns), `devtether down` doesn't exit until the entire process is ready to die. No PID checking needed.

### 4. Silent Fallbacks Swallow the UI
- **The Issue:** When `up -d` executes, it forks a child process and immediately exits. The child then binds ports and prints the rich UI (Route table, `⚠ fell back to port 8080` warnings). Because it's detached, this UI goes directly into the `.logs/devtether.log` file. The user only sees `DevTether daemon started (PID X)` and assumes it bound to port 80. When they navigate to `app.localhost`, it fails because DevTether silently fell back to 8080.
- **The Fix:** ~~`daemonize()` should query `/status` after a timeout.~~ **Revised:** Use an anonymous pipe (`os.Pipe()` + `cmd.ExtraFiles`). The child writes a small JSON payload (containing the bound port) to fd 3 after all servers are bound. The parent blocks on `readEnd.Read()` (event-driven, not polling). The parent then parses the JSON payload, checks if the port differs from `:80`, and explicitly prints an OS-aware warning (omitting `setcap` on macOS) to the foreground terminal before exiting.

### Resolution
All four issues are now covered by event-driven kernel primitives. The polling-based approaches documented in the original discussion have been superseded. See `premortem_2.6_2.7.md` for the full analysis. Updated approach:

| Issue | Old (Polling) | New (Event-Driven) |
|---|---|---|
| `logs -f` zombie | Poll `CheckRunning` 1s | Blocking `flock(LOCK_EX)` |
| `down` wait | Poll `CheckRunning` 50ms | HTTP connection hold + EOF |
| Ghost TCP bind | `Signal(0)` polling | Subsumed by connection hold |
| Silent fallback | Poll `/status` with timeout | Anonymous pipe with JSON payload |

