# Webhook Traffic Inspection

**Status:** Deferred to a future release.

## Context & Vision
A significant pain point in local development is debugging incoming webhooks from third-party services (e.g., Stripe, GitHub, Twilio). Currently, developers have to tail the raw HTTP proxy logs and piece together the request payload manually.

We envision a native **Webhook Traffic Inspection** feature in DevTether. This would allow developers to stream, replay, and inspect incoming requests and payloads in real-time, completely locally.

### Technical Implementation Ideas
- **Memory Buffering:** Use `sync.Pool` to efficiently buffer recent traffic payloads in memory (with a strict rolling cap to prevent OOM errors).
- **SSE Endpoint:** Serve the buffered traffic via a Server-Sent Events (SSE) endpoint at `/api/stream`.
- **UI Integration:** This feature naturally pairs with the proposed Stateless Web GUI, allowing developers to view the webhook payloads in a clean, graphical interface.

## Why Deferred?
Per the "No AI Slop / Engine 1 Focus" mandate (`docs/engineering-standards.md`), we are intentionally limiting the scope of the current release to the core networking layer. Buffering HTTP bodies and serving SSE streams adds significant memory and complexity overhead. This will be implemented only when the core routing engine is completely rock-solid.
