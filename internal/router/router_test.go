package router

import (
	"sort"
	"testing"
)

func TestEngine_AddAndResolve(t *testing.T) {
	e := NewEngine()

	if err := e.AddRoute("api.localhost", "api-service", 40123, RouteStatic); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	target := e.Resolve("api.localhost")
	if target == nil {
		t.Fatal("expected target for 'api.localhost', got nil")
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

	if target := e.Resolve("unknown.localhost"); target != nil {
		t.Fatalf("expected nil for unknown route, got %v", target)
	}
}

func TestEngine_RemoveRoute(t *testing.T) {
	e := NewEngine()
	e.AddRoute("api.localhost", "api", 8080, RouteStatic)
	e.RemoveRoute("api.localhost")

	if target := e.Resolve("api.localhost"); target != nil {
		t.Fatalf("expected nil after removal, got %v", target)
	}
}

func TestEngine_Domains(t *testing.T) {
	e := NewEngine()
	e.AddRoute("a.localhost", "a", 3000, RouteStatic)
	e.AddRoute("b.localhost", "b", 4000, RouteStatic)
	e.AddRoute("c.localhost", "c", 5000, RouteOrchestrated)

	domains := e.Domains()
	sort.Strings(domains)

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
	e.AddRoute("a.localhost", "a", 3000, RouteStatic)
	e.AddRoute("b.localhost", "b", 4000, RouteStatic)

	snapshot := e.GetAllRoutes()
	if len(snapshot) != 2 {
		t.Fatalf("expected 2 routes in snapshot, got %d", len(snapshot))
	}

	// Verify the snapshot is a copy — modifying it shouldn't affect the engine.
	delete(snapshot, "a.localhost")
	if e.Resolve("a.localhost") == nil {
		t.Fatal("deleting from snapshot affected the engine — snapshot must be a copy")
	}
}

func TestEngine_Concurrency(t *testing.T) {
	e := NewEngine()
	e.AddRoute("api.localhost", "api", 8080, RouteStatic)

	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 1000; i++ {
			_ = e.Resolve("api.localhost")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			e.AddRoute("web.localhost", "web", 8081+i, RouteStatic)
		}
		done <- true
	}()

	<-done
	<-done
	// If it didn't panic with concurrent map access, test passed.
}

// Verify Engine satisfies the Resolver interface at compile time.
var _ Resolver = (*Engine)(nil)
