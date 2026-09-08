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
	_ = e.AddRoute("api.localhost", "api", 8080, RouteStatic)
	e.RemoveRoute("api.localhost")

	if _, ok := e.Resolve("api.localhost"); ok {
		t.Fatalf("expected ok=false after removal, got ok=true")
	}
}

func TestEngine_Domains(t *testing.T) {
	e := NewEngine()
	_ = e.AddRoute("a.localhost", "a", 3000, RouteStatic)
	_ = e.AddRoute("b.localhost", "b", 4000, RouteStatic)
	_ = e.AddRoute("c.localhost", "c", 5000, RouteOrchestrated)

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
	_ = e.AddRoute("a.localhost", "a", 3000, RouteStatic)
	_ = e.AddRoute("b.localhost", "b", 4000, RouteStatic)

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

func TestEngine_Concurrency(t *testing.T) {
	e := NewEngine()
	_ = e.AddRoute("api.localhost", "api", 8080, RouteStatic)

	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 1000; i++ {
			_, _ = e.Resolve("api.localhost")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			_ = e.AddRoute("web.localhost", "web", 8081+i, RouteStatic)
		}
		done <- true
	}()

	<-done
	<-done
	// If it didn't panic with concurrent map access, test passed.
}

// Verify Engine satisfies the Resolver interface at compile time.
var _ Resolver = (*Engine)(nil)
