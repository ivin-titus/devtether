// Package router provides the in-memory routing table that maps
// incoming hostnames to local backend targets. It is the shared
// data structure consumed by both the proxy and DNS engines.
package router

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
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

	// RouteLive is a route added at runtime via the IPC API. It is
	// ephemeral: it lives only in the running daemon and is discarded on
	// restart, unlike RouteStatic routes from devtether.yaml.
	RouteLive RouteType = "live"
)

// RouteView is a read-only snapshot of a route for external consumers.
type RouteView struct {
	Domain      string
	ServiceName string
	Port        int
	Type        RouteType
	// Incarnation identifies the current lifetime of this route. It is
	// issued by the engine on every insert and carried internally for
	// incarnation-aware health identity and conditional deletion.
	Incarnation uint64
}

// Target represents where a specific domain should be routed.
type Target struct {
	ServiceName string
	Port        int
	URL         *url.URL
	Type        RouteType
	// Incarnation identifies the current lifetime of this route (see
	// RouteView.Incarnation).
	Incarnation uint64
}

// Resolver looks up a routing target by hostname.
// This is the contract between the proxy/DNS engines and the routing table.
type Resolver interface {
	Resolve(host string) (Target, bool)
	Domains() []string
}

// ErrRouteExists is returned by AddRoute when the domain is already
// registered. Creation is strict: an existing route is never silently
// replaced.
var ErrRouteExists = errors.New("route already exists")

// Engine holds the mapping of incoming host domains to local Target
// configurations. It is thread-safe for concurrent reads and writes.
type Engine struct {
	routes  map[string]*Target
	nextInc uint64 // incarnation counter; issued under mu on every insert
	mu      sync.RWMutex
}

// NewEngine initializes an empty thread-safe routing table.
func NewEngine() *Engine {
	return &Engine{
		routes: make(map[string]*Target),
	}
}

// AddRoute maps an incoming domain to a local port with the given route type.
// Creation is atomic and strict: the existence check and insertion happen
// under a single write lock, so concurrent creates of the same domain yield
// exactly one success and one ErrRouteExists. On success it returns the
// route's incarnation, a unique identifier for this lifetime of the route.
func (e *Engine) AddRoute(domain, serviceName string, port int, routeType RouteType) (uint64, error) {
	domain = netutil.NormalizeHost(domain)
	if domain == "" {
		return 0, fmt.Errorf("route domain must not be empty")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.routes[domain]; exists {
		return 0, fmt.Errorf("%w: %s", ErrRouteExists, domain)
	}

	targetURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return 0, fmt.Errorf("failed to parse proxy target URL: %w", err)
	}

	e.nextInc++
	inc := e.nextInc
	e.routes[domain] = &Target{
		ServiceName: serviceName,
		Port:        port,
		URL:         targetURL,
		Type:        routeType,
		Incarnation: inc,
	}
	return inc, nil
}

// RemoveRoute deletes a domain mapping from the routing table, but only if
// the stored incarnation still equals expectedInc. This is the sole delete
// path: requiring the incarnation means a stale confirmation can never
// remove a concurrent replacement route. It returns the removed
// incarnation and whether a route was actually deleted; ok is false when
// the domain is absent, the incarnation does not match, or expectedInc is 0
// (0 is never a valid incarnation).
func (e *Engine) RemoveRoute(domain string, expectedInc uint64) (uint64, bool) {
	domain = netutil.NormalizeHost(domain)
	if expectedInc == 0 {
		return 0, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	t, ok := e.routes[domain]
	if !ok || t.Incarnation != expectedInc {
		return 0, false
	}
	delete(e.routes, domain)
	return t.Incarnation, true
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
		Incarnation: t.Incarnation,
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
	sort.Strings(domains)
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
			Incarnation: t.Incarnation,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Domain < views[j].Domain })
	return views
}
