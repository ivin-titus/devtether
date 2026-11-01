// Package health records TCP-reachability observations for routed backends.
//
// Health is observational metadata only: it measures whether a backend TCP
// listener is reachable, never application-level health, and it never affects
// route registration or DNS answers. An HTTP 4xx/5xx response still proves
// reachability; a connection that is accepted and then closed, a response-
// header timeout, or client cancellation is inconclusive and does not mark a
// backend offline.
package health

import (
	"context"
	"errors"
	"net"
	"sort"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

// Status is the latest observed reachability state of a route's backend.
type Status string

const (
	// StatusOnline means a backend connection or response was observed.
	StatusOnline Status = "online"
	// StatusOffline means a confirmed dial-stage failure was observed.
	StatusOffline Status = "offline"
	// StatusUnknown means no conclusive observation exists yet.
	StatusUnknown Status = "unknown"
)

const (
	// probeTimeout bounds each individual TCP probe.
	probeTimeout = 200 * time.Millisecond
	// checkBudget bounds one complete check pass over all targets.
	checkBudget = 5 * time.Second
	// checkConcurrency bounds simultaneous probes within a check pass.
	checkConcurrency = 8
)

// Observation is a single health measurement for one route.
type Observation struct {
	Status    Status
	CheckedAt time.Time
	// Latency is the TCP connection latency of the probe or dial attempt;
	// zero when no dial timing was measured (e.g., proxy-traffic observations).
	Latency time.Duration
	// Reason is a short failure classification, set when Status is offline.
	Reason string
}

// Target identifies a backend to probe. Addr is a loopback host:port.
type Target struct {
	Domain string
	Addr   string
}

// CheckResult is the probe outcome for one target.
type CheckResult struct {
	Domain string
	Addr   string
	Observation
}

// Tracker holds the latest health observation per route domain.
//
// Ordering policy: every health-producing operation (a probe or a proxied
// request) allocates a monotonically increasing generation when it *begins*,
// via Next. Observations are recorded with that generation, and a write is
// applied only when its generation is newer than the stored one — so a
// slower operation that started earlier can never overwrite evidence from an
// operation that started later, regardless of completion order.
type Tracker struct {
	mu   sync.RWMutex
	seq  uint64
	gens map[string]uint64
	obs  map[string]Observation
}

// NewTracker returns an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{
		gens: make(map[string]uint64),
		obs:  make(map[string]Observation),
	}
}

// Next allocates the generation for an operation beginning now.
func (t *Tracker) Next() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	return t.seq
}

// Observe records o for domain when gen is newer than the generation of the
// stored observation. It returns true when the observation was applied.
func (t *Tracker) Observe(domain string, gen uint64, o Observation) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if gen <= t.gens[domain] {
		return false
	}
	t.gens[domain] = gen
	t.obs[domain] = o
	return true
}

// ObserveNow allocates a fresh generation and records o. Use it when the
// observation itself begins now (e.g., evidence obtained mid-request).
func (t *Tracker) ObserveNow(domain string, o Observation) bool {
	return t.Observe(domain, t.Next(), o)
}

// Snapshot returns the latest observation for each domain, sorted by domain.
// Domains with no observation report StatusUnknown.
func (t *Tracker) Snapshot(domains []string) []CheckResult {
	t.mu.RLock()
	defer t.mu.RUnlock()
	results := make([]CheckResult, 0, len(domains))
	for _, d := range domains {
		o, ok := t.obs[d]
		if !ok {
			o = Observation{Status: StatusUnknown}
		}
		results = append(results, CheckResult{Domain: d, Observation: o})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Domain < results[j].Domain })
	return results
}

// Probe performs one TCP reachability check against addr and returns the
// measurement. It never writes shared state. A dial that fails because ctx
// was cancelled or expired is reported as unknown, not offline.
func Probe(ctx context.Context, addr string) Observation {
	start := time.Now()
	dialer := net.Dialer{Timeout: probeTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		o := Observation{CheckedAt: time.Now(), Latency: time.Since(start)}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			o.Status = StatusUnknown // aborted probe is inconclusive
			return o
		}
		o.Status = StatusOffline
		o.Reason = dialReason(err)
		return o
	}
	_ = conn.Close()
	return Observation{Status: StatusOnline, CheckedAt: time.Now(), Latency: time.Since(start)}
}

// dialReason maps a dial-stage error to a short user-facing classification.
func dialReason(err error) string {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	default:
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return "dial timeout"
		}
		return "connect failed"
	}
}

// CheckAll probes every target once with bounded concurrency and a shared
// time budget, records each result under the generation allocated when its
// probe began, and returns one result per target in input order. Targets
// whose probes could not complete before the budget expired are returned as
// StatusUnknown and leave any earlier stored observation untouched.
func (t *Tracker) CheckAll(ctx context.Context, targets []Target) []CheckResult {
	return t.checkAll(ctx, targets, Probe)
}

// checkAll is CheckAll with an injectable probe function (test seam).
func (t *Tracker) checkAll(ctx context.Context, targets []Target, probe func(context.Context, string) Observation) []CheckResult {
	ctx, cancel := context.WithTimeout(ctx, checkBudget)
	defer cancel()

	results := make([]CheckResult, len(targets))
	done := make([]bool, len(targets))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(checkConcurrency)
	for i, tgt := range targets {
		g.Go(func() error {
			gen := t.Next()
			obs := probe(gctx, tgt.Addr)
			if gctx.Err() != nil && obs.Status == StatusUnknown {
				return nil // budget/cancel: keep prior observation
			}
			t.Observe(tgt.Domain, gen, obs)
			mu.Lock()
			results[i] = CheckResult{Domain: tgt.Domain, Addr: tgt.Addr, Observation: obs}
			done[i] = true
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	for i, tgt := range targets {
		if !done[i] {
			results[i] = CheckResult{
				Domain: tgt.Domain,
				Addr:   tgt.Addr,
				Observation: Observation{
					Status:    StatusUnknown,
					CheckedAt: time.Now(),
				},
			}
		}
	}
	return results
}
