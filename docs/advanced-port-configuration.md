# Advanced Port Configuration

DevTether uses safe defaults and a small amount of explicit setup rather than claiming zero configuration.

The common path is:

```text
devtether init
→ add routes
→ devtether up
```

Use this guide when changing proxy or DNS ports manually.

## 1. Proxy Port

The default configured proxy port is **80**.

### Unprivileged ports

A value such as `8000`, `8080`, or `3000` can be bound by a normal user.

The configured port is always tried first and the listener remains loopback-only.

### Privileged ports

For values below `1024`, binding the requested port requires the relevant OS privileges.

On Linux you can grant the binary:

```bash
sudo setcap cap_net_bind_service=+ep $(which devtether)
```

On macOS, use the required administrative privileges when binding a privileged port.

### Configured port vs fallback port

If the configured proxy port cannot be bound because of permission or address-in-use conditions, DevTether tries **127.0.0.1:8080**.

```text
configured port
    ↓
success → use configured port
    ↓
permission/address-in-use failure
    ↓
try 127.0.0.1:8080
    ↓
failure → startup fails
```

There is no random or OS-assigned `:0` fallback.

Therefore, privileges are required to keep a privileged configured port; they are not necessarily required for DevTether to start if the 8080 fallback is available.

## 2. DNS Port

The current default DNS bind is:

```text
127.0.0.1:5335
```

You can change `dns.bind`, for example:

```yaml
dns:
  bind: "127.0.0.1:5300"
```

The embedded DNS server uses that exact address and port. It does not silently switch to another DNS port.

### Resolver alignment

Your host resolver must forward `localhost` queries to the same port.

For example, if your Linux resolver contains:

```text
DNS=127.0.0.1:5335
```

and you change DevTether to `5300`, update the resolver entry to `5300` too.

The same rule applies to macOS `/etc/resolver/localhost` and dnsmasq configuration.

`devtether init` writes the current `5335` default into the resolver configuration it creates. Manual DNS-port changes are intentionally an advanced/manual operation.

## 3. Log Path

When `settings.log_path` is omitted, detached logs are written to:

```text
<config-directory>/.logs/devtether.log
```

When `settings.log_path` is provided, its value is used as the log directory. A relative custom path is resolved from the process working directory; use an absolute path for an unambiguous location when invoking a config from another directory.

Detached log opening is fail-closed: the final log directory must be a real directory (not a symlink) owned by the current user and not group/world-writable, and `devtether.log` must be a regular file owned by the current user with a single hard link and no group/world write permission. Unsafe paths are rejected before the daemon starts. Intermediate ancestor directories may still be symlinks (for example macOS `/var` → `/private/var`); only the final directory and the log file itself are protected.

Ownership is checked against the invoking user's real UID. If the log directory was created under a different execution mode (for example `sudo` versus a normal user), that mode cannot reuse it; run with the same mode or set `settings.log_path` to a separate directory for the other mode. Filesystems that do not honor Unix ownership (for example NTFS/exFAT/FUSE mounts) report every file as the mounting user, so a different-UID execution cannot pass the ownership check there — point `settings.log_path` at a native filesystem in that case.

## 4. Keep the DNS and Proxy Boundaries Separate

- `proxy.port` controls the HTTP reverse proxy.
- `dns.bind` controls the embedded DNS listener.
- Changing one does not automatically change the other.

Port changes are deliberately explicit so the daemon's behavior stays predictable.
