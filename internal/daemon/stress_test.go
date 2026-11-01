package daemon

// Temporary stress/regression harness for the event-driven health rework.
// Exercises the real daemon IPC socket + real proxy traffic concurrently.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ivin-titus/devtether/internal/health"
	"github.com/ivin-titus/devtether/internal/proxy"
	"github.com/ivin-titus/devtether/internal/router"
)

func newStack(t *testing.T, routes map[string]int) (context.CancelFunc, *health.Tracker, string) {
	t.Helper()
	// macOS sun_path limit (~104 chars) rules out t.TempDir() paths.
	dir, err := os.MkdirTemp("/tmp", "dt")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	e := router.NewEngine()
	for d, p := range routes {
		if addErr := e.AddRoute(d, "svc", p, router.RouteStatic); addErr != nil {
			t.Fatal(addErr)
		}
	}
	tr := health.NewTracker()
	ctx, cancel := context.WithCancel(context.Background())

	srv := NewServer(e, tr, cancel, "devtether.yaml")
	ln, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("daemon Listen: %v", err)
	}
	go func() { _ = srv.Serve(ctx, ln) }()

	pSrv := &http.Server{
		Handler:           proxy.LoggingMiddleware(proxy.NewHandler(e, tr)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	pl, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}
	go func() { _ = pSrv.Serve(pl) }()
	t.Cleanup(func() {
		_ = pSrv.Shutdown(context.Background())
		_ = srv.httpServer.Shutdown(context.Background())
	})
	return cancel, tr, "http://" + pl.Addr().String()
}

func backendPort(t *testing.T, h http.HandlerFunc) int {
	t.Helper()
	b := httptest.NewServer(h)
	t.Cleanup(b.Close)
	return b.Listener.Addr().(*net.TCPAddr).Port
}

func deadPort(t *testing.T) int {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return p
}

func proxyReq(t *testing.T, ctx context.Context, base, host, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy %s: %v", host, err)
	}
	return resp
}

func statusOf(t *testing.T, tr *health.Tracker, domain string) health.Status {
	t.Helper()
	return tr.Snapshot([]string{domain})[0].Status
}

func TestStressHealthUnderLoad(t *testing.T) {
	good := backendPort(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	fiveHundred := backendPort(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	hanging := backendPort(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		if f, ok := w.(http.Flusher); ok {
			_, _ = fmt.Fprint(w, "chunk")
			f.Flush()
		}
		<-r.Context().Done()
	})
	streaming := backendPort(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		if f, ok := w.(http.Flusher); ok {
			_, _ = fmt.Fprint(w, "data: part\n\n")
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	})
	dead := deadPort(t)

	cancel, tr, proxyBase := newStack(t, map[string]int{
		"good.localhost":      good,
		"fiveoh.localhost":    fiveHundred,
		"hanging.localhost":   hanging,
		"stream.localhost":    streaming,
		"dead.localhost":      dead,
		"untouched.localhost": deadPort(t),
	})
	defer cancel()

	ctx := context.Background()
	client := NewClient()

	// 1. Concurrent load against every route class.
	var wg sync.WaitGroup
	fail := func(format string, a ...any) { t.Errorf(format, a...) }

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := proxyReq(t, ctx, proxyBase, "good.localhost", "/")
			if r.StatusCode != 200 {
				fail("good: %d", r.StatusCode)
			}
			_ = r.Body.Close()
		}()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := proxyReq(t, ctx, proxyBase, "dead.localhost", "/")
			if r.StatusCode != 502 {
				fail("dead: expected 502 got %d", r.StatusCode)
			}
			_ = r.Body.Close()
		}()
	}
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := proxyReq(t, ctx, proxyBase, "fiveoh.localhost", "/")
			if r.StatusCode != 500 {
				fail("500: %d", r.StatusCode)
			}
			_ = r.Body.Close()
		}()
	}

	// 2. Client cancellations mid-stream (must never mark offline).
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, cc := context.WithTimeout(ctx, 40*time.Millisecond)
			defer cc()
			r, err := http.DefaultClient.Do(mkReq(c, proxyBase, "hanging.localhost"))
			if err != nil {
				return // request died before response — fine
			}
			_, _ = io.ReadAll(r.Body) // reads until client ctx cancel
			_ = r.Body.Close()
		}()
	}

	// 3. Mid-stream aborts (ADR-008: truncated body, no offline, no 502 injection).
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := proxyReq(t, ctx, proxyBase, "stream.localhost", "/")
			body, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if r.StatusCode != 200 {
				fail("stream: status %d", r.StatusCode)
			}
			if err == nil {
				fail("stream: expected truncated read")
			}
			if string(body) != "data: part\n\n" {
				fail("stream: body %q", body)
			}
		}()
	}

	// 4. Concurrent /routes/check bursts while traffic flows.
	var checkErrs []error
	var checkMu sync.Mutex
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := client.CheckRoutes(ctx)
			checkMu.Lock()
			defer checkMu.Unlock()
			if err != nil {
				checkErrs = append(checkErrs, err)
				return
			}
			if len(res) != 6 {
				checkErrs = append(checkErrs, fmt.Errorf("check returned %d rows", len(res)))
			}
		}()
	}
	wg.Wait()

	for _, e := range checkErrs {
		t.Errorf("CheckRoutes: %v", e)
	}

	// 5. Final state assertions — generation ordering must hold.
	if got := statusOf(t, tr, "good.localhost"); got != health.StatusOnline {
		t.Errorf("good: %s", got)
	}
	if got := statusOf(t, tr, "fiveoh.localhost"); got != health.StatusOnline {
		t.Errorf("500-backend: %s (500 must be online)", got)
	}
	if got := statusOf(t, tr, "dead.localhost"); got != health.StatusOffline {
		t.Errorf("dead: %s", got)
	}
	if got := statusOf(t, tr, "hanging.localhost"); got != health.StatusOnline {
		t.Errorf("hanging: %s (client cancel must not flip to offline)", got)
	}
	if got := statusOf(t, tr, "stream.localhost"); got != health.StatusOnline {
		t.Errorf("stream: %s (mid-stream abort must not flip offline)", got)
	}
	// Never proxied, but CheckRoutes probes all routes → refused → offline.
	if got := statusOf(t, tr, "untouched.localhost"); got != health.StatusOffline {
		t.Errorf("untouched: %s", got)
	}

	// 6. IPC regression: GET /routes fields, GET /status.
	routes, err := client.ListRoutes(ctx)
	if err != nil {
		t.Fatalf("ListRoutes: %v", err)
	}
	byDomain := map[string]RouteResponse{}
	for _, r := range routes {
		byDomain[r.Domain] = r
	}
	if byDomain["dead.localhost"].Status != "offline" || byDomain["good.localhost"].Status != "online" {
		t.Errorf("routes health fields wrong: %+v", routes)
	}
	if byDomain["untouched.localhost"].Status != "offline" {
		t.Errorf("untouched status: %q", byDomain["untouched.localhost"].Status)
	}
	st, err := client.Status(ctx)
	if err != nil || st.Routes != 6 {
		t.Errorf("Status: %+v err=%v", st, err)
	}

	// 7. CheckRoutes against the live stack once more for latency sanity.
	res, err := client.CheckRoutes(ctx)
	if err != nil {
		t.Fatalf("final CheckRoutes: %v", err)
	}
	for _, r := range res {
		if r.Domain == "good.localhost" && r.Latency <= 0 {
			t.Errorf("good latency %v", r.Latency)
		}
		if r.Domain == "dead.localhost" && r.Reason == "" {
			t.Error("dead check missing reason")
		}
	}

	// 8. Shutdown regression: POST /shutdown via the nonce-bearing path.
	if err := client.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown IPC: %v", err)
	}
}

func mkReq(ctx context.Context, base, host string) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/", nil)
	req.Host = host
	return req
}

// TestStressCheckOnlyOnRequest verifies no background probing happens: a
// daemon left idle must not touch backends. We prove it by pointing a route
// at a port we control and asserting zero connection attempts while idle.
func TestStressNoBackgroundPolling(t *testing.T) {
	var attempts int32
	var mu sync.Mutex
	l, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			attempts++
			mu.Unlock()
			_ = c.Close()
		}
	}()
	defer func() { _ = l.Close() }()
	port := l.Addr().(*net.TCPAddr).Port

	cancel, tr, _ := newStack(t, map[string]int{"idle.localhost": port})
	defer cancel()

	// Startup-equivalent one-shot check drains the expected count.
	client := NewClient()
	if _, err := client.CheckRoutes(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	baseline := attempts
	mu.Unlock()
	if baseline != 1 {
		t.Fatalf("expected exactly 1 probe from CheckAll, got %d", baseline)
	}

	// Idle 2s — more than the old 3s ticker's half period, and well beyond
	// any accidental fast loop.
	time.Sleep(2 * time.Second)
	mu.Lock()
	got := attempts
	mu.Unlock()
	if got != baseline {
		t.Fatalf("background probing detected: %d extra connection attempts while idle", got-baseline)
	}
	if got := statusOf(t, tr, "idle.localhost"); got != health.StatusOnline {
		t.Errorf("idle route should be online after check, got %s", got)
	}
}

// TestStressMalformedAndConcurrentIPC checks malformed requests and heavy
// concurrent IPC mixing.
func TestStressConcurrentIPC(t *testing.T) {
	cancel, _, _ := newStack(t, map[string]int{"a.localhost": deadPort(t)})
	defer cancel()
	ctx := context.Background()
	client := NewClient()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); _, _ = client.ListRoutes(ctx) }()
		go func() { defer wg.Done(); _, _ = client.Status(ctx) }()
		go func() {
			defer wg.Done()
			res, err := client.CheckRoutes(ctx)
			if err != nil || len(res) != 1 {
				t.Errorf("check: %v %d", err, len(res))
			}
		}()
	}
	wg.Wait()

	// Malformed: GET on /routes/check must 405 (POST-only).
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://d/routes/check", nil)
	uc := &http.Client{Transport: &http.Transport{
		DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
			return new(net.Dialer).DialContext(c, "unix", SocketPath())
		},
	}}
	resp, err := uc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 405 {
		t.Errorf("GET /routes/check: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// TestStressShutdownDuringCheck verifies cancellation during an in-flight
// check does not corrupt state.
func TestStressShutdownDuringCheck(t *testing.T) {
	// Slow backend: probe will hang for its 200ms timeout.
	l, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { time.Sleep(400 * time.Millisecond); _ = c.Close() }()
		}
	}()

	cancel, tr, _ := newStack(t, map[string]int{"slow.localhost": l.Addr().(*net.TCPAddr).Port})

	client := NewClient()
	var res []CheckResponse
	var cerr error
	done := make(chan struct{})
	go func() {
		res, cerr = client.CheckRoutes(context.Background())
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel() // daemon ctx dies mid-check
	<-done
	// Either an error or valid results are acceptable; corrupt state is not.
	if cerr == nil && len(res) != 1 {
		t.Errorf("check returned %d rows", len(res))
	}
	snap := tr.Snapshot([]string{"slow.localhost"})
	valid := snap[0].Status == health.StatusOnline || snap[0].Status == health.StatusUnknown
	if !valid {
		t.Errorf("unexpected status after cancel: %s", snap[0].Status)
	}
}

// TestStressCheckResponseDecode sanity-checks JSON contract on the wire.
func TestStressCheckJSONContract(t *testing.T) {
	cancel, _, _ := newStack(t, map[string]int{"x.localhost": deadPort(t)})
	defer cancel()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://d/routes/check", nil)
	uc := &http.Client{Transport: &http.Transport{
		DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
			return new(net.Dialer).DialContext(c, "unix", SocketPath())
		},
	}}
	resp, err := uc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var rows []CheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rows) != 1 || rows[0].Domain != "x.localhost" || rows[0].Status != "offline" {
		t.Fatalf("unexpected wire shape: %+v", rows)
	}
}
