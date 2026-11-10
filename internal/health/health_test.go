package health

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ephemeralListener binds an ephemeral loopback TCP port and returns its
// address; the listener is closed at test cleanup.
func ephemeralListener(t *testing.T) string {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().String()
}

// refusedAddr returns a loopback address whose listener was already closed,
// so dialing it yields ECONNREFUSED.
func refusedAddr(t *testing.T) string {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return addr
}

func TestProbeOnline(t *testing.T) {
	obs := Probe(context.Background(), ephemeralListener(t))
	if obs.Status != StatusOnline {
		t.Fatalf("expected online, got %s", obs.Status)
	}
	if obs.Latency <= 0 {
		t.Fatalf("expected positive latency, got %s", obs.Latency)
	}
}

func TestProbeConnectionRefused(t *testing.T) {
	obs := Probe(context.Background(), refusedAddr(t))
	if obs.Status != StatusOffline {
		t.Fatalf("expected offline, got %s", obs.Status)
	}
	if obs.Reason != "connection refused" {
		t.Fatalf("expected 'connection refused', got %q", obs.Reason)
	}
}

func TestProbeDialTimeout(t *testing.T) {
	// A context deadline shorter than the probe timeout makes the dial abort;
	// a ctx-expired probe is inconclusive, not offline.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // let the deadline pass
	obs := Probe(ctx, ephemeralListener(t))
	if obs.Status != StatusUnknown {
		t.Fatalf("expected unknown on expired ctx, got %s", obs.Status)
	}
}

func TestProbeContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	obs := Probe(ctx, ephemeralListener(t))
	if obs.Status != StatusUnknown {
		t.Fatalf("expected unknown on cancelled ctx, got %s", obs.Status)
	}
}

func TestDialReason(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "connection refused",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			},
			want: "connection refused",
		},
		{
			name: "timeout",
			err: &net.OpError{
				Op:     "dial",
				Net:    "tcp",
				Err:    fmt.Errorf("i/o timeout"),
				Source: nil,
			},
			want: "connect failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dialReason(tt.err); got != tt.want {
				t.Fatalf("dialReason: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDialReasonTimeout(t *testing.T) {
	err := &net.OpError{Op: "dial", Net: "tcp", Err: &timeoutError{}}
	if got := dialReason(err); got != "dial timeout" {
		t.Fatalf("expected 'dial timeout', got %q", got)
	}
}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return false }

func TestObserveOrdering(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 1)

	old := tr.Next()
	newer := tr.Next()

	if !tr.Observe("app.localhost", 1, newer, Observation{Status: StatusOnline}) {
		t.Fatal("newer observation should be applied")
	}
	if tr.Observe("app.localhost", 1, old, Observation{Status: StatusOffline}) {
		t.Fatal("older-generation observation must not overwrite a newer one")
	}

	got := tr.Snapshot([]string{"app.localhost"})[0].Status
	if got != StatusOnline {
		t.Fatalf("expected online after stale-write rejection, got %s", got)
	}
}

func TestSnapshotUnknownAndSorted(t *testing.T) {
	tr := NewTracker()
	tr.Register("b.localhost", 1)
	tr.ObserveNow("b.localhost", 1, Observation{Status: StatusOnline})

	snap := tr.Snapshot([]string{"z.localhost", "a.localhost", "b.localhost"})
	if len(snap) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(snap))
	}
	if snap[0].Domain != "a.localhost" || snap[1].Domain != "b.localhost" || snap[2].Domain != "z.localhost" {
		t.Fatalf("snapshot not sorted: %+v", snap)
	}
	if snap[0].Status != StatusUnknown {
		t.Fatalf("unchecked route must report unknown, got %s", snap[0].Status)
	}
	if snap[1].Status != StatusOnline {
		t.Fatalf("expected online, got %s", snap[1].Status)
	}
}

func TestConcurrentObserve(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 1)
	tr.Register("api.localhost", 1)
	const workers = 16
	const writes = 50

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < writes; i++ {
				tr.ObserveNow("app.localhost", 1, Observation{Status: StatusOnline})
				tr.ObserveNow("api.localhost", 1, Observation{Status: StatusOffline})
			}
		}()
	}
	wg.Wait()

	snap := tr.Snapshot([]string{"app.localhost", "api.localhost"})
	if snap[0].Status != StatusOffline || snap[1].Status != StatusOnline {
		t.Fatalf("unexpected final state: %+v", snap)
	}
}

func TestCheckAll(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 1)
	tr.Register("api.localhost", 1)
	online := ephemeralListener(t)
	offline := refusedAddr(t)

	results := tr.CheckAll(context.Background(), []Target{
		{Domain: "app.localhost", Addr: online, Incarnation: 1},
		{Domain: "api.localhost", Addr: offline, Incarnation: 1},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	byDomain := map[string]CheckResult{}
	for _, r := range results {
		byDomain[r.Domain] = r
	}
	if byDomain["app.localhost"].Status != StatusOnline {
		t.Fatalf("app: expected online, got %s", byDomain["app.localhost"].Status)
	}
	if byDomain["api.localhost"].Status != StatusOffline {
		t.Fatalf("api: expected offline, got %s", byDomain["api.localhost"].Status)
	}
	if byDomain["api.localhost"].Reason == "" {
		t.Fatal("api: expected a failure reason")
	}

	// Results were also written into tracker state.
	snap := tr.Snapshot([]string{"app.localhost", "api.localhost"})
	if snap[1].Status != StatusOnline || snap[0].Status != StatusOffline {
		t.Fatalf("tracker state not updated: %+v", snap)
	}
}

func TestCheckAllCancelledKeepsUnknown(t *testing.T) {
	tr := NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := tr.CheckAll(ctx, []Target{{Domain: "app.localhost", Addr: ephemeralListener(t)}})
	if len(results) != 1 || results[0].Status != StatusUnknown {
		t.Fatalf("expected unknown on cancelled check, got %+v", results)
	}
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusUnknown {
		t.Fatalf("cancelled probe must not write offline, got %s", got)
	}
}

// TestCheckAllStaleProbeCannotClobber covers the ordering guarantee
// deterministically: a probe that began *before* a newer observation
// (smaller generation) cannot overwrite it even though it completes later.
// Ordering is controlled with a probe seam that blocks until released —
// no timing or sleeps.
func TestCheckAllStaleProbeCannotClobber(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 1)

	started := make(chan struct{})
	release := make(chan struct{})
	probe := func(context.Context, string) Observation {
		close(started)
		<-release
		return Observation{Status: StatusOffline, CheckedAt: time.Now()}
	}

	// Startup probe begins: gen N allocated at probe start, then blocks.
	done := make(chan []CheckResult, 1)
	go func() {
		done <- tr.checkAll(context.Background(),
			[]Target{{Domain: "app.localhost", Addr: "127.0.0.1:1", Incarnation: 1}}, probe)
	}()
	<-started

	// A later proxy observation begins (gen N+1) and completes first.
	gen := tr.Next()
	tr.Observe("app.localhost", 1, gen, Observation{Status: StatusOnline, CheckedAt: time.Now()})

	// The older startup probe completes later with an offline result.
	close(release)
	<-done

	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusOnline {
		t.Fatalf("stale probe must not overwrite newer observation, got %s", got)
	}
}

func TestNextMonotonic(t *testing.T) {
	tr := NewTracker()
	a, b := tr.Next(), tr.Next()
	if b <= a {
		t.Fatalf("generations must increase: %d then %d", a, b)
	}
}

// TestRegisterRejectsZeroDuplicateAndStale pins the high-water-mark rule:
// only an incarnation above hi ever becomes live.
func TestRegisterRejectsZeroDuplicateAndStale(t *testing.T) {
	tr := NewTracker()

	tr.Register("app.localhost", 0)
	if _, ok := tr.cur["app.localhost"]; ok {
		t.Fatal("zero incarnation must never become live")
	}

	tr.Register("app.localhost", 5)
	tr.Register("app.localhost", 5) // duplicate
	tr.Register("app.localhost", 4) // stale
	if got := tr.cur["app.localhost"]; got != 5 {
		t.Fatalf("cur = %d, want 5", got)
	}
	if got := tr.hi["app.localhost"]; got != 5 {
		t.Fatalf("hi = %d, want 5", got)
	}
}

// TestUnregisterTombstoneBlocksReregister: removing a route tombstones its
// incarnation so a delayed Register can never resurrect it.
func TestUnregisterTombstoneBlocksReregister(t *testing.T) {
	tr := NewTracker()

	tr.Unregister("app.localhost", 10) // route removed before ever registering
	tr.Register("app.localhost", 10)   // delayed register of the removed incarnation
	if _, ok := tr.cur["app.localhost"]; ok {
		t.Fatal("removed incarnation must not resurrect")
	}

	tr.Register("app.localhost", 11) // genuine replacement
	if got := tr.cur["app.localhost"]; got != 11 {
		t.Fatalf("cur = %d, want 11", got)
	}
	if got := tr.hi["app.localhost"]; got != 11 {
		t.Fatalf("hi = %d, want 11", got)
	}
}

// TestInterleavingA: a stale Unregister arriving after remove→re-add must not
// clear the replacement's live incarnation or health.
func TestInterleavingA(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 11)
	tr.ObserveNow("app.localhost", 11, Observation{Status: StatusOnline})

	// RemoveRoute(d, 10) → AddRoute → Register(11) already happened; now a
	// delayed Unregister(10) arrives.
	tr.Unregister("app.localhost", 10)

	if got := tr.cur["app.localhost"]; got != 11 {
		t.Fatalf("stale unregister cleared cur: got %d, want 11", got)
	}
	if tr.Observe("app.localhost", 10, tr.Next(), Observation{Status: StatusOffline}) {
		t.Fatal("old-incarnation observation must be rejected")
	}
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusOnline {
		t.Fatalf("replacement health must survive, got %s", got)
	}
}

// TestInterleavingB: a delayed Register for a removed incarnation arriving
// after the replacement registered must not displace it.
func TestInterleavingB(t *testing.T) {
	tr := NewTracker()

	// AddRoute (never registered) → RemoveRoute(d, 10) → Unregister(10) →
	// AddRoute → Register(11) → delayed Register(10).
	tr.Unregister("app.localhost", 10)
	tr.Register("app.localhost", 11)
	tr.Register("app.localhost", 10)

	if got := tr.cur["app.localhost"]; got != 11 {
		t.Fatalf("delayed register displaced cur: got %d, want 11", got)
	}
	if got := tr.hi["app.localhost"]; got != 11 {
		t.Fatalf("hi = %d, want 11", got)
	}
}

// TestZeroIncarnationRejected: zero incarnations are rejected by every
// operation and mutate nothing.
func TestZeroIncarnationRejected(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 3)

	if tr.Observe("app.localhost", 0, tr.Next(), Observation{Status: StatusOnline}) {
		t.Fatal("zero-incarnation Observe must be rejected")
	}
	if tr.ObserveNow("app.localhost", 0, Observation{Status: StatusOnline}) {
		t.Fatal("zero-incarnation ObserveNow must be rejected")
	}
	tr.Unregister("app.localhost", 0)
	tr.Register("app.localhost", 0)

	if got := tr.cur["app.localhost"]; got != 3 {
		t.Fatalf("cur = %d, want 3 (zero-incarnation ops must be no-ops)", got)
	}
	if got := tr.hi["app.localhost"]; got != 3 {
		t.Fatalf("hi = %d, want 3", got)
	}
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusUnknown {
		t.Fatalf("zero-incarnation ops must not write health, got %s", got)
	}
}

// TestUnregisterClearsOnlyMatchingIncarnation: a mismatched Unregister
// tombstones hi but leaves the live incarnation and its health intact; a
// matching one clears them.
func TestUnregisterClearsOnlyMatchingIncarnation(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 7)
	tr.ObserveNow("app.localhost", 7, Observation{Status: StatusOnline})

	tr.Unregister("app.localhost", 8) // mismatch
	if got := tr.cur["app.localhost"]; got != 7 {
		t.Fatalf("mismatched unregister cleared cur: got %d, want 7", got)
	}
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusOnline {
		t.Fatalf("mismatched unregister cleared health, got %s", got)
	}

	tr.Unregister("app.localhost", 7) // match
	if _, ok := tr.cur["app.localhost"]; ok {
		t.Fatal("matching unregister must clear cur")
	}
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusUnknown {
		t.Fatalf("matching unregister must clear health, got %s", got)
	}
}

// TestConcurrentUnregisterObserve races observations against an unregister;
// whatever the interleaving, the final state is consistent and the race
// detector validates locking.
func TestConcurrentUnregisterObserve(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 1)

	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				tr.ObserveNow("app.localhost", 1, Observation{Status: StatusOnline})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		tr.Unregister("app.localhost", 1)
	}()
	wg.Wait()

	// Whether observations landed before the unregister or were rejected
	// after it, the unregister clears them: the final state is unknown.
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusUnknown {
		t.Fatalf("expected unknown after unregister, got %s", got)
	}
	if _, ok := tr.cur["app.localhost"]; ok {
		t.Fatal("cur must be absent after unregister")
	}
}

// TestConcurrentRegisterHighestIncarnationWins launches concurrent Register
// calls for incarnations 1..8 (plus one duplicate) released together. The
// highest incarnation must win regardless of arrival order: anything not
// strictly greater than the high-water mark is rejected.
func TestConcurrentRegisterHighestIncarnationWins(t *testing.T) {
	tr := NewTracker()

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := uint64(1); i <= 8; i++ {
		inc := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tr.Register("app.localhost", inc)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		tr.Register("app.localhost", 3) // duplicate of an in-flight incarnation
	}()
	close(start)
	wg.Wait()

	if got := tr.hi["app.localhost"]; got != 8 {
		t.Fatalf("hi = %d, want 8", got)
	}
	if got := tr.cur["app.localhost"]; got != 8 {
		t.Fatalf("cur = %d, want 8", got)
	}

	// The winning incarnation records observations; losers do not.
	tr.Observe("app.localhost", 7, tr.Next(), Observation{Status: StatusOffline, CheckedAt: time.Now()})
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusUnknown {
		t.Fatalf("stale incarnation observation must be rejected, got %s", got)
	}
	tr.Observe("app.localhost", 8, tr.Next(), Observation{Status: StatusOnline, CheckedAt: time.Now()})
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusOnline {
		t.Fatalf("winning incarnation must record online, got %s", got)
	}
}

// TestCheckAllProbeAfterReplacementRejected: a probe that began against the
// old incarnation and completes after remove→re-add must not overwrite the
// replacement's health. Ordering is channel-gated — no sleeps.
func TestCheckAllProbeAfterReplacementRejected(t *testing.T) {
	tr := NewTracker()
	tr.Register("app.localhost", 1)

	started := make(chan struct{})
	release := make(chan struct{})
	probe := func(context.Context, string) Observation {
		close(started)
		<-release
		return Observation{Status: StatusOffline, CheckedAt: time.Now()}
	}

	done := make(chan []CheckResult, 1)
	go func() {
		done <- tr.checkAll(context.Background(),
			[]Target{{Domain: "app.localhost", Addr: "127.0.0.1:1", Incarnation: 1}}, probe)
	}()
	<-started

	// The route is replaced while the probe is in flight.
	tr.Unregister("app.localhost", 1)
	tr.Register("app.localhost", 2)
	tr.Observe("app.localhost", 2, tr.Next(), Observation{Status: StatusOnline, CheckedAt: time.Now()})

	close(release)
	<-done

	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusOnline {
		t.Fatalf("old-incarnation probe must not overwrite replacement health, got %s", got)
	}
}

// TestCheckAllUnregisteredIncarnationLeavesStateUntouched is the
// wire-compatibility lock: targets without a registered incarnation still
// return correct per-probe results (routes --check contract) but never
// write tracker state.
func TestCheckAllUnregisteredIncarnationLeavesStateUntouched(t *testing.T) {
	tr := NewTracker()

	results := tr.CheckAll(context.Background(), []Target{
		{Domain: "app.localhost", Addr: ephemeralListener(t)}, // Incarnation 0
	})
	if len(results) != 1 || results[0].Status != StatusOnline {
		t.Fatalf("wire result must stay correct, got %+v", results)
	}
	if got := tr.Snapshot([]string{"app.localhost"})[0].Status; got != StatusUnknown {
		t.Fatalf("unregistered target must not write tracker state, got %s", got)
	}
}
