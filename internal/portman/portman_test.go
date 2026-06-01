package portman

import (
	"sync"
	"testing"
)

func TestGetFreePort(t *testing.T) {
	pm := NewManager()

	port1, err := pm.GetFreePort()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if port1 <= 0 || port1 > 65535 {
		t.Errorf("Expected valid port number, got %d", port1)
	}

	if !pm.allocated[port1] {
		t.Errorf("Expected port %d to be marked as allocated", port1)
	}

	port2, err := pm.GetFreePort()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if port1 == port2 {
		t.Errorf("GetFreePort returned the same port twice: %d", port1)
	}
}

func TestReleasePort(t *testing.T) {
	pm := NewManager()

	port, err := pm.GetFreePort()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	pm.ReleasePort(port)

	if pm.allocated[port] {
		t.Errorf("Expected port %d to be removed from allocated map", port)
	}
}

func TestConcurrentAllocation(t *testing.T) {
	pm := NewManager()
	var wg sync.WaitGroup

	numWorkers := 100
	ports := make([]int, numWorkers)
	errors := make([]error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p, err := pm.GetFreePort()
			ports[idx] = p
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	seen := make(map[int]bool)
	for i := 0; i < numWorkers; i++ {
		if errors[i] != nil {
			t.Errorf("Worker %d returned error: %v", i, errors[i])
			continue
		}

		p := ports[i]
		if seen[p] {
			t.Errorf("Duplicate port assigned concurrently: %d", p)
		}
		seen[p] = true
	}
}
