# Service Mode & System Integration - Discussion Space
*(This document preserves historical architectural discussions for the deferred OS Service and daemonization features.)*

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

---

## Discussion: Phase 3 (Onboarding & Integration) Blindspots
As we move into Phase 3 (the interactive `init` wizard and the `service install` system), we are touching operating system integrations. Based on industry standards (and lessons learned from Phase 2), here are the critical blindspots we must resolve before executing:

### Subphase 3.1: `devtether init` Interactive Wizard
1. **The Automation & Flag Blindspot (Next.js Style UX):**
   - *Risk:* Interactive prompts break CI/CD pipelines. A simple `--yes` flag is too blunt.
   - *Resolution:* Follow modern UX (like Next.js/Vite). Support explicit flags for every prompt: `--daemon=true`, `--port=8080`, `--setcap=false`. If `os.Stdin` is not a TTY, require these flags or fallback to safe defaults.
2. **The Idempotency Blindspot (Safe Overwrites):**
   - *Risk:* Silently overwriting an existing `devtether.yaml` destroys user configuration.
   - *Resolution:* If `devtether.yaml` exists, explicitly print: `"devtether.yaml already exists. Skipping initialization."` and instruct the user to `"Rerun with --force to overwrite."`
3. **The macOS `setcap` Blindspot:**
   - *Risk:* Prompting for `setcap` on macOS is a terrible UX (it doesn't exist).
   - *Resolution:* The wizard flow must skip the `setcap` prompt entirely if `runtime.GOOS != "linux"`.

### Subphase 3.2: `devtether service install` & `uninstall`
4. **The Privilege/Path Blindspot (Interactive Elevation):**
   - *Risk:* Users running `devtether service install` without `sudo` will get cryptic permission denied errors when we try to write to `/etc/systemd/system/`. 
   - *Resolution:* If the user lacks root privileges, intercept the flow and prompt: `"No system privileges detected. Would you like to install this as a root daemon (requires sudo)? [y/N]"`. If yes, we can instruct them to rerun with `sudo devtether service install` (or potentially invoke `sudo` internally). Otherwise, fall back to installing a user-level daemon (`~/.config/systemd/user/`).
5. **The Service Uninstall Blindspot:**
   - *Risk:* Providing `install` without a robust `uninstall` orphans daemon files on the host OS.
   - *Resolution:* Explicitly implement `devtether service uninstall`. It must stop the service, disable it from booting, and securely remove the `.service`/`.plist` file from the host OS, respecting the same privilege tiers as the installer.
6. **The Working Directory / Config Resolution Blindspot:**
   - *Risk:* `systemd`/`launchd` run with an undefined working directory. `devtether up` defaults to `./devtether.yaml`.
   - *Resolution:* The generated service file MUST explicitly pass the absolute config path: `ExecStart=/usr/local/bin/devtether up --config /absolute/path/devtether.yaml`.

### Audit-Driven Additions (P3-1 through P3-5)
7. **Idempotency Exit Code (P3-1):**
   - *Question:* Current `init` returns exit 1 when file exists. Should we keep exit 1 (safer for scripts) or switch to exit 0 (friendlier)?
   - *Recommendation:* Keep exit 1 with improved message: `"devtether.yaml already exists. Skipping initialization. (rerun with --force to overwrite)"`.
8. **Service Activation Semantics (P3-4):**
   - *Question:* Should `devtether service install` only write the unit file, or also reload and enable/start the service?
   - *Recommendation:* `install` = write + reload + enable. `uninstall` = stop + disable + remove + reload. This matches `systemctl enable --now` industry standard.
9. **`--setcap` Flag Semantics (P3-5):**
   - *Question:* Should `--setcap` on `devtether init` actually run `sudo setcap` on the binary, or just toggle config comments?
   - *Recommendation:* Actually apply setcap via `exec.Command("sudo", "setcap", ...)`. This is a convenience action, not a config change.

### Subphase 3.2.1: Post-Implementation Audit Fixes (P3-9 through P3-12)
The post-implementation audit identified 4 architectural flaws that must be resolved before proceeding to documentation.

10. **TTY Detection Testability Bypass (P3-9):**
    - *Risk:* `init.go` and `service.go` accept an `io.Reader` for testing, but they hardcode `term.IsTerminal(int(os.Stdin.Fd()))`. This breaks the `engineering-standards.md` testing rules because running `go test` from an actual terminal will cause the tests to hang waiting for input.
    - *Resolution:* Abstract the TTY check into a function passed as a dependency or check the `Fd()` of the actual provided `io.Reader` (by casting it to an `*os.File` or similar `Fd()` interface).
11. **Uninstall Service State Leak (P3-10):**
    - *Risk:* If a user manually deletes `/etc/systemd/system/devtether.service`, `UninstallService` immediately errors out and fails to run `systemctl disable devtether` and `systemctl stop devtether`. The service keeps running indefinitely in the kernel.
    - *Resolution:* Deactivation must occur *before* asserting the file exists on disk.
12. **Blind `systemd` Execution on Linux (P3-11):**
    - *Risk:* `service install` assumes `systemd` is the init system on all Linux hosts. It will fail cryptically on Alpine/Docker.
    - *Resolution:* Explicitly verify systemd presence (e.g. check if `/run/systemd/system` exists) before installing.
13. **`launchd` Load Idempotency (P3-12):**
    - *Risk:* `launchctl load -w` fails if the service is already loaded.
    - *Resolution:* Ignore the specific exit code or perform an `unload` before `load`.
14. **Service Environment Stripping (P3-13):**
    - *Risk:* Systemd and launchd strip user environment variables like `$HOME`. If `devtether up` attempts to write to paths relative to the current directory or the user's home, it could resolve to `/` and crash.
    - *Resolution:* Rely on absolute paths explicitly.
15. **Missing Service Working Directory (P3-14):**
    - *Risk:* The templates do not specify a working directory, defaulting to `/`. Any relative paths in `devtether.yaml` will fail.
    - *Resolution:* Inject `WorkingDirectory` explicitly into the systemd and launchd templates, pointing to the directory containing the config file.
16. **Launchd Log Path Initialization (P3-15):**
    - *Risk:* `launchd` will silently fail if the parent directory of `StandardOutPath` does not exist.
    - *Resolution:* Ensure `os.MkdirAll` is called on the log directory path during `service install` on macOS.
17. **Ephemeral `setcap` Paths (P3-16):**
    - *Risk:* If the user runs `go run main.go init --setcap`, setcap is applied to the temporary go-build binary, silently vanishing later.
    - *Resolution:* Warn if `os.Executable()` is inside `/tmp` or `go-build`.
18. **Systemd Path Whitespace Parsing Failure (P3-17):**
    - *Risk:* The generated `ExecStart` line in systemd is unquoted. Because the config file path might contain spaces (e.g., `/path/with spaces/devtether.yaml`), systemd will split on the space and crash immediately.
    - *Resolution:* Wrap paths in quotes in `systemdTemplate`: `ExecStart="{{.BinaryPath}}" up --config "{{.ConfigPath}}"`
19. **Privilege Inversion on User Services (P3-18):**
    - *Risk:* Running `sudo devtether service install --user` resolves `$HOME` to `/root` but executes `systemctl --user enable` as root, which crashes due to a missing D-Bus session.
    - *Resolution:* Actively block `--user` (or inferred user mode) if `daemon.IsRoot()` is true.
20. **Unquoted Shell Fallback Command (P3-19):**
    - *Risk:* The CLI prints `sudo setcap ... $(which devtether)` if setcap fails. If `which devtether` returns a path with spaces, the copy-pasted command will fail.
    - *Resolution:* Quote the output: `sudo setcap ... "%s"` instead of a subshell.

---

## Discussion: CLI Flags vs Wizard Questions (Logging & Automation)

A question was raised regarding whether `devtether init` should ask the user for their preferred logging mode (e.g., standard vs. verbose/debug) during the interactive wizard, and whether we need non-TTY flags (like `--debug=true`) for automation.

**My Opinion:**

1. **The Wizard Question is Overkill:** The DevTether `init` wizard should remain as fast and frictionless as possible. The vast majority of developers do not want to be bogged down with 10 different configuration questions. If we just generate a rich, heavily commented `devtether.yaml` (as discussed in DX-1), DevOps users can simply open the file and uncomment `log_level: debug`. We should only ask for things in the wizard that are *mandatory* or require OS-level interaction (like ports, DNS, and setcap).
2. **Non-TTY Flags are Necessary (For Automation):** While the interactive wizard shouldn't ask about logging, the CLI *should* support non-TTY flags for automation scripts. For example, `devtether init --log-level=debug --force` would allow a CI/CD pipeline or deployment script to generate a config file without hanging on an interactive prompt. 
3. **Global CLI Overrides:** It would also be highly beneficial to have global CLI flags (e.g., `devtether up --debug`) that temporarily override the YAML config. This is a standard CLI pattern.

**Verdict:** Do not add a logging question to the interactive wizard (keep it lean). However, we should plan to add comprehensive non-TTY override flags (`--debug`, `--port`, etc.) to the CLI commands for automation purposes. This is classic "Scope Discovery" for a future phase.
