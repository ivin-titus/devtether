package router

import (
	"testing"
)

func TestRoutingEngine(t *testing.T) {
	e := NewEngine()

	// Test Add
	err := e.AddRoute("api.internal", "api-service", 40123)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Test Get Exists
	target := e.GetTarget("api.internal")
	if target == nil {
		t.Fatalf("Expected target for 'api.internal', got nil")
	}

	if target.Port != 40123 {
		t.Errorf("Expected port 40123, got %d", target.Port)
	}
	if target.ServiceName != "api-service" {
		t.Errorf("Expected service name 'api-service', got '%s'", target.ServiceName)
	}
	if target.URL.String() != "http://127.0.0.1:40123" {
		t.Errorf("Expected URL 'http://127.0.0.1:40123', got '%s'", target.URL.String())
	}

	// Test Get Does Not Exist
	missing := e.GetTarget("unknown.internal")
	if missing != nil {
		t.Fatalf("Expected nil for unknown route, got %v", missing)
	}

	// Test Remove
	e.RemoveRoute("api.internal")
	removed := e.GetTarget("api.internal")
	if removed != nil {
		t.Fatalf("Expected nil after removal, got %v", removed)
	}
}

func TestRoutingEngine_Concurrency(t *testing.T) {
	e := NewEngine()

	// Add initial
	e.AddRoute("api.internal", "api", 8080)

	done := make(chan bool)

	// Reader coroutine
	go func() {
		for i := 0; i < 1000; i++ {
			_ = e.GetTarget("api.internal")
		}
		done <- true
	}()

	// Writer coroutine
	go func() {
		for i := 0; i < 1000; i++ {
			e.AddRoute("web.internal", "web", 8081+i)
		}
		done <- true
	}()

	<-done
	<-done

	// If it didn't panic with concurrent map writes, test passed.
}
