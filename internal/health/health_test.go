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

	old := tr.Next()
	newer := tr.Next()

	if !tr.Observe("app.localhost", newer, Observation{Status: StatusOnline}) {
		t.Fatal("newer observation should be applied")
	}
	if tr.Observe("app.localhost", old, Observation{Status: StatusOffline}) {
		t.Fatal("older-generation observation must not overwrite a newer one")
	}

	got := tr.Snapshot([]string{"app.localhost"})[0].Status
	if got != StatusOnline {
		t.Fatalf("expected online after stale-write rejection, got %s", got)
	}
}

func TestSnapshotUnknownAndSorted(t *testing.T) {
	tr := NewTracker()
	tr.ObserveNow("b.localhost", Observation{Status: StatusOnline})

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
	const workers = 16
	const writes = 50

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < writes; i++ {
				tr.ObserveNow("app.localhost", Observation{Status: StatusOnline})
				tr.ObserveNow("api.localhost", Observation{Status: StatusOffline})
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
	online := ephemeralListener(t)
	offline := refusedAddr(t)

	results := tr.CheckAll(context.Background(), []Target{
		{Domain: "app.localhost", Addr: online},
		{Domain: "api.localhost", Addr: offline},
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
			[]Target{{Domain: "app.localhost", Addr: "127.0.0.1:1"}}, probe)
	}()
	<-started

	// A later proxy observation begins (gen N+1) and completes first.
	gen := tr.Next()
	tr.Observe("app.localhost", gen, Observation{Status: StatusOnline, CheckedAt: time.Now()})

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
