# Feature Proposal: On-Demand Health Checks

**Status:** Deferred to future release (post-v2.0.0-beta.7)
**Priority:** High

## Problem
The `devtether up -d` daemon currently spawns a background goroutine that polls every configured route every few seconds (`net.Dial`) to print online/offline status changes to the daemon logs. This violates the "Lazy Senior Dev" (YAGNI) standard by generating continuous, infinite CPU wakeups and local network noise for a service that should idle at 0% CPU.

## Proposed Solution
1. **Remove Daemon Polling Entirely**: The background daemon should not proactively poll routes. Instead, it relies strictly on **Passive Health Checks**: when a request routes to a dead backend, the proxy natively catches the `connection refused` error, serves the DevTether HTML 502 page, and logs `[ERROR] domain is offline`.
2. **On-Demand Polling (`devtether routes`)**: To preserve the ability for developers to check route status, the `devtether routes` CLI command should be upgraded. Instead of merely printing the static list of routes from `devtether.yaml` or daemon memory, the CLI should actively `net.Dial` the ports *on invocation* and print the live status:
   ```text
   app.localhost   → :3000   [● Online]
   api.localhost   → :8042   [○ Offline]
   ```

## Why it was deferred
Refactoring the CLI client's output table and ensuring concurrent dial timeouts within the `devtether routes` command execution was deemed too much risk/stress for the stabilization phase of `beta.7`. To mitigate the CPU tax in the interim, the polling delay was increased from `1.5s` to `3.0s`.

This proposal should be prioritized in `beta.8`.
