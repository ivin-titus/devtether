package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ivin-titus/devtether/internal/health"
	"github.com/ivin-titus/devtether/internal/router"
)

func newEngine(t *testing.T, domain string, port int) *router.Engine {
	t.Helper()
	e := router.NewEngine()
	if err := e.AddRoute(domain, "svc", port, router.RouteStatic); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	return e
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestHandleRoutesCheckRequiresPost(t *testing.T) {
	s := &Server{engine: router.NewEngine(), tracker: health.NewTracker()}
	rec := httptest.NewRecorder()
	s.handleRoutesCheck(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/routes/check", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /routes/check: got %d, want 405", rec.Code)
	}
}

func TestHandleRoutesCheck(t *testing.T) {
	// One live backend, one dead backend.
	live, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = live.Close() }()
	livePort := live.Addr().(*net.TCPAddr).Port
	deadPort := freePort(t)

	e := router.NewEngine()
	if err := e.AddRoute("ok.localhost", "svc", livePort, router.RouteStatic); err != nil {
		t.Fatal(err)
	}
	if err := e.AddRoute("down.localhost", "svc", deadPort, router.RouteStatic); err != nil {
		t.Fatal(err)
	}

	s := &Server{engine: e, tracker: health.NewTracker()}
	rec := httptest.NewRecorder()
	s.handleRoutesCheck(rec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/routes/check", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var results []CheckResponse
	if err := json.NewDecoder(rec.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	byDomain := map[string]CheckResponse{}
	for _, r := range results {
		byDomain[r.Domain] = r
	}
	if byDomain["ok.localhost"].Status != "online" {
		t.Fatalf("ok.localhost: got %q, want online", byDomain["ok.localhost"].Status)
	}
	if byDomain["ok.localhost"].Latency <= 0 {
		t.Fatal("ok.localhost: expected positive latency")
	}
	if byDomain["down.localhost"].Status != "offline" {
		t.Fatalf("down.localhost: got %q, want offline", byDomain["down.localhost"].Status)
	}
	if byDomain["down.localhost"].Reason == "" {
		t.Fatal("down.localhost: expected failure reason")
	}
}

func TestHandleRoutesIncludesHealth(t *testing.T) {
	e := newEngine(t, "app.localhost", 8080)
	tr := health.NewTracker()
	tr.ObserveNow("app.localhost", health.Observation{Status: health.StatusOnline, CheckedAt: time.Now()})

	s := &Server{engine: e, tracker: tr}
	rec := httptest.NewRecorder()
	s.handleRoutes(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/routes", nil))

	var routes []RouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&routes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(routes) != 1 || routes[0].Status != "online" {
		t.Fatalf("expected online route, got %+v", routes)
	}
	if routes[0].LastChecked == "" {
		t.Fatal("expected lastChecked timestamp")
	}
}

func TestHandleRoutesUnknownHealth(t *testing.T) {
	s := &Server{engine: newEngine(t, "app.localhost", 8080), tracker: health.NewTracker()}
	rec := httptest.NewRecorder()
	s.handleRoutes(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/routes", nil))

	var routes []RouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&routes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(routes) != 1 || routes[0].Status != "unknown" {
		t.Fatalf("unchecked route must report unknown, got %+v", routes)
	}
}
