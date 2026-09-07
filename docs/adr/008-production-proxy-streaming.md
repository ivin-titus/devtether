# ADR 008: Production-Grade Streaming & Context Integrity

## Status
Accepted

## Context
During early iterations of the reverse proxy (Phases 1-7), HTTP middleware (like custom `ErrorHandler` and logging) failed to properly track the lifecycle of the underlying `http.ResponseWriter`.
Specifically, if a backend streamed a chunked response but crashed mid-stream:
1. The proxy caught the `io.EOF`.
2. Our custom `ErrorHandler` attempted to render an HTML 502 page and invoked `w.WriteHeader(502)`.
3. Because the middleware blindly allowed this, it injected a 502 status and a raw HTML payload directly into the middle of the client's broken binary or JSON stream, corrupting the payload entirely.

Additionally, routing logic occurred redundantly. The target was resolved in `ServeHTTP` and again in the `httputil.ReverseProxy.Rewrite` hook, introducing a Time-Of-Check to Time-Of-Use (TOCTOU) risk where a volatile route could disappear between the two steps.

## Decision
DevTether must operate as a production-grade reverse proxy capable of seamlessly supporting modern continuous-connection protocols (e.g., Server-Sent Events / SSE, WebSockets, GraphQL subscriptions, and long-polling). To ensure this:

1. **State Tracking:** Any HTTP middleware that intercepts or mutates a `http.ResponseWriter` MUST track whether headers have been sent via a `wroteHeader` boolean flag.
2. **Abort, Don't Corrupt:** If a downstream error is detected *after* headers were already sent to the client, the middleware MUST NOT attempt to write fallback headers or error bodies. The correct action is to `panic(http.ErrAbortHandler)` to cleanly abort the TCP connection without a noisy standard library stack trace.
3. **Context Isolation:** Routing resolution MUST only occur once per request. The resolved `Target` must be injected into the `context.Context` during `ServeHTTP`, and all downstream hooks (including `Rewrite`) must extract it from the context rather than re-resolving the host.

## Consequences
- **Positive:** Guarantees data integrity for all streaming endpoints. Completely prevents payload corruption.
- **Positive:** Prepares the routing engine for Engine 2 (volatile runtime routes) by eliminating TOCTOU data races.
- **Negative:** Middleware authors must be vigilant about wrapping `ResponseWriter` correctly.
