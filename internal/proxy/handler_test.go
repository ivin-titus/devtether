package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ivin-titus/devtether/internal/health"
	"github.com/ivin-titus/devtether/internal/router"
)

const testHost = "app.localhost"

// newTestProxy routes testHost to backendPort through the real middleware
// stack (LoggingMiddleware → Handler → httputil.ReverseProxy) on an
// ephemeral httptest server.
func newTestProxy(t *testing.T, backendPort int) *httptest.Server {
	t.Helper()

	engine := router.NewEngine()
	if err := engine.AddRoute(testHost, "test-service", backendPort, router.RouteStatic); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	srv := httptest.NewServer(LoggingMiddleware(NewHandler(engine, health.NewTracker())))
	t.Cleanup(srv.Close)
	return srv
}

// proxyGet issues a GET against the proxy with the routed Host header.
func proxyGet(t *testing.T, ctx context.Context, proxyURL, path string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyURL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = testHost

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	return resp
}

// backendPort extracts the port of an httptest server's URL.
func backendPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	var p int
	if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return p
}

// waitForChannel fails the test if ch is not closed within the timeout.
func waitForChannel(t *testing.T, ch <-chan struct{}, what string, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// TestStreamingSSENotBuffered verifies that a streaming response is flushed
// to the client incrementally rather than buffered until the backend
// finishes (ADR-008 streaming support).
func TestStreamingSSENotBuffered(t *testing.T) {
	event1Sent := make(chan struct{})
	releaseRest := make(chan struct{})
	var once sync.Once

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		_, _ = fmt.Fprint(w, "data: event-1\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(event1Sent)

		// Hold the stream open until the client has provably received
		// event-1. If the proxy buffered the response, the client read
		// below would never complete and the test would time out.
		<-releaseRest

		_, _ = fmt.Fprint(w, "data: event-2\n\ndata: event-3\n\n")
	}))
	// Release the backend handler before closing the server, so a failed
	// assertion cannot leave Close() waiting on the blocked handler.
	t.Cleanup(func() {
		once.Do(func() { close(releaseRest) })
		backend.Close()
	})

	proxySrv := newTestProxy(t, backendPort(t, backend))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp := proxyGet(t, ctx, proxySrv.URL, "/events")
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read until the complete first event (including its terminating
	// blank line) arrives; the backend is still blocked, so this read
	// succeeding proves the first event was not buffered.
	waitForChannel(t, event1Sent, "backend to send event-1", 5*time.Second)

	reader := bufio.NewReader(resp.Body)
	if err := readUntilContains(ctx, reader, "event-1\n\n"); err != nil {
		t.Fatalf("did not receive complete event-1 while stream was still open: %v", err)
	}

	// Release the remaining events and verify the complete body.
	once.Do(func() { close(releaseRest) })
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read remaining stream: %v", err)
	}
	if want := "data: event-2\n\ndata: event-3\n\n"; string(rest) != want {
		t.Fatalf("remaining body = %q, want %q — stream truncated", rest, want)
	}
}

// readUntilContains reads from r until data contains want or ctx expires.
func readUntilContains(ctx context.Context, r *bufio.Reader, want string) error {
	var sb strings.Builder
	for !strings.Contains(sb.String(), want) {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := r.ReadString('\n')
		sb.WriteString(line)
		if err != nil {
			return err
		}
	}
	return nil
}

// TestClientCancellationPropagates verifies that cancelling the client
// request context cancels the backend request context through the proxy.
func TestClientCancellationPropagates(t *testing.T) {
	backendStarted := make(chan struct{})
	backendCancelled := make(chan struct{})
	var once sync.Once

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: start\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(backendStarted)

		<-r.Context().Done()
		once.Do(func() { close(backendCancelled) })
	}))
	defer backend.Close()

	proxySrv := newTestProxy(t, backendPort(t, backend))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp := proxyGet(t, ctx, proxySrv.URL, "/events")
	defer func() { _ = resp.Body.Close() }()

	waitForChannel(t, backendStarted, "backend handler to start streaming", 5*time.Second)

	cancel()

	waitForChannel(t, backendCancelled, "backend request context cancellation", 5*time.Second)
}

// TestMidStreamAbort verifies ADR-008: when the backend fails mid-stream
// (headers already sent), the proxy aborts the connection and the client
// observes a truncated body — never an injected 502 error page. The
// backend flushes a 200 status and a first chunk, then aborts, so the
// client deterministically receives status 200 and the partial payload
// before the stream is severed.
func TestMidStreamAbort(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "partial-payload")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Abort mid-stream: the proxy's copy loop hits a read error after
		// headers were already sent downstream.
		panic(http.ErrAbortHandler)
	}))
	defer backend.Close()

	proxySrv := newTestProxy(t, backendPort(t, backend))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxySrv.URL+"/download", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = testHost

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("no HTTP response from proxy: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 — proxy must not rewrite an open stream", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err == nil {
		t.Fatalf("expected truncated-stream read error, got clean EOF with body %q", body)
	}
	if got := string(body); got != "partial-payload" {
		t.Fatalf("body = %q, want partial-payload", got)
	}
	if strings.Contains(string(body), "Bad Gateway") {
		t.Fatal("proxy injected a 502 error page into an already-streaming response")
	}
}

// TestHTTPUpgradeTunnel verifies that a generic HTTP Upgrade handshake
// (the mechanism WebSocket-style connections rely on) and bidirectional
// data flow work through the proxy middleware stack. The middleware's
// Unwrap() must expose the real writer for hijacking. This exercises the
// 101 upgrade and raw tunnel only; it is not an RFC 6455 WebSocket frame
// test.
func TestHTTPUpgradeTunnel(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "not a websocket request", http.StatusBadRequest)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack unsupported", http.StatusInternalServerError)
			return
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = rw.Flush()

		// Echo lines back until the client disconnects.
		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				return
			}
			if _, err := rw.WriteString("echo: " + line); err != nil {
				return
			}
			_ = rw.Flush()
		}
	}))
	defer backend.Close()

	proxySrv := newTestProxy(t, backendPort(t, backend))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proxyAddr := strings.TrimPrefix(proxySrv.URL, "http://")
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	_, err = fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n", testHost)
	if err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	status, err := rw.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("upgrade status = %q, want 101", status)
	}
	// Drain handshake headers.
	for {
		hdr, herr := rw.ReadString('\n')
		if herr != nil {
			t.Fatalf("read handshake headers: %v", herr)
		}
		if hdr == "\r\n" {
			break
		}
	}

	if _, werr := rw.WriteString("ping\n"); werr != nil {
		t.Fatalf("write frame: %v", werr)
	}
	if ferr := rw.Flush(); ferr != nil {
		t.Fatalf("flush frame: %v", ferr)
	}

	echo, err := rw.ReadString('\n')
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if echo != "echo: ping\n" {
		t.Fatalf("echo = %q, want %q", echo, "echo: ping\n")
	}
}

// TestXForwardedHeaders verifies the documented Rewrite contract: the
// backend receives the rewritten target URL plus X-Forwarded-* headers
// derived from the routing context.
func TestXForwardedHeaders(t *testing.T) {
	var mu sync.Mutex
	var gotXFHost, gotXFFor, gotXFProto string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotXFHost = r.Header.Get("X-Forwarded-Host")
		gotXFFor = r.Header.Get("X-Forwarded-For")
		gotXFProto = r.Header.Get("X-Forwarded-Proto")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxySrv := newTestProxy(t, backendPort(t, backend))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp := proxyGet(t, ctx, proxySrv.URL, "/path")
	_ = resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if gotXFHost != testHost {
		t.Fatalf("X-Forwarded-Host = %q, want %q", gotXFHost, testHost)
	}
	if gotXFFor == "" {
		t.Fatal("X-Forwarded-For is empty, want client IP")
	}
	if gotXFProto != "http" {
		t.Fatalf("X-Forwarded-Proto = %q, want http", gotXFProto)
	}
}

// TestBadGatewayPage verifies that when the backend is unreachable the
// proxy renders the branded 502 page (headers not yet sent path).
func TestBadGatewayPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Grab an ephemeral port and release it so nothing listens there.
	l, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	proxySrv := newTestProxy(t, port)

	resp := proxyGet(t, ctx, proxySrv.URL, "/")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Bad Gateway") {
		t.Fatal("502 body does not contain the branded error page")
	}
}

// newTestProxyWithTracker is newTestProxy that also returns the health
// tracker shared with the handler under test.
func newTestProxyWithTracker(t *testing.T, backendPort int) (*httptest.Server, *health.Tracker) {
	t.Helper()

	engine := router.NewEngine()
	if err := engine.AddRoute(testHost, "test-service", backendPort, router.RouteStatic); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	tracker := health.NewTracker()
	srv := httptest.NewServer(LoggingMiddleware(NewHandler(engine, tracker)))
	t.Cleanup(srv.Close)
	return srv, tracker
}

func routeStatus(t *testing.T, tracker *health.Tracker) health.Status {
	t.Helper()
	return tracker.Snapshot([]string{testHost})[0].Status
}

// TestHealthOnlineObservation verifies that a successful backend response —
// even an application-level HTTP 500 — marks the route online.
func TestHealthOnlineObservation(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	proxySrv, tracker := newTestProxyWithTracker(t, backendPort(t, backend))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp := proxyGet(t, ctx, proxySrv.URL, "/")
	_ = resp.Body.Close()

	if got := routeStatus(t, tracker); got != health.StatusOnline {
		t.Fatalf("HTTP 500 response must still mark backend online, got %s", got)
	}
}

// TestHealthOfflineObservation verifies that a backend dial failure marks
// the route offline.
func TestHealthOfflineObservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	l, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	proxySrv, tracker := newTestProxyWithTracker(t, port)

	resp := proxyGet(t, ctx, proxySrv.URL, "/")
	_ = resp.Body.Close()

	if got := routeStatus(t, tracker); got != health.StatusOffline {
		t.Fatalf("refused backend must mark route offline, got %s", got)
	}
}

// TestHealthClientCancelLeavesState verifies that client cancellation does
// not incorrectly mark a backend offline.
func TestHealthClientCancelLeavesState(t *testing.T) {
	backendStarted := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "chunk")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(backendStarted)
		<-r.Context().Done()
	}))
	defer backend.Close()

	proxySrv, tracker := newTestProxyWithTracker(t, backendPort(t, backend))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := proxyGet(t, ctx, proxySrv.URL, "/stream")
	defer func() { _ = resp.Body.Close() }()
	waitForChannel(t, backendStarted, "backend to start", 5*time.Second)
	cancel()
	time.Sleep(50 * time.Millisecond)

	if got := routeStatus(t, tracker); got != health.StatusOnline {
		t.Fatalf("client cancel must not mark backend offline, got %s", got)
	}
}

// TestHealthConcurrentRequests verifies tracker updates are race-free under
// concurrent proxy traffic.
func TestHealthConcurrentRequests(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxySrv, tracker := newTestProxyWithTracker(t, backendPort(t, backend))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				resp := proxyGet(t, ctx, proxySrv.URL, "/")
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	if got := routeStatus(t, tracker); got != health.StatusOnline {
		t.Fatalf("expected online after concurrent requests, got %s", got)
	}
}

// roundTripFunc lets tests inject a controllable backend transport into the
// handler's reverse proxy without sleeps or external network dependencies.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type dialTimeoutErr struct{}

func (dialTimeoutErr) Error() string   { return "i/o timeout" }
func (dialTimeoutErr) Timeout() bool   { return true }
func (dialTimeoutErr) Temporary() bool { return false }

func dialOpError(err error) error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: err}
}

func newProxyWithTransport(t *testing.T, rt http.RoundTripper) (*httptest.Server, *health.Tracker) {
	t.Helper()
	engine := router.NewEngine()
	if err := engine.AddRoute(testHost, "test-service", 1, router.RouteStatic); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	tracker := health.NewTracker()
	h := NewHandler(engine, tracker)
	h.rp.Transport = rt
	srv := httptest.NewServer(LoggingMiddleware(h))
	t.Cleanup(srv.Close)
	return srv, tracker
}

// TestHealthClientCancelDuringDial verifies that cancelling the client while
// the proxy is dialing the backend records no health observation at all — the
// tracker stays StatusUnknown — even though the transport error wraps a
// *net.OpError with Op == "dial". Fully event-driven: no sleeps.
func TestHealthClientCancelDuringDial(t *testing.T) {
	dialStarted := make(chan struct{})
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-dialStarted:
		default:
			close(dialStarted)
		}
		<-r.Context().Done()
		return nil, dialOpError(r.Context().Err())
	})
	proxySrv, tracker := newProxyWithTransport(t, rt)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxySrv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = testHost

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, _ := http.DefaultClient.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-dialStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("dial never started")
	}

	cancel()

	select {
	case <-requestDone:
	case <-time.After(5 * time.Second):
		t.Fatal("request goroutine did not finish")
	}

	if got := routeStatus(t, tracker); got != health.StatusUnknown {
		t.Fatalf("client cancel during dial must record no observation, got %s", got)
	}
}

// TestHealthClientDeadlineDuringDial verifies that an expiring client request
// deadline during backend dial records no health observation — the tracker
// stays StatusUnknown. Event-driven: the transport signals entry, the test
// then waits for ctx expiry and request completion. No sleeps.
func TestHealthClientDeadlineDuringDial(t *testing.T) {
	dialStarted := make(chan struct{})
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-dialStarted:
		default:
			close(dialStarted)
		}
		<-r.Context().Done()
		return nil, dialOpError(r.Context().Err())
	})
	proxySrv, tracker := newProxyWithTransport(t, rt)

	// A comfortably long deadline: the test guarantees entry via dialStarted
	// before expiry can matter, so 300ms is never the limiting factor.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxySrv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = testHost

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, _ := http.DefaultClient.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	// Guarantee the deadline expires *during* the dial, not before it.
	select {
	case <-dialStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("dial never started")
	}

	select {
	case <-requestDone:
	case <-time.After(5 * time.Second):
		t.Fatal("request goroutine did not finish")
	}

	if got := routeStatus(t, tracker); got != health.StatusUnknown {
		t.Fatalf("client deadline during dial must record no observation, got %s", got)
	}
}

// TestHealthDialTimeoutMarksOffline verifies a genuine backend dial timeout —
// with the client context still alive — retains the offline classification.
func TestHealthDialTimeoutMarksOffline(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, dialOpError(dialTimeoutErr{})
	})
	proxySrv, tracker := newProxyWithTransport(t, rt)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp := proxyGet(t, ctx, proxySrv.URL, "/")
	_ = resp.Body.Close()

	if got := routeStatus(t, tracker); got != health.StatusOffline {
		t.Fatalf("genuine dial timeout must mark offline, got %s", got)
	}
}
