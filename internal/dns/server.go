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
	"runtime"
	"strings"
	"sync"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/ivin-titus/devtether/internal/router"
	"github.com/miekg/dns"
)

// Server is an embedded DNS resolver that answers A record queries
// for domains registered in the DevTether routing table.
type Server struct {
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
	// Use a dedicated mux instead of the global DefaultServeMux
	// to avoid conflicts if multiple Server instances are created (e.g. in tests).
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleRequest)

	// Shut down when context is cancelled.
	// The server variable is local to avoid a data race between this
	// goroutine (reading) and the main goroutine (writing during fallback).
	shutdownStarted := make(chan struct{})
	var (
		serverMu sync.Mutex
		server   *dns.Server
	)

	go func() {
		<-ctx.Done()
		close(shutdownStarted)
		serverMu.Lock()
		srv := server
		serverMu.Unlock()
		if srv != nil {
			_ = srv.Shutdown()
		}
	}()

	// Try configured bind address.
	serverMu.Lock()
	server = &dns.Server{Addr: s.bind, Net: "udp", Handler: mux}
	srv := server
	serverMu.Unlock()
	
	err := srv.ListenAndServe()
	if err == nil || isShutdown(shutdownStarted) {
		return nil
	}

	// Fall back to 5353 on permission denied or address in use.
	if !netutil.IsRecoverable(err) {
		return fmt.Errorf("dns server failed: %w", err)
	}

	log.Printf("[dns] %s unavailable — falling back to 127.0.0.1:5353", s.bind)
	if netutil.IsPermissionError(err) {
		if runtime.GOOS == "linux" {
			log.Printf("[dns] to use port 53, run: sudo setcap cap_net_bind_service=+ep $(which devtether)")
		} else {
			log.Printf("[dns] to use port 53, run devtether with administrator privileges")
		}
	}

	s.bind = "127.0.0.1:5353"
	serverMu.Lock()
	server = &dns.Server{Addr: s.bind, Net: "udp", Handler: mux}
	srv = server
	serverMu.Unlock()
	log.Printf("[dns] listening on %s (tlds: %v)", s.bind, s.tlds)

	err = srv.ListenAndServe()
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
// receive a response. All other queries receive NXDOMAIN (per ADR-002).
func (s *Server) handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false
	m.Authoritative = true

	if r.Opcode != dns.OpcodeQuery {
		_ = w.WriteMsg(m)
		return
	}

	matched := false
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
			matched = true
		}
	}

	// Return NXDOMAIN for queries that matched no routes (ADR-002 contract).
	if !matched && len(m.Answer) == 0 {
		m.Rcode = dns.RcodeNameError
	}

	_ = w.WriteMsg(m)
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
