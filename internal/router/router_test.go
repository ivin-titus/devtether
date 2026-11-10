package router

import (
	"errors"
	"sync"
	"testing"
)

func TestEngine_AddAndResolve(t *testing.T) {
	e := NewEngine()

	if _, err := e.AddRoute("api.localhost", "api-service", 40123, RouteStatic); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	target, ok := e.Resolve("api.localhost")
	if !ok {
		t.Fatal("expected target for 'api.localhost', got ok=false")
	}
	if target.Port != 40123 {
		t.Errorf("expected port 40123, got %d", target.Port)
	}
	if target.ServiceName != "api-service" {
		t.Errorf("expected service name 'api-service', got %q", target.ServiceName)
	}
	if target.URL.String() != "http://127.0.0.1:40123" {
		t.Errorf("expected URL 'http://127.0.0.1:40123', got %q", target.URL.String())
	}
	if target.Type != RouteStatic {
		t.Errorf("expected route type %q, got %q", RouteStatic, target.Type)
	}
}

func TestEngine_ResolveNotFound(t *testing.T) {
	e := NewEngine()

	if _, ok := e.Resolve("unknown.localhost"); ok {
		t.Fatalf("expected ok=false for unknown route, got ok=true")
	}
}

func TestEngine_RemoveRoute(t *testing.T) {
	e := NewEngine()
	inc, _ := e.AddRoute("Api.Localhost", "api", 8080, RouteStatic)

	removed, ok := e.RemoveRoute("API.LOCALHOST", inc)
	if !ok {
		t.Fatal("expected removal to succeed with the correct incarnation")
	}
	if removed != inc {
		t.Errorf("RemoveRoute returned inc %d, want %d", removed, inc)
	}

	if _, ok := e.Resolve("api.localhost"); ok {
		t.Fatalf("expected ok=false after removal, got ok=true")
	}
}

func TestEngine_AddRouteRejectsEmptyDomain(t *testing.T) {
	if _, err := NewEngine().AddRoute("", "api", 8080, RouteStatic); err == nil {
		t.Fatal("AddRoute() error = nil, want empty domain error")
	}
}

func TestEngine_Domains(t *testing.T) {
	e := NewEngine()
	_, _ = e.AddRoute("a.localhost", "a", 3000, RouteStatic)
	_, _ = e.AddRoute("b.localhost", "b", 4000, RouteStatic)
	_, _ = e.AddRoute("c.localhost", "c", 5000, RouteOrchestrated)

	domains := e.Domains()

	if len(domains) != 3 {
		t.Fatalf("expected 3 domains, got %d", len(domains))
	}
	expected := []string{"a.localhost", "b.localhost", "c.localhost"}
	for i, d := range domains {
		if d != expected[i] {
			t.Errorf("domain[%d] = %q, want %q", i, d, expected[i])
		}
	}
}

func TestEngine_GetAllRoutes(t *testing.T) {
	e := NewEngine()
	_, _ = e.AddRoute("a.localhost", "a", 3000, RouteStatic)
	_, _ = e.AddRoute("b.localhost", "b", 4000, RouteStatic)

	snapshot := e.GetAllRoutes()
	if len(snapshot) != 2 {
		t.Fatalf("expected 2 routes in snapshot, got %d", len(snapshot))
	}

	// Verify the snapshot is a copy — modifying it shouldn't affect the engine.
	originalPort := snapshot[0].Port
	snapshot[0].Port = 9999
	if target, ok := e.Resolve(snapshot[0].Domain); !ok {
		t.Fatal("target should still exist in engine")
	} else if target.Port == 9999 {
		t.Fatal("modifying snapshot affected the engine — snapshot must be a copy")
	} else if target.Port != originalPort {
		t.Fatalf("engine port changed unexpectedly: got %d, want %d", target.Port, originalPort)
	}
}

func TestEngine_AddRouteReturnsUniqueIncarnations(t *testing.T) {
	e := NewEngine()

	inc1, err := e.AddRoute("a.localhost", "a", 3000, RouteStatic)
	if err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	inc2, err := e.AddRoute("b.localhost", "b", 4000, RouteStatic)
	if err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	if inc1 == 0 || inc2 == 0 {
		t.Fatalf("incarnations must be non-zero, got %d and %d", inc1, inc2)
	}
	if inc1 == inc2 {
		t.Fatalf("incarnations must be unique, got %d twice", inc1)
	}

	// Resolve must carry the incarnation.
	target, ok := e.Resolve("a.localhost")
	if !ok {
		t.Fatal("expected target for 'a.localhost'")
	}
	if target.Incarnation != inc1 {
		t.Errorf("Resolve incarnation = %d, want %d", target.Incarnation, inc1)
	}

	// GetAllRoutes must carry the incarnation too.
	for _, view := range e.GetAllRoutes() {
		want := inc1
		if view.Domain == "b.localhost" {
			want = inc2
		}
		if view.Incarnation != want {
			t.Errorf("RouteView for %s has incarnation %d, want %d", view.Domain, view.Incarnation, want)
		}
	}
}

func TestEngine_AddRouteRejectsDuplicate(t *testing.T) {
	e := NewEngine()
	inc1, err := e.AddRoute("api.localhost", "api", 8080, RouteStatic)
	if err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	// Same domain, different case — normalization must still conflict.
	_, err = e.AddRoute("API.LOCALHOST", "other", 9090, RouteLive)
	if !errors.Is(err, ErrRouteExists) {
		t.Fatalf("AddRoute duplicate error = %v, want ErrRouteExists", err)
	}

	// The original route must be untouched.
	target, ok := e.Resolve("api.localhost")
	if !ok {
		t.Fatal("original route missing after rejected duplicate")
	}
	if target.Port != 8080 || target.Type != RouteStatic || target.Incarnation != inc1 {
		t.Errorf("original route mutated by rejected duplicate: %+v", target)
	}
}

func TestEngine_ReAddGetsNewIncarnation(t *testing.T) {
	e := NewEngine()
	inc1, _ := e.AddRoute("api.localhost", "api", 8080, RouteStatic)
	if _, ok := e.RemoveRoute("api.localhost", inc1); !ok {
		t.Fatal("removal failed")
	}

	inc2, err := e.AddRoute("api.localhost", "api", 9090, RouteLive)
	if err != nil {
		t.Fatalf("re-add after removal: %v", err)
	}
	if inc2 <= inc1 {
		t.Errorf("re-added incarnation %d must exceed removed %d", inc2, inc1)
	}

	target, _ := e.Resolve("api.localhost")
	if target.Incarnation != inc2 || target.Type != RouteLive {
		t.Errorf("resolved target = %+v, want incarnation %d type %q", target, inc2, RouteLive)
	}
}

func TestEngine_StaleIncarnationCannotRemoveReplacement(t *testing.T) {
	e := NewEngine()
	inc1, err := e.AddRoute("api.localhost", "api", 8080, RouteStatic)
	if err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	if _, ok := e.RemoveRoute("api.localhost", inc1); !ok {
		t.Fatal("initial removal failed")
	}

	// Re-creating the same domain issues a strictly newer incarnation.
	inc2, err := e.AddRoute("api.localhost", "api", 9090, RouteLive)
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if inc2 <= inc1 {
		t.Fatalf("replacement incarnation %d must exceed removed %d", inc2, inc1)
	}

	// A stale confirmation carrying the removed incarnation must not delete
	// the replacement route.
	if _, ok := e.RemoveRoute("api.localhost", inc1); ok {
		t.Fatal("RemoveRoute with stale incarnation deleted the replacement route")
	}

	// The replacement must survive intact and be visible in both views.
	target, ok := e.Resolve("api.localhost")
	if !ok {
		t.Fatal("replacement route missing after stale removal attempt")
	}
	if target.Incarnation != inc2 || target.Port != 9090 || target.Type != RouteLive {
		t.Errorf("replacement target = %+v, want incarnation %d port 9090 type %q", target, inc2, RouteLive)
	}
	views := e.GetAllRoutes()
	if len(views) != 1 || views[0].Incarnation != inc2 {
		t.Fatalf("GetAllRoutes = %+v, want exactly one route with incarnation %d", views, inc2)
	}
}

func TestEngine_RemoveRouteRejectsWrongIncarnation(t *testing.T) {
	e := NewEngine()
	inc1, _ := e.AddRoute("api.localhost", "api", 8080, RouteStatic)

	// A stale incarnation must not delete the live route.
	if _, ok := e.RemoveRoute("api.localhost", inc1+1); ok {
		t.Fatal("RemoveRoute with wrong incarnation succeeded")
	}
	if _, ok := e.Resolve("api.localhost"); !ok {
		t.Fatal("route deleted by mismatched incarnation")
	}

	// Zero is never a valid incarnation.
	if _, ok := e.RemoveRoute("api.localhost", 0); ok {
		t.Fatal("RemoveRoute with incarnation 0 succeeded")
	}

	// Unknown domain.
	if _, ok := e.RemoveRoute("unknown.localhost", inc1); ok {
		t.Fatal("RemoveRoute on unknown domain succeeded")
	}
}

func TestEngine_ConcurrentCreateSameDomain(t *testing.T) {
	e := NewEngine()

	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	var successes, conflicts int

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := e.AddRoute("race.localhost", "svc", 8080, RouteLive)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else if errors.Is(err, ErrRouteExists) {
				conflicts++
			} else {
				t.Errorf("unexpected AddRoute error: %v", err)
			}
		}()
	}
	wg.Wait()

	if successes != 1 || conflicts != n-1 {
		t.Fatalf("concurrent creates: got %d successes, %d conflicts; want 1 and %d", successes, conflicts, n-1)
	}
}

func TestEngine_ConcurrentRemoveSameIncarnation(t *testing.T) {
	e := NewEngine()
	inc, _ := e.AddRoute("race.localhost", "svc", 8080, RouteStatic)

	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	var removed int

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, ok := e.RemoveRoute("race.localhost", inc); ok {
				mu.Lock()
				removed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if removed != 1 {
		t.Fatalf("concurrent removes: %d succeeded, want exactly 1", removed)
	}
}

func TestEngine_ConcurrentAddRemoveLookup(t *testing.T) {
	e := NewEngine()
	_, _ = e.AddRoute("stable.localhost", "svc", 8080, RouteStatic)

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = e.Resolve("stable.localhost")
			_ = e.Domains()
			_ = e.GetAllRoutes()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = e.AddRoute("churn.localhost", "svc", 8080, RouteLive)
			// Remove only when present; incarnation is read under lock via Resolve.
			if target, ok := e.Resolve("churn.localhost"); ok {
				_, _ = e.RemoveRoute("churn.localhost", target.Incarnation)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = e.AddRoute("other.localhost", "svc", 8081, RouteLive)
		}
	}()

	wg.Wait()
	// Test passes if no race condition or panic occurs during concurrent access.
}

func TestEngine_Concurrency(t *testing.T) {
	e := NewEngine()
	_, _ = e.AddRoute("api.localhost", "api", 8080, RouteStatic)

	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 1000; i++ {
			_, _ = e.Resolve("api.localhost")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			_, _ = e.AddRoute("web.localhost", "web", 8081+i, RouteStatic)
		}
		done <- true
	}()

	<-done
	<-done
	// Test passes if no race condition or panic occurs during concurrent access.
}

// Verify Engine satisfies the Resolver interface at compile time.
var _ Resolver = (*Engine)(nil)
