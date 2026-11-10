package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ivin-titus/devtether/internal/health"
	"github.com/ivin-titus/devtether/internal/router"
)

func newEngine(t *testing.T, domain string, port int) *router.Engine {
	t.Helper()
	e := router.NewEngine()
	if _, err := e.AddRoute(domain, "svc", port, router.RouteStatic); err != nil {
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
	if _, err := e.AddRoute("ok.localhost", "svc", livePort, router.RouteStatic); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddRoute("down.localhost", "svc", deadPort, router.RouteStatic); err != nil {
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
	inc := e.GetAllRoutes()[0].Incarnation
	tr := health.NewTracker()
	tr.Register("app.localhost", inc)
	tr.ObserveNow("app.localhost", inc, health.Observation{Status: health.StatusOnline, CheckedAt: time.Now()})

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

// TestRouteTargetsCarryIncarnation pins the construction-site contract:
// probe targets must carry the route's incarnation or their observations
// are silently rejected.
func TestRouteTargetsCarryIncarnation(t *testing.T) {
	e := router.NewEngine()
	inc, err := e.AddRoute("app.localhost", "svc", 8080, router.RouteStatic)
	if err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	targets := routeTargets(e.GetAllRoutes())
	if len(targets) != 1 || targets[0].Incarnation != inc {
		t.Fatalf("routeTargets lost incarnation: %+v", targets)
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

// testNonce is deliberately non-empty: Server values built directly in tests
// otherwise carry nonce == "", and a missing header would pass the nonce
// check through empty-string equality.
const testNonce = "test-nonce-0123456789"

func newMutServer(e *router.Engine) (*Server, *health.Tracker) {
	tr := health.NewTracker()
	for _, rv := range e.GetAllRoutes() {
		tr.Register(rv.Domain, rv.Incarnation)
	}
	return &Server{engine: e, tracker: tr, nonce: testNonce}, tr
}

func postRoutes(s *Server, body, nonce string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/routes", strings.NewReader(body))
	if nonce != "" {
		req.Header.Set("X-Devtether-Nonce", nonce)
	}
	rec := httptest.NewRecorder()
	s.handleRoutes(rec, req)
	return rec
}

func deleteRoute(s *Server, domain, rawQuery, nonce string) *httptest.ResponseRecorder {
	target := "/routes/x"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, target, nil)
	req.SetPathValue("domain", domain)
	if nonce != "" {
		req.Header.Set("X-Devtether-Nonce", nonce)
	}
	rec := httptest.NewRecorder()
	s.handleRouteDomain(rec, req)
	return rec
}

func TestRouteMutationsRequireNonce(t *testing.T) {
	e := newEngine(t, "app.localhost", 8080)
	inc := e.GetAllRoutes()[0].Incarnation
	s, _ := newMutServer(e)

	for _, nonce := range []string{"", "wrong-nonce"} {
		if rec := postRoutes(s, `{"domain":"new.localhost","port":3000}`, nonce); rec.Code != http.StatusForbidden {
			t.Fatalf("POST nonce %q: got %d, want 403", nonce, rec.Code)
		}
		if rec := deleteRoute(s, "app.localhost", fmt.Sprintf("inc=%d", inc), nonce); rec.Code != http.StatusForbidden {
			t.Fatalf("DELETE nonce %q: got %d, want 403", nonce, rec.Code)
		}
	}
	if _, ok := e.Resolve("new.localhost"); ok {
		t.Fatal("rejected POST mutated the engine")
	}
	if _, ok := e.Resolve("app.localhost"); !ok {
		t.Fatal("rejected DELETE mutated the engine")
	}
	// Correct nonce proceeds.
	if rec := postRoutes(s, `{"domain":"new.localhost","port":3000}`, testNonce); rec.Code != http.StatusCreated {
		t.Fatalf("POST correct nonce: got %d, want 201: %s", rec.Code, rec.Body)
	}
}

func TestPostRoutesCreatesLiveRoute(t *testing.T) {
	e := router.NewEngine()
	s, tr := newMutServer(e)

	rec := postRoutes(s, `{"port":3000,"domain":"App.Localhost"}`, testNonce)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201: %s", rec.Code, rec.Body)
	}
	var rr RouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&rr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rr.Domain != "app.localhost" || rr.Port != 3000 || rr.Type != string(router.RouteLive) || rr.Incarnation == 0 {
		t.Fatalf("unexpected response: %+v", rr)
	}
	target, ok := e.Resolve("app.localhost")
	if !ok || target.Type != router.RouteLive || target.Incarnation != rr.Incarnation {
		t.Fatalf("engine state: %+v ok=%v", target, ok)
	}
	// Tracker was registered with the exact engine-returned incarnation.
	if !tr.ObserveNow("app.localhost", rr.Incarnation, health.Observation{Status: health.StatusOnline, CheckedAt: time.Now()}) {
		t.Fatal("tracker did not register the returned incarnation")
	}

	// Duplicate (including case variant) → 409, engine unchanged.
	if dup := postRoutes(s, `{"domain":"app.localhost","port":4000}`, testNonce); dup.Code != http.StatusConflict {
		t.Fatalf("duplicate POST: got %d, want 409", dup.Code)
	}
	if after, _ := e.Resolve("app.localhost"); after.Port != 3000 || after.Incarnation != rr.Incarnation {
		t.Fatalf("duplicate POST changed route: %+v", after)
	}
}

func TestPostRoutesStrictJSON(t *testing.T) {
	cases := map[string]string{
		"empty body":          ``,
		"syntax error":        `{"domain":`,
		"empty object":        `{}`,
		"missing port":        `{"domain":"a.localhost"}`,
		"missing domain":      `{"port":3000}`,
		"unknown field":       `{"domain":"a.localhost","port":3000,"extra":1}`,
		"duplicate domain":    `{"domain":"a.localhost","domain":"b.localhost","port":3000}`,
		"duplicate port":      `{"domain":"a.localhost","port":3000,"port":4000}`,
		"case variant PORT":   `{"domain":"a.localhost","PORT":3000}`,
		"case variant Domain": `{"Domain":"a.localhost","port":3000}`,
		"null body":           `null`,
		"array body":          `[{"domain":"a.localhost","port":3000}]`,
		"string body":         `"a.localhost"`,
		"number body":         `3000`,
		"trailing object":     `{"domain":"a.localhost","port":3000}{}`,
		"trailing garbage":    `{"domain":"a.localhost","port":3000} x`,
		"port string":         `{"domain":"a.localhost","port":"3000"}`,
		"port float":          `{"domain":"a.localhost","port":3000.0}`,
		"port exponent":       `{"domain":"a.localhost","port":3e3}`,
		"port negative":       `{"domain":"a.localhost","port":-1}`,
		"port bool":           `{"domain":"a.localhost","port":true}`,
		"port null":           `{"domain":"a.localhost","port":null}`,
		"port object":         `{"domain":"a.localhost","port":{}}`,
		"port overflow":       `{"domain":"a.localhost","port":99999999999999999999999}`,
		"domain number":       `{"domain":1,"port":3000}`,
		"domain null":         `{"domain":null,"port":3000}`,
		"domain array":        `{"domain":["a.localhost"],"port":3000}`,
		"port zero":           `{"domain":"a.localhost","port":0}`,
		"port 65536":          `{"domain":"a.localhost","port":65536}`,
		"invalid domain":      `{"domain":"a.internal","port":3000}`,
		"wildcard domain":     `{"domain":"*.localhost","port":3000}`,
		"whitespace domain":   `{"domain":" a.localhost","port":3000}`,
		"empty domain":        `{"domain":"","port":3000}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			e := router.NewEngine()
			s, _ := newMutServer(e)
			rec := postRoutes(s, body, testNonce)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body)
			}
			if n := len(e.GetAllRoutes()); n != 0 {
				t.Fatalf("invalid request mutated engine: %d routes", n)
			}
		})
	}
}

func TestPostRoutesOversizedBody(t *testing.T) {
	e := router.NewEngine()
	s, _ := newMutServer(e)
	body := `{"domain":"a.localhost","port":3000,"pad":"` + strings.Repeat("x", 9<<10) + `"}`
	if rec := postRoutes(s, body, testNonce); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
	// Oversized whitespace padding before a valid object still trips the
	// limit rather than being parsed.
	padded := strings.Repeat(" ", 9<<10) + `{"domain":"a.localhost","port":3000}`
	if rec := postRoutes(s, padded, testNonce); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("padded: got %d, want 413", rec.Code)
	}
	if n := len(e.GetAllRoutes()); n != 0 {
		t.Fatalf("oversized request mutated engine: %d routes", n)
	}
}

func TestPostRoutesPortBoundaries(t *testing.T) {
	e := router.NewEngine()
	s, _ := newMutServer(e)
	for _, tc := range []struct {
		domain string
		port   string
		want   int
	}{
		{"p1.localhost", "1", http.StatusCreated},
		{"p65535.localhost", "65535", http.StatusCreated},
		{"p0.localhost", "0", http.StatusBadRequest},
		{"p65536.localhost", "65536", http.StatusBadRequest},
		{"pneg.localhost", "-1", http.StatusBadRequest},
	} {
		rec := postRoutes(s, fmt.Sprintf(`{"domain":%q,"port":%s}`, tc.domain, tc.port), testNonce)
		if rec.Code != tc.want {
			t.Fatalf("port %s: got %d, want %d: %s", tc.port, rec.Code, tc.want, rec.Body)
		}
	}
}

func TestRoutesMethodNotAllowed(t *testing.T) {
	s, _ := newMutServer(router.NewEngine())
	for _, m := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), m, "/routes", nil)
		req.Header.Set("X-Devtether-Nonce", testNonce)
		s.handleRoutes(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /routes: got %d, want 405", m, rec.Code)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), m, "/routes/app.localhost?inc=1", nil)
		req.SetPathValue("domain", "app.localhost")
		req.Header.Set("X-Devtether-Nonce", testNonce)
		s.handleRouteDomain(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /routes/{domain}: got %d, want 405", m, rec.Code)
		}
	}
}

func TestDeleteRouteIncarnationGrammar(t *testing.T) {
	queries := []string{
		"",                         // missing
		"inc=",                     // empty
		"inc",                      // bare
		"inc=0",                    // zero
		"inc=-1",                   // negative
		"inc=%2B1",                 // +1
		"inc=%201",                 // leading whitespace
		"inc=1%20",                 // trailing whitespace
		"inc=0x1",                  // hex
		"inc=1.0",                  // float
		"inc=1e1",                  // exponent
		"inc=010",                  // leading zero
		"inc=18446744073709551616", // MaxUint64 + 1
		"inc=99999999999999999999999",
		"inc=1&inc=2", // duplicate
		"inc=1&inc=1", // duplicate, same value
		"inc=abc",
		"Inc=1", // case variant key
	}
	e := newEngine(t, "app.localhost", 8080)
	inc := e.GetAllRoutes()[0].Incarnation
	s, tr := newMutServer(e)
	for _, q := range queries {
		if rec := deleteRoute(s, "app.localhost", q, testNonce); rec.Code != http.StatusBadRequest {
			t.Fatalf("query %q: got %d, want 400", q, rec.Code)
		}
	}
	if got, ok := e.Resolve("app.localhost"); !ok || got.Incarnation != inc {
		t.Fatal("invalid DELETE mutated engine")
	}
	if !tr.ObserveNow("app.localhost", inc, health.Observation{Status: health.StatusOnline, CheckedAt: time.Now()}) {
		t.Fatal("invalid DELETE mutated tracker registration")
	}
}

func TestDeleteRouteDomainValidation(t *testing.T) {
	e := newEngine(t, "app.localhost", 8080)
	inc := e.GetAllRoutes()[0].Incarnation
	s, _ := newMutServer(e)
	for _, d := range []string{"", "app.internal", " app.localhost", "app.localhost ", "a/b", "*.localhost", "localhost", "app.localhost:80"} {
		if rec := deleteRoute(s, d, fmt.Sprintf("inc=%d", inc), testNonce); rec.Code != http.StatusBadRequest {
			t.Fatalf("domain %q: got %d, want 400", d, rec.Code)
		}
	}
	if _, ok := e.Resolve("app.localhost"); !ok {
		t.Fatal("invalid DELETE mutated engine")
	}
}

func TestDeleteRouteStatusDistinctions(t *testing.T) {
	e := newEngine(t, "app.localhost", 8080)
	inc := e.GetAllRoutes()[0].Incarnation
	s, tr := newMutServer(e)

	// Valid but absent domain → 404.
	if rec := deleteRoute(s, "missing.localhost", "inc=1", testNonce); rec.Code != http.StatusNotFound {
		t.Fatalf("absent: got %d, want 404", rec.Code)
	}
	// Max uint64 is grammatically valid → reaches the engine.
	if rec := deleteRoute(s, "missing.localhost", "inc=18446744073709551615", testNonce); rec.Code != http.StatusNotFound {
		t.Fatalf("absent max uint64: got %d, want 404", rec.Code)
	}
	if rec := deleteRoute(s, "app.localhost", "inc=18446744073709551615", testNonce); rec.Code != http.StatusConflict {
		t.Fatalf("present max uint64: got %d, want 409", rec.Code)
	}
	// Present, wrong incarnation → 409, route + health intact.
	if rec := deleteRoute(s, "app.localhost", fmt.Sprintf("inc=%d", inc+1), testNonce); rec.Code != http.StatusConflict {
		t.Fatalf("wrong inc: got %d, want 409", rec.Code)
	}
	if _, ok := e.Resolve("app.localhost"); !ok {
		t.Fatal("409 removed the route")
	}
	// Present, wrong incarnation → 409, route + health intact.
	if rec := deleteRoute(s, "APP.localhost", fmt.Sprintf("inc=%d", inc+1), testNonce); rec.Code != http.StatusConflict {
		t.Fatalf("case-variant wrong inc: got %d, want 409", rec.Code)
	}
	// Case-variant domain with right incarnation → 204 (engine normalizes).
	rec := deleteRoute(s, "APP.localhost", fmt.Sprintf("inc=%d", inc), testNonce)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("right inc: got %d body %q, want 204 empty", rec.Code, rec.Body)
	}
	if _, ok := e.Resolve("app.localhost"); ok {
		t.Fatal("204 left the route in the engine")
	}
	// Tracker retired the incarnation: observations and re-registration of
	// the removed incarnation are rejected.
	if tr.ObserveNow("app.localhost", inc, health.Observation{Status: health.StatusOnline, CheckedAt: time.Now()}) {
		t.Fatal("tracker still accepts the removed incarnation")
	}
	// Second DELETE of the same incarnation → 404.
	if rec := deleteRoute(s, "app.localhost", fmt.Sprintf("inc=%d", inc), testNonce); rec.Code != http.StatusNotFound {
		t.Fatalf("repeat delete: got %d, want 404", rec.Code)
	}
}

// TestDeleteStaticAndLiveRoutes covers D14: static routes are removable
// from the running daemon exactly like live ones; nothing touches YAML (the
// daemon never holds a config writer).
func TestDeleteStaticAndLiveRoutes(t *testing.T) {
	e := newEngine(t, "static.localhost", 8080)
	s, _ := newMutServer(e)
	rec := postRoutes(s, `{"domain":"live.localhost","port":3000}`, testNonce)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: %d", rec.Code)
	}
	for _, rv := range e.GetAllRoutes() {
		if del := deleteRoute(s, rv.Domain, fmt.Sprintf("inc=%d", rv.Incarnation), testNonce); del.Code != http.StatusNoContent {
			t.Fatalf("DELETE %s (%s): got %d, want 204", rv.Domain, rv.Type, del.Code)
		}
	}
	if n := len(e.GetAllRoutes()); n != 0 {
		t.Fatalf("expected empty engine, got %d", n)
	}
}

// TestDeleteStaleIncarnationPreservesReplacement is the handler-level D15
// stale-confirmation race in scripted order: a caller listed inc A, the
// route was removed and re-created as inc B, then the stale DELETE arrives.
func TestDeleteStaleIncarnationPreservesReplacement(t *testing.T) {
	e := router.NewEngine()
	s, tr := newMutServer(e)

	var first RouteResponse
	rec := postRoutes(s, `{"domain":"app.localhost","port":3000}`, testNonce)
	_ = json.NewDecoder(rec.Body).Decode(&first)
	if del := deleteRoute(s, "app.localhost", fmt.Sprintf("inc=%d", first.Incarnation), testNonce); del.Code != http.StatusNoContent {
		t.Fatalf("first delete: %d", del.Code)
	}
	var second RouteResponse
	rec = postRoutes(s, `{"domain":"app.localhost","port":4000}`, testNonce)
	_ = json.NewDecoder(rec.Body).Decode(&second)
	if second.Incarnation <= first.Incarnation {
		t.Fatalf("incarnations not increasing: %d then %d", first.Incarnation, second.Incarnation)
	}
	tr.ObserveNow("app.localhost", second.Incarnation, health.Observation{Status: health.StatusOnline, CheckedAt: time.Now()})

	// Stale confirmation of the first incarnation → 409; replacement and
	// its health survive.
	if del := deleteRoute(s, "app.localhost", fmt.Sprintf("inc=%d", first.Incarnation), testNonce); del.Code != http.StatusConflict {
		t.Fatalf("stale delete: got %d, want 409", del.Code)
	}
	got, ok := e.Resolve("app.localhost")
	if !ok || got.Incarnation != second.Incarnation || got.Port != 4000 {
		t.Fatalf("replacement lost: %+v ok=%v", got, ok)
	}
	if st := tr.Snapshot([]string{"app.localhost"})[0].Status; st != health.StatusOnline {
		t.Fatalf("replacement health cleared: %s", st)
	}
}

func TestHandleRoutesIncludesIncarnation(t *testing.T) {
	e := newEngine(t, "app.localhost", 8080)
	inc := e.GetAllRoutes()[0].Incarnation
	s, _ := newMutServer(e)
	rec := httptest.NewRecorder()
	s.handleRoutes(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/routes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /routes: %d", rec.Code)
	}
	// Wire shape: beta.7 fields unchanged plus additive "incarnation".
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: %+v", rows)
	}
	for _, key := range []string{"domain", "serviceName", "port", "type", "status", "incarnation"} {
		if _, ok := rows[0][key]; !ok {
			t.Fatalf("missing key %q in %v", key, rows[0])
		}
	}
	if got := uint64(rows[0]["incarnation"].(float64)); got != inc {
		t.Fatalf("incarnation: got %d, want %d", got, inc)
	}
}
