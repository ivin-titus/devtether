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

	srv := httptest.NewServer(LoggingMiddleware(NewHandler(engine)))
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
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read until event-1 arrives; the backend is still blocked, so this
	// read succeeding proves the first event was not buffered.
	waitForChannel(t, event1Sent, "backend to send event-1", 5*time.Second)

	reader := bufio.NewReader(resp.Body)
	if err := readUntilContains(ctx, reader, "event-1"); err != nil {
		t.Fatalf("did not receive event-1 while stream was still open: %v", err)
	}

	// Release the remaining events and verify the complete body.
	once.Do(func() { close(releaseRest) })
	if err := readUntilContains(ctx, reader, "event-3"); err != nil {
		t.Fatalf("did not receive full stream: %v", err)
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
	defer resp.Body.Close()

	waitForChannel(t, backendStarted, "backend handler to start streaming", 5*time.Second)

	cancel()

	waitForChannel(t, backendCancelled, "backend request context cancellation", 5*time.Second)
}

// TestMidStreamAbort verifies ADR-008: when the backend fails mid-stream
// (headers already sent), the proxy aborts the connection and the client
// observes a truncated body — never an injected 502 error page.
func TestMidStreamAbort(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Declare more than we deliver, then return early: the upstream
		// body ends unexpectedly, surfacing as a mid-stream proxy error.
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial-payload"))
	}))
	defer backend.Close()

	proxySrv := newTestProxy(t, backendPort(t, backend))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The abort may surface either as a transport error from Do or as a
	// truncated body read — both prove the connection was torn down.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxySrv.URL+"/download", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = testHost

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Aborted mid-stream before/at delivery; no 502 page was rendered.
		return
	}
	defer resp.Body.Close()

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

// TestWebSocketUpgrade verifies that an HTTP Upgrade handshake and
// bidirectional data flow work through the proxy middleware stack.
// The middleware's Unwrap() must expose the real writer for hijacking.
func TestWebSocketUpgrade(t *testing.T) {
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
		defer conn.Close()

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

	proxyAddr := strings.TrimPrefix(proxySrv.URL, "http://")
	conn, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()
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
		line, err := rw.ReadString('\n')
		if err != nil {
			t.Fatalf("read handshake headers: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}

	if _, err := rw.WriteString("ping\n"); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	if err := rw.Flush(); err != nil {
		t.Fatalf("flush frame: %v", err)
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
	resp.Body.Close()

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
	// Grab an ephemeral port and release it so nothing listens there.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	proxySrv := newTestProxy(t, port)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp := proxyGet(t, ctx, proxySrv.URL, "/")
	defer resp.Body.Close()

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
