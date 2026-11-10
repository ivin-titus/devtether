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
	"strings"
	"sync"
	"testing"
	"time"

	"errors"

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
	tr := health.NewTracker()
	for d, p := range routes {
		inc, addErr := e.AddRoute(d, "svc", p, router.RouteStatic)
		if addErr != nil {
			t.Fatal(addErr)
		}
		tr.Register(d, inc)
	}
	ctx, cancel := context.WithCancel(context.Background())

	srv := NewServer(e, tr, cancel, "devtether.yaml")
	ln, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("daemon Listen: %v", err)
	}
	// Serve's deferred cleanup (nonce/PID/socket removal) resolves paths
	// from XDG_RUNTIME_DIR at exit time; waiting for it in t.Cleanup keeps
	// it from deleting a later test's runtime files.
	serveDone := make(chan struct{})
	go func() { _ = srv.Serve(ctx, ln); close(serveDone) }()

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
		cancel()
		<-serveDone
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

// rawIPC sends one request over the daemon socket, optionally with the
// on-disk nonce, and returns the status code.
func rawIPC(t *testing.T, method, path string, body io.Reader, withNonce bool) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, "http://d"+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if withNonce {
		nonce, nErr := readNonce()
		if nErr != nil {
			t.Fatalf("readNonce: %v", nErr)
		}
		req.Header.Set("X-Devtether-Nonce", nonce)
	}
	resp, err := socketClient(5 * time.Second).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func apiStatus(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}

// TestStressRouteClientLifecycle exercises the real socket: client 201/204
// success paths, typed APIError statuses, and the GET incarnation contract.
func TestStressRouteClientLifecycle(t *testing.T) {
	cancel, tr, base := newStack(t, map[string]int{"static.localhost": deadPort(t)})
	defer cancel()
	ctx := context.Background()
	client := NewClient()
	port := backendPort(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

	created, err := client.AddRoute(ctx, "live.localhost", port)
	if err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	if created.Type != "live" || created.Incarnation == 0 || created.Domain != "live.localhost" {
		t.Fatalf("unexpected created route: %+v", created)
	}
	// Proxy serves the live route immediately — no restart.
	resp := proxyReq(t, ctx, base, "live.localhost", "/")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("proxy live route: %d", resp.StatusCode)
	}

	routes, err := client.ListRoutes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var listed RouteResponse
	for _, r := range routes {
		if r.Domain == "live.localhost" {
			listed = r
		}
	}
	if listed.Incarnation != created.Incarnation {
		t.Fatalf("GET incarnation %d != POST incarnation %d", listed.Incarnation, created.Incarnation)
	}

	// Typed error statuses.
	if _, err = client.AddRoute(ctx, "live.localhost", port); apiStatus(err) != http.StatusConflict {
		t.Fatalf("duplicate AddRoute: %v", err)
	}
	if _, err = client.AddRoute(ctx, "bad.internal", port); apiStatus(err) != http.StatusBadRequest {
		t.Fatalf("invalid AddRoute: %v", err)
	}
	if _, err = client.AddRoute(ctx, "p.localhost", 70000); apiStatus(err) != http.StatusBadRequest {
		t.Fatalf("out-of-range AddRoute: %v", err)
	}
	if err = client.RemoveRoute(ctx, "live.localhost", created.Incarnation+100); apiStatus(err) != http.StatusConflict {
		t.Fatalf("stale RemoveRoute: %v", err)
	}
	if err = client.RemoveRoute(ctx, "absent.localhost", 1); apiStatus(err) != http.StatusNotFound {
		t.Fatalf("absent RemoveRoute: %v", err)
	}
	var apiErr *APIError
	if err = client.RemoveRoute(ctx, "absent.localhost", 1); !errors.As(err, &apiErr) || apiErr.Body == "" {
		t.Fatalf("APIError must carry trimmed body: %v", err)
	}

	// 204 success for live and static routes.
	if err = client.RemoveRoute(ctx, "live.localhost", created.Incarnation); err != nil {
		t.Fatalf("RemoveRoute live: %v", err)
	}
	staticInc := uint64(0)
	routes, _ = client.ListRoutes(ctx)
	for _, r := range routes {
		if r.Domain == "static.localhost" {
			staticInc = r.Incarnation
		}
	}
	if err = client.RemoveRoute(ctx, "static.localhost", staticInc); err != nil {
		t.Fatalf("RemoveRoute static: %v", err)
	}
	// After removal new proxy requests are rejected and health is retired.
	resp = proxyReq(t, ctx, base, "live.localhost", "/")
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusTeapot {
		t.Fatal("proxy still serves removed route")
	}
	if st := statusOf(t, tr, "live.localhost"); st != health.StatusUnknown {
		t.Fatalf("removed route health: %s", st)
	}

	// Re-add strictly increases the incarnation.
	again, err := client.AddRoute(ctx, "live.localhost", port)
	if err != nil || again.Incarnation <= created.Incarnation {
		t.Fatalf("re-add: %+v %v", again, err)
	}
}

// TestStressRouteMuxAndNonce pins ServeMux precedence and nonce enforcement
// on the production mux over the real socket.
func TestStressRouteMuxAndNonce(t *testing.T) {
	cancel, _, _ := newStack(t, map[string]int{"app.localhost": deadPort(t)})
	defer cancel()

	cases := []struct {
		method, path string
		body         string
		nonce        bool
		want         int
	}{
		{http.MethodDelete, "/routes/check?inc=1", "", true, http.StatusMethodNotAllowed},
		{http.MethodDelete, "/routes/check/", "", true, http.StatusNotFound},
		{http.MethodDelete, "/routes/", "", true, http.StatusNotFound},
		{http.MethodDelete, "/routes/app.localhost/extra?inc=1", "", true, http.StatusNotFound},
		{http.MethodGet, "/routes/app.localhost", "", true, http.StatusMethodNotAllowed},
		{http.MethodDelete, "/routes/a%2Fb.localhost?inc=1", "", true, http.StatusBadRequest},
		{http.MethodDelete, "/routes/%20app.localhost?inc=1", "", true, http.StatusBadRequest},
		{http.MethodDelete, "/routes/app.localhost?inc=1", "", false, http.StatusForbidden},
		{http.MethodPost, "/routes", `{"domain":"n.localhost","port":3000}`, false, http.StatusForbidden},
		{http.MethodPut, "/routes", "", true, http.StatusMethodNotAllowed},
		{http.MethodPost, "/routes", `{"domain":"n.localhost","port":3000,"x":1}`, true, http.StatusBadRequest},
		{http.MethodPost, "/routes", strings.Repeat(" ", 9<<10), true, http.StatusRequestEntityTooLarge},
		// beta.7 contracts unchanged.
		{http.MethodGet, "/routes", "", false, http.StatusOK},
		{http.MethodGet, "/status", "", false, http.StatusOK},
		{http.MethodGet, "/routes/check", "", false, http.StatusMethodNotAllowed},
		{http.MethodPost, "/shutdown", "", false, http.StatusForbidden},
	}
	for _, c := range cases {
		var body io.Reader
		if c.body != "" {
			body = strings.NewReader(c.body)
		}
		if got := rawIPC(t, c.method, c.path, body, c.nonce); got != c.want {
			t.Errorf("%s %s (nonce=%v): got %d, want %d", c.method, c.path, c.nonce, got, c.want)
		}
	}
}

// startGate runs n workers that block until the gate closes, so all requests
// race into the daemon together; the gate replaces timing-based ordering.
func startGate(n int, work func(i int)) {
	gate := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			work(i)
		}()
	}
	close(gate)
	wg.Wait()
}

func TestStressConcurrentRouteCreate(t *testing.T) {
	cancel, _, _ := newStack(t, nil)
	defer cancel()
	client := NewClient()
	const n = 16
	codes := make([]int, n)
	incs := make([]uint64, n)
	startGate(n, func(i int) {
		r, err := client.AddRoute(context.Background(), "race.localhost", 3000+i)
		if err == nil {
			codes[i], incs[i] = http.StatusCreated, r.Incarnation
			return
		}
		codes[i] = apiStatus(err)
	})
	created, conflicts := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if created != 1 || conflicts != n-1 {
		t.Fatalf("created=%d conflicts=%d", created, conflicts)
	}
}

func TestStressConcurrentRouteDelete(t *testing.T) {
	cancel, tr, _ := newStack(t, nil)
	defer cancel()
	client := NewClient()
	r, err := client.AddRoute(context.Background(), "race.localhost", 3000)
	if err != nil {
		t.Fatal(err)
	}
	const n = 16
	codes := make([]int, n)
	startGate(n, func(i int) {
		if err := client.RemoveRoute(context.Background(), "race.localhost", r.Incarnation); err != nil {
			codes[i] = apiStatus(err)
			return
		}
		codes[i] = http.StatusNoContent
	})
	removed, missing := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusNoContent:
			removed++
		case http.StatusNotFound:
			missing++
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if removed != 1 || missing != n-1 {
		t.Fatalf("removed=%d missing=%d", removed, missing)
	}
	if tr.ObserveNow("race.localhost", r.Incarnation, health.Observation{Status: health.StatusOnline, CheckedAt: time.Now()}) {
		t.Fatal("removed incarnation still observable")
	}
}

// TestStressDeleteRacingReAdd races stale DELETEs of incarnation A against a
// concurrent re-creation (incarnation B). No stale DELETE may remove B, and
// after settling the tracker reflects only B.
func TestStressDeleteRacingReAdd(t *testing.T) {
	cancel, tr, _ := newStack(t, nil)
	defer cancel()
	client := NewClient()
	ctx := context.Background()
	a, err := client.AddRoute(ctx, "race.localhost", 3000)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.RemoveRoute(ctx, "race.localhost", a.Incarnation); err != nil {
		t.Fatal(err)
	}

	const n = 16
	codes := make([]int, n)
	var b *RouteResponse
	startGate(n+1, func(i int) {
		if i == n {
			r, addErr := client.AddRoute(ctx, "race.localhost", 4000)
			if addErr != nil {
				t.Errorf("re-add: %v", addErr)
				return
			}
			b = r
			return
		}
		if rmErr := client.RemoveRoute(ctx, "race.localhost", a.Incarnation); rmErr != nil {
			codes[i] = apiStatus(rmErr)
			return
		}
		codes[i] = http.StatusNoContent
	})
	if b == nil {
		t.Fatal("re-add failed")
	}
	for i, c := range codes {
		if c != http.StatusNotFound && c != http.StatusConflict {
			t.Fatalf("stale DELETE %d: got %d, want 404 or 409", i, c)
		}
	}
	if b.Incarnation <= a.Incarnation {
		t.Fatalf("incarnations not increasing: %d then %d", a.Incarnation, b.Incarnation)
	}
	routes, err := client.ListRoutes(ctx)
	if err != nil || len(routes) != 1 || routes[0].Incarnation != b.Incarnation || routes[0].Port != 4000 {
		t.Fatalf("replacement lost: %+v %v", routes, err)
	}
	// Post-settle: only B is live in the tracker.
	if tr.ObserveNow("race.localhost", a.Incarnation, health.Observation{Status: health.StatusOffline, CheckedAt: time.Now()}) {
		t.Fatal("stale incarnation A observable after settle")
	}
	if !tr.ObserveNow("race.localhost", b.Incarnation, health.Observation{Status: health.StatusOnline, CheckedAt: time.Now()}) {
		t.Fatal("replacement incarnation B not registered")
	}
	// A now-stale DELETE of A against live B is a deterministic 409.
	if err := client.RemoveRoute(ctx, "race.localhost", a.Incarnation); apiStatus(err) != http.StatusConflict {
		t.Fatalf("stale DELETE after settle: %v", err)
	}
	if st := statusOf(t, tr, "race.localhost"); st != health.StatusOnline {
		t.Fatalf("replacement health cleared: %s", st)
	}
}
