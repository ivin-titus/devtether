package portman

import (
	"fmt"
	"net"
	"sync"
)

// Manager tracks allocated ports to prevent overlap within DevTether
type Manager struct {
	allocated map[int]bool
	mu        sync.Mutex
}

// NewManager creates a new Port Manager
func NewManager() *Manager {
	return &Manager{
		allocated: make(map[int]bool),
	}
}

// GetFreePort asks the OS for a free TCP port and reserves it in our internal map.
func (pm *Manager) GetFreePort() (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Try up to 5 times to get an unreserved port
	for i := 0; i < 5; i++ {
		addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("failed to resolve tcp address: %w", err)
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return 0, fmt.Errorf("failed to listen on port 0: %w", err)
		}

		port := l.Addr().(*net.TCPAddr).Port
		l.Close() // Immediately close it so the child process can use it

		// Ensure we haven't already assigned this port internally
		if !pm.allocated[port] {
			pm.allocated[port] = true
			return port, nil
		}
	}

	return 0, fmt.Errorf("could not find an unallocated port after 5 attempts")
}

// ReleasePort frees up a port in the application's internal tracker
func (pm *Manager) ReleasePort(port int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.allocated, port)
}
