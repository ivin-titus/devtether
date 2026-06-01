// Package dns provides an embedded DNS resolver that answers A record
// queries for domains registered in the DevTether routing table.
//
// Design decisions (see ADR-002):
//   - Non-recursive: never forwards queries upstream. Unknown domains get NXDOMAIN.
//   - Loopback-only by default: binds to 127.0.0.1:53. LAN exposure requires explicit opt-in.
//   - Route-aware: only answers for domains that exist in the routing table.
//   - Does NOT support the ".local" TLD (reserved by mDNS, RFC 6762).
package dns

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/router"
	"github.com/miekg/dns"
)

// Server is an embedded DNS resolver that answers A record queries
// for domains registered in the DevTether routing table.
type Server struct {
	server   *dns.Server
	resolver router.Resolver
	tlds     []string
	bind     string
}

// NewServer creates a DNS server configured by the given DNSConfig.
// The resolver is used to determine which domains have active routes.
func NewServer(cfg config.DNSConfig, resolver router.Resolver) *Server {
	return &Server{
		resolver: resolver,
		tlds:     cfg.TLD,
		bind:     cfg.Bind,
	}
}

// Start begins listening for DNS queries. It blocks until the context
// is cancelled or a fatal error occurs.
//
// Port resolution order:
//  1. Configured bind address (default: 127.0.0.1:53)
//  2. If EACCES/EADDRINUSE → 127.0.0.1:5353
//  3. If 5353 also fails → returns error (DNS is non-fatal in up.go)
func (s *Server) Start(ctx context.Context) error {
	dns.HandleFunc(".", s.handleRequest)

	// Shut down when context is cancelled.
	shutdownStarted := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(shutdownStarted)
		if s.server != nil {
			s.server.Shutdown()
		}
	}()

	// Try configured bind address.
	s.server = &dns.Server{Addr: s.bind, Net: "udp"}
	err := s.server.ListenAndServe()
	if err == nil || isShutdown(shutdownStarted) {
		return nil
	}

	// Fall back to 5353 on permission denied or address in use.
	if !isRecoverableError(err) {
		return fmt.Errorf("dns server failed: %w", err)
	}

	log.Printf("[dns] %s unavailable — falling back to 127.0.0.1:5353", s.bind)
	if isPermissionError(err) {
		log.Printf("[dns] to use port 53, run: sudo setcap cap_net_bind_service=+ep $(which devtether)")
	}

	s.bind = "127.0.0.1:5353"
	s.server = &dns.Server{Addr: s.bind, Net: "udp"}
	log.Printf("[dns] listening on %s (tlds: %v)", s.bind, s.tlds)

	err = s.server.ListenAndServe()
	if err == nil || isShutdown(shutdownStarted) {
		return nil
	}
	return fmt.Errorf("dns server failed on fallback port: %w", err)
}

// isShutdown checks if the shutdown channel has been closed (non-blocking).
func isShutdown(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// handleRequest processes incoming DNS queries. Only A record queries
// for domains registered in the routing table under a configured TLD
// receive a response. All other queries receive NXDOMAIN.
func (s *Server) handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false
	m.Authoritative = true

	if r.Opcode != dns.OpcodeQuery {
		w.WriteMsg(m)
		return
	}

	for _, q := range m.Question {
		if q.Qtype != dns.TypeA {
			continue
		}

		// DNS names are FQDN with trailing dot — strip it for lookup.
		name := strings.TrimSuffix(strings.ToLower(q.Name), ".")

		if !s.matchesTLD(name) {
			continue
		}

		// Only answer for domains that actually have routes registered.
		if s.resolver.Resolve(name) == nil {
			continue
		}

		// Static routing always resolves to loopback.
		rr, err := dns.NewRR(fmt.Sprintf("%s A 127.0.0.1", q.Name))
		if err == nil {
			m.Answer = append(m.Answer, rr)
		}
	}

	w.WriteMsg(m)
}

// matchesTLD checks if the given hostname ends with one of the configured TLDs.
func (s *Server) matchesTLD(name string) bool {
	for _, tld := range s.tlds {
		if strings.HasSuffix(name, "."+tld) || name == tld {
			return true
		}
	}
	return false
}

// isRecoverableError returns true for errors that should trigger a port fallback.
func isRecoverableError(err error) bool {
	return isPermissionError(err) || isAddrInUse(err)
}

// isPermissionError checks if a network error is caused by EACCES or EPERM.
func isPermissionError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "operation not permitted")
}

// isAddrInUse checks if a network error is caused by EADDRINUSE.
func isAddrInUse(err error) bool {
	return strings.Contains(err.Error(), "address already in use")
}
