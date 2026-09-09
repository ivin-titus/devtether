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
	"net"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/logger"
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

// Listen binds the DNS server to the configured port (with fallback) and returns the packet connection.
// Port resolution order:
//  1. Configured bind address (default: 127.0.0.1:53)
//  2. If EACCES/EADDRINUSE → 127.0.0.1:5353
//  3. If 5353 also fails → returns error
func (s *Server) Listen(ctx context.Context) (net.PacketConn, error) {
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp", s.bind)
	if err == nil {
		return pc, nil
	}

	// Fall back to 5353 on permission denied or address in use.
	if !netutil.IsRecoverable(err) {
		return nil, fmt.Errorf("dns server failed: %w", err)
	}

	log := logger.New("dns")
	log.Debug(fmt.Sprintf("%s unavailable — falling back to 127.0.0.1:5353", s.bind))
	if netutil.IsPermissionError(err) {
		if runtime.GOOS == "linux" {
			log.Debug("to use port 53, run: sudo setcap cap_net_bind_service=+ep $(which devtether)")
		} else {
			log.Debug("to use port 53, run devtether with administrator privileges")
		}
	}

	s.bind = "127.0.0.1:5353"
	pc, err = lc.ListenPacket(ctx, "udp", s.bind)
	if err == nil {
		return pc, nil
	}
	return nil, fmt.Errorf("dns server failed on fallback port: %w", err)
}

// Serve begins handling DNS queries on the provided PacketConn.
// It blocks until the context is cancelled.
func (s *Server) Serve(ctx context.Context, pc net.PacketConn) error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleRequest)

	server := &dns.Server{PacketConn: pc, Handler: mux}
	shutdownStarted := make(chan struct{})

	//nolint:gosec // Shutdown requires a fresh, uncancelled context.
	go func() {
		<-ctx.Done()
		close(shutdownStarted)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.ShutdownContext(shutdownCtx)
	}()

	logger.New("dns").Debug(fmt.Sprintf("listening on %s (tlds: %v)", pc.LocalAddr().String(), s.tlds))

	err := server.ActivateAndServe()
	if err == nil || isShutdown(shutdownStarted) {
		return nil
	}
	return err
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
	defer func() {
		if rv := recover(); rv != nil {
			logger.New("dns").Error("panic recovered in handler",
				fmt.Errorf("%v\n%s", rv, debug.Stack()))
		}
	}()

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
		// DNS names are FQDN with trailing dot — strip it for lookup.
		name := strings.TrimSuffix(strings.ToLower(q.Name), ".")

		if !s.matchesTLD(name) {
			continue
		}

		// Check route existence FIRST, independent of query type.
		if _, ok := s.resolver.Resolve(name); !ok {
			continue
		}
		matched = true // Route exists — prevents NXDOMAIN.

		// Only generate A records for TypeA queries.
		if q.Qtype == dns.TypeA {
			rr, err := dns.NewRR(fmt.Sprintf("%s A 127.0.0.1", q.Name))
			if err == nil {
				m.Answer = append(m.Answer, rr)
			}
		}
		// AAAA/HTTPS for valid routes → matched=true, 0 answers → NODATA (RFC 4074).
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
