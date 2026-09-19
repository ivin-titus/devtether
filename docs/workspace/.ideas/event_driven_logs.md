# Event-Driven Log Tailing (Deferred Scope)

## Discussion: `logs -f` Battery Drain

**Date:** 09/19/2026
**Context:** During the Subphase 4.6 Architectural Check, the `devtether-audit` agent flagged that the detached `logs -f` command uses a `50ms` loop to poll the internal `flock` lock file. This polling wakes the CPU 20 times a second, which drains battery and violates the "Lazy Senior Dev" anti-polling standard.

**Decision:** We temporarily opted for the simplest fix (YAGNI): backing off the polling interval from `50ms` to `500ms`. Since exiting a log tail isn't latency-critical, a 500ms delay is perfectly acceptable and drops CPU wakes drastically with zero added code complexity.

However, the ideal, mathematically zero-polling approach requires an event-driven mechanism. This document preserves the architectural design for that future implementation.

## The Ideal Implementation: `fsnotify` Watcher for Locks

While the Linux kernel's `flock(fd, LOCK_EX)` is a blocking syscall that natively waits for a lock to be released, we cannot safely block on it *concurrently* from multiple goroutines or background clients without risking thread starvation or lock contention if the daemon unexpectedly restarts.

Instead, we can exploit the fact that the DevTether daemon deletes its `.pid` file during its graceful shutdown sequence.

### Proposed Architecture

1. Initialize an `fsnotify` watcher on the directory containing the daemon's state files (e.g., `~/.config/devtether/` or `/tmp/devtether-0/`).
2. Add a watcher for the `devtether.pid` file specifically.
3. In a background goroutine, block on `watcher.Events`.
4. If an `fsnotify.Remove` event is emitted for `devtether.pid`, we can reasonably infer the daemon has initiated shutdown.
5. At this point, we can trigger the `flock` check (which is non-blocking) to definitively confirm the lock is released.
6. Cancel the context and cleanly exit the `logs -f` tail.

This completely eliminates all `time.Sleep` loops from the `logs -f` lifecycle, making the entire CLI experience 100% event-driven and strictly zero-polling.
