# Hybrid TTL + Random Eviction (Deferred Scope)

## Discussion: Log Eviction Strategy

**Date:** 09/19/2026
**Context:** During the Subphase 4.6 Architectural Check, the `devtether-audit` agent flagged an overly-hyped comment in `internal/proxy/handler.go` that claimed a "hybrid TTL + random eviction strategy" was being used to stay within the log cache limit.

**Reality:** The actual implementation is a pure random eviction strategy. When the map size hits `logCacheLimit`, it simply evicts the first key returned by a `range` loop. There is no TTL sweeping or expiration-based pruning.

**Decision:** We temporarily opted for the simplest fix (YAGNI): updating the code comment to accurately reflect the pure random eviction approach. Since the log cache map only stores a boolean flag per target URL, a 10,000-entry map takes negligible memory. Building a true TTL sweep for this is classic overengineering.

However, if DevTether ever needs to scale to tens of thousands of dynamic targets (e.g. wildcard subdomains generated programmatically), a true TTL eviction strategy might be needed to avoid unbounded memory growth if the `logCacheLimit` is raised or disabled.

## The Ideal Implementation: True TTL Eviction

To implement a true TTL eviction strategy without blocking the critical path (proxy request routing):

1. **Struct Update**: Change the cache from `map[string]bool` to `map[string]time.Time`, recording the exact timestamp when each URL was logged.
2. **Background Sweeper**: Launch a background goroutine during daemon startup that runs every `N` minutes (`time.Ticker`).
3. **Mutex Locking**: During the sweep, acquire the `sync.Mutex` on the cache, iterate over all entries, and delete any entries where `time.Since(entryTime) > cooldownPeriod`.
4. **Capacity Guard**: Retain the random eviction check during `Write()` as a strict memory circuit breaker if the cache fills up before the sweeper runs.

This guarantees memory scales down cleanly during idle periods, while ensuring O(1) latency on the request path.
