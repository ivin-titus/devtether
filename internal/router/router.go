// Package router provides the in-memory routing table that maps
// incoming hostnames to local backend targets. It is the shared
// data structure consumed by both the proxy and DNS engines.
package router

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// RouteType identifies how a route was registered.
type RouteType string

const (
	// RouteStatic is a route defined in the "routes:" config section.
	RouteStatic RouteType = "static"

	// RouteOrchestrated is a route managed by the orchestration engine (Phase 2).
	RouteOrchestrated RouteType = "orchestrated"
)

// Target represents where a specific domain should be routed.
type Target struct {
	ServiceName string
	Port        int
	URL         *url.URL
	Type        RouteType
}

// Resolver looks up a routing target by hostname.
// This is the contract between the proxy/DNS engines and the routing table.
type Resolver interface {
	Resolve(host string) *Target
	Domains() []string
}

// Engine holds the mapping of incoming host domains to local Target
// configurations. It is thread-safe for concurrent reads and writes.
type Engine struct {
	routes map[string]*Target
	mu     sync.RWMutex
}

// NewEngine initializes an empty thread-safe routing table.
func NewEngine() *Engine {
	return &Engine{
		routes: make(map[string]*Target),
	}
}

// AddRoute maps an incoming domain to a local port with the given route type.
func (e *Engine) AddRoute(domain, serviceName string, port int, routeType RouteType) error {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	e.mu.Lock()
	defer e.mu.Unlock()

	targetURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("failed to parse proxy target URL: %w", err)
	}

	e.routes[domain] = &Target{
		ServiceName: serviceName,
		Port:        port,
		URL:         targetURL,
		Type:        routeType,
	}
	return nil
}

// RemoveRoute deletes a domain mapping from the routing table.
func (e *Engine) RemoveRoute(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.routes, domain)
}

// Resolve returns the Target for a specific domain. Returns nil if not found.
// This method satisfies the Resolver interface.
func (e *Engine) Resolve(host string) *Target {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.routes[host]
}

// Domains returns a list of all registered domain names.
// Used by the DNS engine to determine which queries to answer.
func (e *Engine) Domains() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	domains := make([]string, 0, len(e.routes))
	for d := range e.routes {
		domains = append(domains, d)
	}
	return domains
}

// GetAllRoutes returns a snapshot of the current routing map.
// The returned map is a copy and safe to read without holding the lock.
func (e *Engine) GetAllRoutes() map[string]*Target {
	e.mu.RLock()
	defer e.mu.RUnlock()

	snapshot := make(map[string]*Target, len(e.routes))
	for k, v := range e.routes {
		snapshot[k] = v
	}
	return snapshot
}
