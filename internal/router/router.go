// Package router provides the in-memory routing table that maps
// incoming hostnames to local backend targets. It is the shared
// data structure consumed by both the proxy and DNS engines.
package router

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/ivin-titus/devtether/internal/netutil"
)

// RouteType identifies how a route was registered.
type RouteType string

const (
	// RouteStatic is a route defined in the "routes:" config section.
	RouteStatic RouteType = "static"

	// RouteOrchestrated is a route managed by the orchestration engine
	RouteOrchestrated RouteType = "orchestrated"
)

// RouteView is a read-only snapshot of a route for external consumers.
type RouteView struct {
	Domain      string
	ServiceName string
	Port        int
	Type        RouteType
}

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
	Resolve(host string) (Target, bool)
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
	domain = netutil.NormalizeHost(domain)
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

// Resolve returns the Target for a specific domain. Returns a boolean indicating if found.
// This method satisfies the Resolver interface.
func (e *Engine) Resolve(host string) (Target, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	t, ok := e.routes[host]
	if !ok {
		return Target{}, false
	}
	u := *t.URL // Deep copy
	return Target{
		ServiceName: t.ServiceName,
		Port:        t.Port,
		URL:         &u,
		Type:        t.Type,
	}, true
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
// The returned slice is safe to read without holding the lock.
func (e *Engine) GetAllRoutes() []RouteView {
	e.mu.RLock()
	defer e.mu.RUnlock()

	views := make([]RouteView, 0, len(e.routes))
	for domain, t := range e.routes {
		views = append(views, RouteView{
			Domain:      domain,
			ServiceName: t.ServiceName,
			Port:        t.Port,
			Type:        t.Type,
		})
	}
	return views
}
