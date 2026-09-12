# Audit Report: DevTether Codebase

**Date:** 09/10/2026  
**Auditor:** `@ivin-titus (via Agent:GPT-5.6 Luna)`

## 1. Executive Summary

The DevTether audit identified a set of security, reliability, protocol, concurrency, testing, observability, and initialization issues across the DNS engine, reverse proxy, router, daemon/IPC layer, CLI, logging, and test suite.

The audit covered the codebase state around commit `777af1f` together with the associated uncommitted modifications and reference engineering documents. The majority of higher-severity findings were subsequently marked resolved in the source audit, while several lower-priority architectural and UX issues remain deferred. Following the completion of Phase 2, the remaining explicitly unresolved findings include IPv6 bracket normalization, ANSI corruption in logging, DNS record construction inefficiency, and loss of log severity context.

The core implementation was assessed as generally resilient, with the audit concluding that the principal remaining concerns are edge-case host normalization, logging quality, and DNS parsing efficiency.

The public repository describes DevTether as a Linux-oriented, modular developer networking toolkit built around static routing, a DNS engine, reverse proxy, and Unix-domain-socket IPC daemon, with additional orchestration, tunneling, and access-control engines planned.

## 2. Methodology

The audit source reports:

- Static analysis of the generated unified diff representing Phases 1–6.
- Traceability analysis of error paths, context propagation, network binding lifecycle, and URL normalization across the DNS engine and proxy handler.
- Targeted runtime verification of `net.SplitHostPort` behavior with IPv6 bare host headers.
- Review of `cli_test.go` test-suite behavior.
- Review of `cmd/devtether/main.go`, `internal/cli/*`, `internal/config/*`, `internal/daemon/*`, `internal/dns/*`, `internal/logger/*`, `internal/proxy/*`, and `internal/router/*`.
- Cross-reference against `implementation_plan.md`, `audit_report.md`, `github_issue.md`, `engineering-standards.md`, and ADRs.
- Additional repository-context review of the public DevTether repository and its published architecture and project status.

## 3. Detailed Findings

### Finding 1: DNS Engine Unrecovered Panic Vulnerability

- **Severity:** Critical
- **Location:** `internal/dns/server.go:119` (`handleRequest`)
- **Status:** Resolved in `v2.0.0-beta.5`
- **Description:** The DNS handler processes untrusted UDP packets in a goroutine without an enclosing panic-recovery block. An unexpected slice-bound error, malformed packet condition, or future logic defect inside `handleRequest` could cause an unrecovered panic and terminate the entire DevTether process. This represents a local denial-of-service risk and would become a remote denial-of-service risk if LAN DNS binding is enabled.
- **Remediation Plan:** Wrap the complete `handleRequest` body in a `defer` recovery block, log recovered panics as errors, and prevent a single malformed DNS request from terminating the daemon.

### Finding 2: Reverse Proxy Streaming Violations (Timeouts)

- **Severity:** Critical
- **Location:** `internal/proxy/server.go:40`; `internal/config/config.go:85`
- **Status:** Resolved in `v2.0.0-beta.5`
- **Description:** The proxy's underlying `http.Server` used a hardcoded `ReadTimeout` of 30 seconds and `WriteTimeout` of 60 seconds. Applied globally to reverse-proxy traffic, these absolute deadlines can terminate uploads exceeding 30 seconds and downloads, video streams, WebSocket connections, SSE connections, or other long-lived responses exceeding 60 seconds, even while data is actively flowing.
- **Impact:** Core local-development workflows involving streaming, WebSockets, SSE, hot-reloading streams, and large payloads can be terminated unexpectedly.
- **Remediation Plan:** Remove global `ReadTimeout` and `WriteTimeout` from the reverse-proxy server. Use `ReadHeaderTimeout` for Slowloris protection, `IdleTimeout` for dead connections, and transport-level connection timeouts for outbound dialing.

### Finding 3: IPC Socket TOCTOU Vulnerability & Fast-Restart Race

- **Severity:** High
- **Location:** `internal/daemon/daemon.go:115` (`Start` graceful shutdown); `internal/daemon/daemon.go:96` (`Start` fallback boot)
- **Status:** Resolved in `v2.0.0-beta.5`
- **Description:** The Unix socket is created before its permissions are tightened with `os.Chmod`, creating a race window when the socket falls back to `/tmp/devtether.sock`. In addition, shutdown explicitly removes the socket path after draining connections. A rapid restart can therefore allow a new daemon instance to create a replacement socket before the old instance executes `os.Remove`, allowing the old instance to delete the new instance's active socket.
- **Impact:** Potential local privilege-escalation exposure during insecure socket creation and severe CLI/orchestration breakage during fast daemon restarts.
- **Remediation Plan:** Guarantee restrictive socket creation atomically using an appropriate restrictive `umask`, and remove the unconditional shutdown-time socket deletion so the active listener lifecycle and stale-socket cleanup logic remain authoritative.

### Finding 4: Proxy Engine Violates Secure-By-Default Loopback Binding

- **Severity:** High
- **Location:** `internal/proxy/server.go:85` (`bind`)
- **Status:** Resolved in `v2.0.0-beta.5`
- **Description:** The configured-port bind path used an address such as `":80"`, causing the operating system to listen on all available interfaces rather than loopback only. This conflicts with the project's secure-by-default loopback design. The fallback path, by contrast, correctly used `127.0.0.1:0`, producing inconsistent network exposure depending on which bind path succeeded.
  ```go
  // Step 1: Try configured port.
  addr := fmt.Sprintf(":%d", s.port)
  listener, err := lc.Listen(ctx, "tcp", addr)
  ```
- **Impact:** Local development services could become reachable from an untrusted or public LAN without explicit user intent.
- **Remediation Plan:** Bind the configured proxy port explicitly to loopback, for example `127.0.0.1:<port>`, and preserve LAN exposure only as an explicit opt-in capability.

### Finding 5: Encapsulation Leak and Concurrency Vulnerability in Router

- **Severity:** High
- **Location:** `internal/router/router.go:80` (`Resolve`); `internal/router/router.go:101` (`GetAllRoutes`)
- **Status:** Resolved in `v2.0.0-beta.5`
- **Description:** Router methods returned raw pointers to internal `Target` structures. Although the route map itself was protected by `sync.RWMutex`, callers could retain and mutate the returned objects outside the lock. `GetAllRoutes` copied the map while continuing to share the underlying `Target` pointers. Future in-place mutations could therefore create data races between writers and readers such as the IPC serialization path.
- **Impact:** Thread-safety guarantees are weakened and future orchestration or dynamic-route features could introduce fatal data races.
- **Remediation Plan:** Return copies of `Target` values instead of exposing pointers to mutable internal state.

### Finding 6: DNS Engine NXDOMAIN Protocol Violation

- **Severity:** High
- **Location:** `internal/dns/server.go:134` (`handleRequest`)
- **Status:** Resolved in `v2.0.0-beta.5`
- **Description:** The DNS query loop ignored all non-A queries. Because those queries never set the route-match state, valid domains could fall through to an `NXDOMAIN` response. A domain that exists but does not contain the requested record type should not be represented as non-existent merely because the query was for another record type.
- **Impact:** Dual-stack systems issuing `A` and `AAAA` queries could experience incorrect resolution failures, including browser "Server Not Found" behavior despite a valid `A` record.
- **Remediation Plan:** Separate route-existence detection from record-generation logic. Once a route exists, return `NOERROR` and append an `A` record only when an A record was requested.

### Finding 7: Router Key Normalization Contract Violation

- **Severity:** High
- **Location:** `internal/router/router.go:54` (`AddRoute`)
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** `AddRoute` lowercased route domains but did not remove a trailing dot. The proxy and DNS paths did remove trailing dots before lookup, meaning a route configured as `api.localhost.` could be stored under a key that would never be returned by normal resolution.
- **Impact:** Valid configuration entries could become silently unreachable depending on domain formatting.
- **Remediation Plan:** Normalize route keys consistently at insertion time, for example by lowercasing and removing one trailing dot before storing the route.

### Finding 8: Daemon Pre-flight Probe Hides Permission Errors

- **Severity:** High
- **Location:** `internal/daemon/daemon.go:26` (`CheckRunning`)
- **Status:** Resolved in `v2.0.0-beta.5`
- **Description:** Any IPC dial error was effectively interpreted as "daemon is not running." A permission-denied condition caused by another user owning the socket could therefore be mistaken for an absent daemon, allowing startup to proceed into fallback port binding before the later socket operation failed.
- **Impact:** Misleading startup behavior and poor diagnostics in multi-user environments.
- **Remediation Plan:** Detect permission-related errors explicitly and return them immediately with a useful ownership/access error instead of treating them as daemon absence.

### Finding 9: Proxy Double-Header Corruption on Chunked Backend Crashes

- **Severity:** Medium
- **Location:** `internal/proxy/middleware.go:38` (`Write`); `internal/proxy/handler.go:87` (`ErrorHandler`)
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** `loggingResponseWriter.Write()` did not mark `wroteHeader` when the first `Write()` implicitly generated a `200 OK`. If the backend subsequently failed mid-stream and the reverse proxy invoked its custom error handler, the middleware could incorrectly allow a second `502` header and an HTML error page to be written into an already-started response stream.
- **Impact:** Client payload corruption and `multiple response.WriteHeader` warnings when streaming backends fail.
- **Remediation Plan:** Ensure the response wrapper records the implicit `200 OK` when `Write()` is the first response operation.

### Finding 10: Inconsistent IPv6 Bracket Normalization in Proxy

- **Severity:** Medium
- **Location:** `internal/proxy/handler.go` (`netutil.NormalizeHost`)
- **Status:** Resolved in Phase 1
- **Description:** `net.SplitHostPort("[::1]:8080")` returns the IPv6 address without brackets, while `net.SplitHostPort("[::1]")` fails with a missing-port error. The fallback path consequently preserves brackets for the second case, producing inconsistent host normalization depending on whether a port appears in the Host header.
- **Impact:** Edge-case IPv6 routes may resolve successfully for one Host-header form and return 404 for another.
- **Remediation Plan:** Normalize IPv6 brackets independently of the presence of a port, including explicit bracket stripping after host/port parsing or equivalent robust host normalization.

### Finding 11: Reverse Proxy TOCTOU Race and Redundant Lookup

- **Severity:** Medium
- **Location:** `internal/proxy/handler.go:44` (`Rewrite`) and `ServeHTTP`
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** `ServeHTTP` resolves a route target and stores it in request context, but `ReverseProxy.Rewrite` performs a second resolver lookup instead of using that already-resolved target. Under highly volatile future dynamic-routing conditions, the second lookup could return `nil` after the initial lookup succeeded.
- **Impact:** Redundant routing work and a minor time-of-check/time-of-use window that could result in unexpected `502` responses during dynamic route changes.
- **Remediation Plan:** Use the target stored in request context inside `Rewrite` instead of re-resolving the host.

### Finding 12: Zombie Signal Handler / Unkillable Process

- **Severity:** Medium
- **Location:** `internal/cli/up.go:128` (signal goroutine)
- **Status:** Resolved in Phase 2
- **Description:** The signal handler catches `SIGINT`/`SIGTERM`, initiates cancellation, imposes a five-second shutdown deadline, and then exits without calling `signal.Stop(sigCh)`. Because `signal.Notify` has overridden the default behavior, subsequent `Ctrl+C` signals can be delivered to an abandoned channel rather than restoring default termination semantics.
- **Impact:** Repeated `Ctrl+C` presses may fail to terminate a stalled process immediately, potentially forcing the user to wait or use `kill -9`.
- **Remediation Plan:** Stop signal notification immediately after the first signal is handled so subsequent signals can invoke normal operating-system termination behavior.

### Finding 13: Hardcoded ANSI Corruption in Logging Pipelines

- **Severity:** Medium
- **Location:** `internal/logger/logger.go:30` (`devHandler.Handle`)
- **Status:** Resolved in Phase 3
- **Description:** Non-verbose logging unconditionally embeds ANSI color escape sequences around structured attributes. Redirected output and background/service execution can therefore contain terminal control sequences even when no interactive terminal is present.
  ```go
  if len(attrs) > 0 {
      msg = fmt.Sprintf("%s \033[90m(%s)\033[0m", msg, strings.Join(attrs, " "))
  }
  ```
- **Impact:** Redirected files and external log-processing pipelines can receive polluted, non-printable characters that interfere with parsing and readability.
- **Remediation Plan:** Emit ANSI formatting only when the output is a TTY, or provide an explicit no-color path and use non-colored output for redirected/service logs.

### Finding 14: Synchronous Health Check Linear Scaling Defect

- **Severity:** Low
- **Location:** `internal/cli/up.go:232` (health-check ticker loop)
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** Health checks iterate through routes sequentially, with `isBackendOnline` using a 200 ms dial timeout. With many unreachable targets that blackhole packets, total check duration grows linearly with route count and can exceed the 1.5-second ticker interval.
- **Impact:** Dashboard health indicators can become increasingly delayed or erratic as the number of routes grows.
- **Remediation Plan:** Run backend health checks concurrently using coordinated goroutines so total wall-clock time is bounded by the slowest individual check rather than the sum of all checks.

### Finding 15: Missing Content-Security-Policy on Proxy Error Pages

- **Severity:** Low
- **Location:** `internal/proxy/handler.go:200` (`renderErrorPage`)
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** Error pages reflect untrusted request data such as Host headers and URL paths. Go's contextual `html/template` escaping provides strong protection, but no `Content-Security-Policy` response header was added as a secondary browser-level defense.
- **Impact:** A future template regression or escaping bypass would have fewer browser-side defenses against script execution.
- **Remediation Plan:** Add an appropriate restrictive CSP header to the generated proxy error pages.

### Finding 16: Test Pipe Deadlock Risk on Panic or Verbose Output

- **Severity:** Low
- **Location:** `internal/cli/cli_test.go:63`
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** CLI output is captured through an `os.Pipe`, but the test waits for `Execute()` to finish before reading from the pipe. If output exceeds the pipe buffer, the writer can block indefinitely because no concurrent reader is draining the pipe. A panic can also prevent cleanup from executing.
  ```go
  err := Execute()
  _ = w.Close()
  ```
- **Impact:** Potential CI deadlocks or test-state corruption if output becomes sufficiently large or the command panics.
- **Remediation Plan:** Drain the read side concurrently while `Execute()` runs and use deferred restoration/cleanup of `os.Stdout` and pipe resources.

### Finding 17: Global `os.Stdout` Manipulation in Tests

- **Severity:** Low
- **Location:** `internal/cli/cli_test.go:50`
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** `TestVersionCmd` globally replaces `os.Stdout` without package-level synchronization or process isolation. This works while tests are effectively serialized, but creates a hidden concurrency hazard if future tests introduce `t.Parallel()`.
- **Impact:** Future parallel tests could capture or corrupt each other's output and produce nondeterministic failures.
- **Remediation Plan:** Avoid global stdout interception where practical, document the limitation, or isolate CLI execution in a subprocess for output-sensitive tests.

### Finding 18: Silent Masking of Daemon IPC Errors

- **Severity:** Low
- **Location:** `internal/cli/routes.go:30` (`runRoutes`)
- **Status:** Resolved in Phase 2
- **Description:** `runRoutes` attempts to retrieve active routes through the daemon, but treats any IPC error as evidence that the daemon is not running and falls back to reading the configuration file.
  ```go
  routes, err := client.ListRoutes()
  if err == nil {
      printRoutes(routes)
      return nil
  }
  // Daemon not running — fall back to reading config file.
  ```
- **Impact:** Permission failures, daemon-side failures, or server errors can be silently hidden while the CLI presents stale configuration data as though it were authoritative.
- **Remediation Plan:** Fall back to configuration-file loading only for explicit "daemon unavailable" conditions such as connection refusal or missing-socket errors; propagate other IPC errors.

### Finding 19: Non-Deterministic Output

- **Severity:** Low
- **Location:** `internal/cli/routes.go:54` (`runRoutes`); `internal/daemon/daemon.go:143`
- **Status:** Resolved in Phase 2
- **Description:** Route responses are generated from maps without sorting. Go map iteration order is intentionally non-deterministic, so route listings can appear in a different order across invocations.
- **Impact:** Inconsistent CLI output violates the project's deterministic-output engineering standard and complicates snapshots, scripts, and user expectations.
- **Remediation Plan:** Sort route responses by `Domain` before returning or printing them.

### Finding 20: Inefficient DNS Record Parser on Hot Path

- **Severity:** Low
- **Location:** `internal/dns/server.go:149`
- **Status:** Resolved in Phase 3
- **Description:** The DNS handler constructs a textual resource-record representation with `fmt.Sprintf` and then passes it through `dns.NewRR`, causing a full parser/lexer cycle for each incoming request.
  ```go
  rr, err := dns.NewRR(fmt.Sprintf("%s A 127.0.0.1", q.Name))
  ```
- **Impact:** Avoidable CPU and allocation overhead on the DNS request hot path.
- **Remediation Plan:** Construct the DNS A-record structure directly rather than serializing and reparsing a textual record.

### Finding 21: Loss of Error Severity Context in Standard Logs

- **Severity:** Low
- **Location:** `internal/logger/logger.go:19` (`devHandler.Handle`)
- **Status:** Resolved in Phase 3
- **Description:** In non-verbose mode, the logger extracts the message and attributes but ignores `r.Level`. Error and warning messages can therefore appear visually equivalent to informational messages.
- **Impact:** Important subsystem failures can be overlooked in normal terminal output.
- **Remediation Plan:** Preserve warning/error severity in standard output through an explicit severity marker, suitable visual distinction, or routing of severe events to an appropriate error stream.

### Finding 22: `--config` Flag Silently Ignored by `devtether routes`

- **Severity:** Low
- **Location:** `internal/cli/routes.go:28` (`runRoutes`)
- **Status:** Resolved in Phase 2
- **Description:** `devtether routes --config /path/to/other.yaml` can still prefer the running daemon and return the daemon's active routes instead of the routes from the explicitly requested configuration file.
  ```go
  // Try the daemon first.
  client := daemon.NewClient()
  routes, err := client.ListRoutes()
  ```
- **Impact:** The CLI can silently show a different route set from the one the user explicitly requested, creating significant operator confusion.
- **Remediation Plan:** When the `--config` flag was explicitly changed, bypass the daemon and inspect the requested configuration directly, or clearly document and surface the precedence behavior.

### Finding 23: Throttle Cache O(N) Iteration on Saturated Limit

- **Severity:** Low
- **Location:** `internal/proxy/handler.go:193` (`throttleCache.LoggedRecently`)
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** When the bounded throttle cache reaches its 1024-entry limit and contains no expired entries, incoming requests may trigger a full map iteration to locate expired entries followed by another iteration to choose an eviction candidate.
- **Impact:** Under extreme floods of unique non-existent routes, each request can incur O(N) map iteration. The source audit considered the practical impact negligible at the configured size.
- **Remediation Plan:** No immediate action required. The current bounded-cache approach remains an acceptable simplicity/performance tradeoff unless future measurements demonstrate a meaningful bottleneck.

### Finding 24: Proxy Loop 404 on Init Default Config

- **Severity:** High
- **Location:** `internal/cli/init.go:28` (`defaultConfig`); `internal/proxy/handler.go:114` (`ServeHTTP`)
- **Status:** Resolved in Phase 2
- **Description:** When DevTether cannot bind its preferred privileged port and falls back to port `8080`, `devtether init` can generate a default route such as `api.localhost:8080`. That route points to `127.0.0.1:8080`, which is itself the DevTether proxy. The forwarded request re-enters the proxy with `Host: 127.0.0.1:8080`; host sanitization removes the port, and the router cannot resolve `127.0.0.1`. The request consequently receives a 404. Meanwhile, the health check can still mark the backend as online because the proxy itself is listening on the target port.
- **Impact:** The generated default configuration can produce an apparently healthy route that actually loops back into DevTether and fails with 404, creating a broken first-run experience on systems where port 80 requires elevated privileges.
- **Remediation Plan:** Change the generated default route to use a port outside DevTether's fallback chain, such as `8081` or `3000`, and consider adding explicit proxy-loop detection.

### Finding 36: Dynamic UI CLI Bypasses Logger and Emits Hardcoded ANSI

- **Severity:** Low
- **Location:** `internal/cli/up.go:130` and `internal/cli/up.go:163`
- **Status:** Resolved in Phase 3.C
- **Description:** The core logger uses `os.ModeCharDevice` to strip ANSI escapes, which violates ADR-006 and the explicit engineering standards mandating `golang.org/x/term`. Furthermore, the `devtether up` command's startup sequence uses a "Dynamic UI" banner that directly prints `\033[90m` escape sequences to `os.Stdout` via `fmt.Printf`. This bypasses the logger entirely, resulting in ANSI corruption if the daemon's startup output is redirected to a file or a service manager.
- **Impact:** Redirected daemon logs are polluted with non-printable characters. The current TTY check in `logger.go` is non-compliant with Windows terminal emulators.
- **Remediation Plan:** Apply a standards-compliant strategy. Restore `golang.org/x/term` to `logger.go`. In the startup UI functions, define ANSI sequences as local variables, check `term.IsTerminal(int(os.Stdout.Fd()))`, and reassign the color variables to empty strings (`""`) if the output is not a terminal.

## 4. Resolved Findings

The source audit explicitly records the following previously identified issues as resolved:

### Finding 25: Dedicated "No Route Found" 404 Page

- **Severity:** Not specified
- **Location:** Not specified in source audit
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** The raw `http.Error` fallback for missing routes was replaced with a dedicated HTML error template.
- **Remediation Plan:** Completed.

### Finding 26: "502 Bad Gateway" Formatting Defect

- **Severity:** Not specified
- **Location:** Not specified in source audit
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** Error-page CSS was updated to wrap long values safely using `word-break: break-all` and Flexbox-based columns for small screens.
- **Remediation Plan:** Completed.

### Finding 27: Missing Changelog Entries

- **Severity:** Not specified
- **Location:** Not specified in source audit
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** Release documentation was updated to include the missing changelog entries.
- **Remediation Plan:** Completed.

### Finding 28: Missing Custom Proxy Transport

- **Severity:** Not specified
- **Location:** Not specified in source audit
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** A custom `http.Transport` was introduced with timeout configuration including `ResponseHeaderTimeout`.
- **Remediation Plan:** Completed.

### Finding 29: Unbounded Proxy Error Log Map

- **Severity:** Not specified
- **Location:** Not specified in source audit
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** The unbounded `sync.Map` error logger was replaced by a bounded `throttleCache` to prevent unbounded memory growth.
- **Remediation Plan:** Completed.

### Finding 30: Improper Handling of Client Context Cancellation

- **Severity:** Not specified
- **Location:** Not specified in source audit
- **Status:** Resolved in `v2.0.0-beta.4`
- **Description:** `context.Canceled` conditions are now handled silently to prevent noisy `502` logging after a client disconnects.
- **Remediation Plan:** Completed.

## 5. Protected Threat Checks

The source audit also records the following external threat-audit areas as protected under ADR-009. These are documented as **SAFE**, rather than active vulnerabilities:

### Finding 31: Web UI & Streaming Hijacking (CSWSH)

- **Severity:** Low
- **Location:** Not specified in source audit
- **Status:** Protected under ADR-009
- **Description:** External threat audit identified no active vulnerability in this category.
- **Remediation Plan:** Maintain the existing ADR-009 protections and revalidate them when the Web UI or streaming implementation changes.

### Finding 32: Insecure Service Integration (Systemd LPE)

- **Severity:** Low
- **Location:** Not specified in source audit
- **Status:** Protected under ADR-009
- **Description:** External threat audit identified no active vulnerability in this category.
- **Remediation Plan:** Preserve the existing ADR-009 service-integration protections.

### Finding 33: Terminal Log Forging & ANSI Injection

- **Severity:** Low
- **Location:** Not specified in source audit
- **Status:** Protected under ADR-009
- **Description:** External threat audit identified no active vulnerability in this category.
- **Remediation Plan:** Preserve existing protections and ensure future logging changes do not bypass them.

### Finding 34: Server-Side Request Forgery (SSRF) via Proxying

- **Severity:** Low
- **Location:** Not specified in source audit
- **Status:** Protected under ADR-009
- **Description:** External threat audit identified no active vulnerability in this category.
- **Remediation Plan:** Preserve the existing SSRF protections and revalidate them when proxy-target handling changes.

### Finding 35: Path Traversal via URL Normalization

- **Severity:** Low
- **Location:** Not specified in source audit
- **Status:** Protected under ADR-009
- **Description:** External threat audit identified no active vulnerability in this category.
- **Remediation Plan:** Preserve the existing path-normalization protections and include them in regression testing.

## 6. Component Verdicts

| Component | Verdict | Summary |
| :--- | :--- | :--- |
| UI Templates | PASS | UI templating was successfully decoupled and safely parameterized; no XSS vectors were detected in the reviewed implementation. |
| Context Routing | PASS | Context propagation and proxy target overrides were assessed as correct, with HTML boundaries respected. |
| Proxy Client | PASS | Context cancellation errors were properly swallowed and the reverse-proxy transport was cloned without drifting from `DefaultTransport`. |
| CLI / Testing | CONCERNS | CLI output issues were addressed, but the `version.go` output-stream changes introduced `os.Pipe` and global `os.Stdout` testing risks. |
| Network Edge | CONCERNS | IPv6 `net.SplitHostPort` edge cases and trailing-dot normalization inconsistencies were identified. |
| Repository | PASS | Configuration drift was removed from Git and formatting was standardized. |

The source audit also reports no destructive cross-component interactions and states that the separation of concerns between router and proxy remained intact.

## 7. Previously Reported Issues

The source audit records these earlier issues as resolved:

| Previously Reported Issue | Status |
| :--- | :--- |
| 404 Not Found HTML Page (1.1) | RESOLVED |
| 502 Bad Gateway Layout (1.2) | RESOLVED |
| Missing Custom Transport (2.1) | RESOLVED |
| Unbounded Error Log Map (2.2) | RESOLVED |
| Improper Handling of Cancellations (2.3) | RESOLVED |

The corresponding verification states that `ServeHTTP` delegates correctly to `renderErrorPage`, `word-break` mitigates layout shifts, `ResponseHeaderTimeout` is applied, the error map was replaced with `throttleCache`, and `context.Canceled` is handled gracefully.

## 8. Standards / ADR Compliance

- **Engineering Standards:** The test suite's use of `os.Pipe()` without concurrent draining was identified as a technical robustness violation despite working on the happy path.
- **ADR-002 (Non-recursive DNS):** Strict NXDOMAIN rules were assessed as maintained in the reviewed design.

## 9. Audit Status Summary

The audit record classifies findings by origin and action. Higher-severity architectural findings such as DNS panic recovery, reverse-proxy streaming timeouts, IPC socket lifecycle races, loopback binding, router encapsulation, NXDOMAIN handling, and daemon permission handling were marked resolved in `v2.0.0-beta.5`. Several after-effects from the earlier refactor were marked resolved in `v2.0.0-beta.4`, while a smaller set of architectural, logging, CLI, and initialization concerns remain deferred.

## 10. Final Audit Conclusion

**AUDIT HAS FINDINGS**

The reviewed DevTether implementation was assessed as highly resilient overall, but the audit identified remaining concerns around IPv6 host normalization, deferred signal and logging behavior, CLI error handling and determinism, DNS hot-path efficiency, and the proxy loop created by the generated fallback configuration. The source audit's final conclusion specifically highlights IPv6 normalization, `os.Pipe`-related test fragility, and proxy target-resolution redundancy as areas requiring attention to guarantee strict behavioral contracts.

## 11. Memory Audit (Ad-Hoc Request)

A targeted deep-dive audit was conducted to investigate a reported memory spike (2MB → 6MB) during active web browsing across multiple active and 404 routes.

**Findings:**
- **Zero active memory leaks detected.** The codebase strictly bounds memory usage across all vectors.
- **Proxy Cache Bounding:** The `throttleCache` in `handler.go` employs a hard limit of 1024 entries. Unbounded cache growth is mathematically impossible, even under a 404 DoS flood.
- **Template Recompilation:** Error pages are generated at compile-time using `//go:embed` and parsed exactly once in `init()`. Render time `Execute()` streams directly to the `ResponseWriter` without allocating large intermediary string buffers.
- **IPC Leaks:** `internal/daemon/client.go` properly utilizes `defer resp.Body.Close()`.
- **Router Targets:** `internal/router/router.go` performs deep copies of `url.URL` pointers (`u := *t.URL`), preventing reference-retention leaks.

**Conclusion (Normal Go GC Behavior):**
A memory footprint expanding from 2MB to 6MB under active load is expected standard behavior for the Go runtime. `httputil.ReverseProxy` pre-allocates 32KB I/O copy buffers per active connection, and the `http.Transport` pool keeps connections alive to backends. Additionally, Go's Garbage Collector (`GOGC=100`) allows the heap to double before sweeping. The 6MB plateau represents normal operating buffers and idle keep-alives, not a leak.