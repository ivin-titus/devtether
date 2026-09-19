# Daemon Log Rotation & Size Capping

**Status:** Deferred to a future release.

## Context & Vision
Currently, when `devtether up -d` is executed, the daemon detaches and writes all `stdout`/`stderr` logs to a flat file at `settings.log_path` (defaulting to `./.logs/devtether.log`). 

Over time, especially for users who leave the daemon running for weeks at a time, this `.logs/devtether.log` file will grow unbounded. While Phase 3 implemented a bounded backwards read for the `devtether logs --lines` command, the disk consumption issue remains unaddressed.

### Proposed Solutions
1. **Built-in `lumberjack` style rotation:** Integrate a lightweight, pure-Go log rotation package to automatically cap the file size (e.g., 10MB) and keep a limited number of backups.
2. **OS-Level Delegation (Systemd/Launchd):** When DevTether is eventually deployed as an OS-Native service (Engine 3 / Service Module), we can completely drop file-based logging and delegate everything to the OS (e.g., `journalctl` on Linux or `syslog` on macOS), which natively handles rotation and size capping.

## Why Deferred?
Given our strict "No AI Slop / Engine 1 Focus" mandate (`docs/engineering-standards.md`), we are prioritizing stability and core networking primitives over auxiliary features. Since DevTether is currently aimed at local development (where daemons are frequently restarted and log volumes are relatively low), unbounded log growth is an acceptable technical debt for the beta phase. We will revisit this when we implement the OS Service Module.
