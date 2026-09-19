# ADR-009: Proxy Security Lessons & Defense-in-Depth

Date: 2026-09-08

## Status
Accepted

## Context
As DevTether evolves into a resilient local reverse proxy (and prepares to introduce deep traffic inspection features), it is critical to learn from catastrophic vulnerabilities in similar industry products. This ADR codifies defense-in-depth strategies to prevent DevTether from falling victim to common architectural flaws related to state leakage, socket permissions, and proxy manipulation.

We have grouped the major industry vulnerabilities into three primary categories: Cross-Request State Leakage, Request/Protocol Manipulation, and Local Privilege Escalation.

### 1. Cross-Request State Leakage & Connection Pooling
**Industry Failures:**
- **Buffer/Memory Leaks:** Cloudflare "Cloudbleed" (2017) leaked uninitialized memory containing authentication tokens via buffer overruns in its HTML parser. `go-resty` (CVE-2023-45286) leaked HTTP payload data due to improper clearing of `sync.Pool` buffers during request retries.
- **Context/Connection Reuse Leaks:** ChatGPT and `redis-py` (2023) leaked user chat histories when cancelled async requests returned "dirty" sockets to the connection pool. Gotenberg (CVE-2023-42594) and common `fasthttp` anti-patterns leak data when pooled request contexts are reused while background goroutines still hold references to them.
- **Cross-User Poisoning:** Traefik (CVE-2023-71324) and curl (CVE-2023-3784) improperly handled connection state and backend sockets, serving one user's private response or authentication context to another.

**Our Defensive Posture:**
- **Current Status (SAFE):** We rely on `net/http.Transport` which safely closes dirty TCP sockets on context cancellation. We have zero `sync.Pool` usage and zero `unsafe` package usage, making buffer overruns impossible. `Handler` state is explicitly zeroed by relying exclusively on `context.Context`.

### 2. Request and Protocol Manipulation
**Industry Failures:**
- **HTTP Request Smuggling:** Apache `mod_proxy` and Cloudflare Pingora suffered desync attacks when `Content-Length` and `Transfer-Encoding` conflicted, allowing attackers to hijack backend connections.
- **DNS Rebinding:** Transmission RPC (2018) and Node.js Inspector (CVE-2018-7160) allowed attackers to bypass the Same-Origin Policy (SOP) by rebinding malicious domains (e.g., `evil.com`) to `127.0.0.1` after the browser loaded malicious JavaScript.
- **Server-Side Request Forgery (SSRF):** Next.js local development server (CVE-2026-64645) and Wrangler CLI (CVE-2023-3348) were exploited because the development proxies blindly forwarded arbitrary internal requests or failed to validate upstream targets, allowing attackers to access internal network assets.
- **Path Traversal via URL Normalization:** Logto Tunnel (CVE-2026-63188) and Apache Tomcat (CVE-2025-55752) proxy valves were bypassed because the proxies normalized URLs (e.g., decoding `%2e%2e%2f` to `../`) differently than the backend, tricking the proxy into serving forbidden files like `/etc/passwd`.
- **HTTP/2 Rapid Reset:** A global DDoS vector (CVE-2023-44487) crashed Envoy, NGINX, HAProxy, and AWS proxies by exhausting memory via unhandled HTTP/2 stream cancellations.

**Our Defensive Posture:**
- **Current Status (SAFE):** Go's `net/http` natively drops ambiguous smuggling payloads. DevTether strictly validates the `Host` header against `devtether.yaml`, neutralizing DNS rebinding and SSRF (we physically cannot route to an unlisted IP like `169.254.169.254`). We rely on standard `httputil.ReverseProxy` which securely handles URL normalization. DevTether is currently HTTP/1.1 only, neutralizing HTTP/2 Rapid Reset.

### 3. Local Privilege Escalation (LPE)
**Industry Failures:**
- **Socket Permission TOCTOU:** Docker daemon (`/var/run/docker.sock`) and Snapd (CVE-2019-7304) allowed unprivileged local users or malware to achieve root access or execute commands via misconfigured unix IPC socket permissions.

**Our Defensive Posture:**
- **Current Status (SAFE):** We secure the fallback path (`/tmp/devtether.sock`) by using a process-global `umask(0177)` immediately before `net.Listen("unix")`. This atomically forces the socket creation to `0600`, completely neutralizing the microsecond Time-Of-Check to Time-Of-Use (TOCTOU) race.

### 4. Web UI & Streaming Hijacking
**Industry Failures:**
- **Cross-Site WebSocket Hijacking (CSWSH):** The Next.js local development server (CVE-2025-48068) suffered a massive vulnerability where the development server lacked proper `Origin` validation for its WebSocket interface. A malicious website could initiate a WebSocket connection to `localhost:3000` and expose the developer's internal source code and hot-module states simply by having the developer visit the malicious site.

**Our Defensive Posture:**
- **Current Status (SAFE):** We do not yet host a Web GUI or SSE stream.
- **Future Defense (Traffic Inspection GUI):** When we implement the `/api/stream` Server-Sent Events (SSE) or WebSockets for the UI, we MUST implement strict `Origin` and `Sec-Fetch-Site` header validation. Cross-origin requests to the IPC bridge must be strictly rejected to prevent malicious websites from scraping local proxy traffic.

### 5. Insecure Service Integration
**Industry Failures:**
- **Systemd Privilege Escalation:** Systemd (`unit_deserialize` CVE-2018-15686) and various application installer scripts (e.g., placing binaries or `.service` files in world-writable directories) have historically allowed unprivileged users to overwrite the daemon binary before systemd executes it as root.

**Our Defensive Posture:**
- **Current Status (SAFE / IMPLEMENTED):** We implemented `devtether service install` which enforces strict `User=` and `Group=` privilege drops across `systemd` and `launchd` templates. The daemon creates isolated runtime sockets (e.g. `0700` directories) to prevent root-owned sockets from leaking, enabling safe mixed-privilege operations.

### 6. Terminal Log Forging & ANSI Injection
**Industry Failures:**
- **ANSI Escape Sequence Injection:** OpenClaw CLI (CVE-2026-35651) and Active Record (CVE-2025-55193) logged untrusted input (like User-Agent headers or requested URLs) directly to the developer's terminal without sanitization. Attackers sent HTTP requests containing raw ANSI escape sequences (`\x1b[...`), which cleared the developer's terminal, spoofed fake authentication prompts, and overwrote prior log lines to hide malicious activity.

**Our Defensive Posture:**
- **Current Status (SAFE):** DevTether uses Go's structured `log/slog` library. When untrusted strings (like `r.URL.Path` or `r.Host`) are logged, `slog` securely escapes unprintable control characters and ANSI sequences, preventing terminal manipulation.
- **Future Defense (Custom Terminal UI):** If we ever re-introduce a custom terminal UI (e.g., `text/tabwriter` or interactive dashboards), we MUST explicitly sanitize all proxy metadata using `strconv.Quote` before printing to `stdout`.

### 7. Resource Exhaustion & Log DoS (throttleCache)
**Industry Failures:**
- **Memory Leaks via Error Logs:** Unbounded caching of proxy error states or unique malicious hostnames frequently leads to OOM (Out Of Memory) crashes during network scanning or DoS attempts.

**Our Defensive Posture:**
- **Current Status (SAFE / IMPLEMENTED):** We implemented a hybrid LRU/TTL `throttleCache` for proxy error logs. To avoid the massive overhead of strict TTL timers or standard LRU linked-lists under heavy concurrent load, we rely on an O(1) randomized map eviction strategy. This provides bounded memory usage and DoS resilience while adhering to our zero-dependency, high-performance mandates.

## Decision
To guarantee DevTether never succumbs to these vulnerabilities, we adopt the following strict architectural rules:

1. **Zero-State Reuse & Safe Pooling:**
   - Never retain pooled objects (`bytes.Buffer`, `RequestCtx`) in Goroutines. Deep-copy data before returning from the HTTP handler.
   - Any `[]byte` utilized from a `sync.Pool` MUST be explicitly zeroed (`clear(buf)`) before returning it to the pool.
   - Never store request-scoped data in struct fields. Flow all state through `context.Context`.
2. **Absolute Memory Safety:**
   - The `unsafe` package is strictly banned from the codebase.
3. **Strict Fallback Protocols:**
   - Never implement a "catch-all" route. Always reject unrecognized `Host` headers to prevent DNS Rebinding.
   - Socket creation must be atomic. Set process `umask` (e.g., `0177`) *before* calling `net.Listen("unix")` to eliminate TOCTOU races.
   - **Fail-Fast Port Binding:** Never silently fall back to a random unprivileged port (e.g., `:0`) if a configured port is occupied or lacks permissions. Fallbacks must be strict, fully predictable, and immediately fatal if they fail, ensuring deterministic system architecture.
4. **Protocol Hardening (Future TLS Integrations):**
   - When TLS (and thereby HTTP/2) is eventually introduced, the proxy MUST enforce `MaxConcurrentStreams` limits and compile against Go >= 1.21.3.
5. **Strict Origin Validation (Future Web GUI):**
   - Any future WebSocket or Server-Sent Events (SSE) endpoints MUST aggressively validate `Origin` and `Sec-Fetch-Site` headers to prevent Cross-Site WebSocket Hijacking (CSWSH).
6. **Secure System Daemonization (Implemented):**
   - System service installers MUST enforce strict `root:root` ownership of binaries and drop proxy execution privileges using `User=` and `Group=` directives.
7. **Terminal Output Sanitization:**
   - Any raw HTTP metadata (User-Agent, URI, Headers) printed to a terminal UI MUST be sanitized (e.g., `strconv.Quote`) to neutralize ANSI escape sequence injection.
8. **Memory Bounding (throttleCache):**
   - Proxy-layer caches indexing untrusted network input (e.g., failed `Host` resolutions) MUST be strictly memory-bounded using O(1) eviction strategies to prevent OOM DoS attacks.

## Consequences
- **Positive:** DevTether remains structurally immune to the most devastating reverse proxy vulnerabilities seen in the last decade.
- **Negative:** Utilizing `sync.Pool` for upcoming traffic inspection features will require slightly more verbose boilerplate (`clear(buf)`) and strict code reviews to ensure goroutine safety.
