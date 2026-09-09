package dns

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/router"
	mdns "github.com/miekg/dns"
)

// TestNXDOMAIN verifies that unregistered domains receive NXDOMAIN (ADR-002).
// Regression test for bug C2: previously returned NOERROR with empty answers.
func TestNXDOMAIN(t *testing.T) {
	engine := router.NewEngine()
	_ = engine.AddRoute("myapp.localhost", "myapp", 3000, router.RouteStatic)

	srv, addr := startTestServer(t, engine, []string{"localhost"})
	_ = srv

	c := new(mdns.Client)
	c.Timeout = 2 * time.Second

	tests := []struct {
		name      string
		query     string
		wantRcode int
		wantAns   bool
	}{
		{
			name:      "registered domain returns A record",
			query:     "myapp.localhost.",
			wantRcode: mdns.RcodeSuccess,
			wantAns:   true,
		},
		{
			name:      "unregistered domain returns NXDOMAIN",
			query:     "unknown.localhost.",
			wantRcode: mdns.RcodeNameError,
			wantAns:   false,
		},
		{
			name:      "wrong TLD returns NXDOMAIN",
			query:     "myapp.example.com.",
			wantRcode: mdns.RcodeNameError,
			wantAns:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := new(mdns.Msg)
			msg.SetQuestion(tt.query, mdns.TypeA)

			resp, _, err := c.Exchange(msg, addr)
			if err != nil {
				t.Fatalf("DNS query failed: %v", err)
			}

			if resp.Rcode != tt.wantRcode {
				t.Errorf("Rcode = %s, want %s",
					mdns.RcodeToString[resp.Rcode],
					mdns.RcodeToString[tt.wantRcode])
			}

			hasAnswers := len(resp.Answer) > 0
			if hasAnswers != tt.wantAns {
				t.Errorf("has answers = %v, want %v (answers: %v)",
					hasAnswers, tt.wantAns, resp.Answer)
			}
		})
	}
}

// TestDedicatedMux verifies that Server uses a dedicated ServeMux, not the
// global DefaultServeMux (bug C3). Two servers must coexist without conflicts.
func TestDedicatedMux(t *testing.T) {
	engine1 := router.NewEngine()
	_ = engine1.AddRoute("app1.localhost", "app1", 3001, router.RouteStatic)

	engine2 := router.NewEngine()
	_ = engine2.AddRoute("app2.localhost", "app2", 3002, router.RouteStatic)

	_, addr1 := startTestServer(t, engine1, []string{"localhost"})
	_, addr2 := startTestServer(t, engine2, []string{"localhost"})

	c := new(mdns.Client)
	c.Timeout = 2 * time.Second

	// Server 1 should only know about app1.
	msg := new(mdns.Msg)
	msg.SetQuestion("app1.localhost.", mdns.TypeA)
	resp, _, err := c.Exchange(msg, addr1)
	if err != nil {
		t.Fatalf("query to server 1 failed: %v", err)
	}
	if len(resp.Answer) == 0 {
		t.Error("server 1 should resolve app1.localhost")
	}

	// Server 2 should only know about app2.
	msg = new(mdns.Msg)
	msg.SetQuestion("app2.localhost.", mdns.TypeA)
	resp, _, err = c.Exchange(msg, addr2)
	if err != nil {
		t.Fatalf("query to server 2 failed: %v", err)
	}
	if len(resp.Answer) == 0 {
		t.Error("server 2 should resolve app2.localhost")
	}

	// Server 1 should NOT resolve app2 (proves muxes are independent).
	msg = new(mdns.Msg)
	msg.SetQuestion("app2.localhost.", mdns.TypeA)
	resp, _, err = c.Exchange(msg, addr1)
	if err != nil {
		t.Fatalf("cross-query failed: %v", err)
	}
	if resp.Rcode != mdns.RcodeNameError {
		t.Errorf("server 1 should NXDOMAIN for app2, got %s",
			mdns.RcodeToString[resp.Rcode])
	}
}

// TestConcurrentStartShutdown verifies no data race on the server field (bug C1).
// Run with -race to confirm.
func TestConcurrentStartShutdown(t *testing.T) {
	engine := router.NewEngine()
	_ = engine.AddRoute("test.localhost", "test", 8080, router.RouteStatic)

	cfg := config.DNSConfig{
		TLD:  []string{"localhost"},
		Bind: ephemeralAddr(t),
	}
	srv := NewServer(cfg, engine)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		pc, err := srv.Listen(ctx)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- srv.Serve(ctx, pc)
	}()

	// Give the server time to bind, then immediately cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5 seconds")
	}
}

// startTestServer creates and starts a DNS server on an ephemeral port.
// The server is automatically shut down when the test finishes.
func startTestServer(t *testing.T, engine *router.Engine, tlds []string) (*Server, string) {
	t.Helper()

	addr := ephemeralAddr(t)
	cfg := config.DNSConfig{
		TLD:  tlds,
		Bind: addr,
	}
	srv := NewServer(cfg, engine)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		pc, err := srv.Listen(ctx)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- srv.Serve(ctx, pc)
	}()

	// Wait for the server to be ready by polling.
	deadline := time.Now().Add(2 * time.Second)
	c := new(mdns.Client)
	c.Timeout = 100 * time.Millisecond
	for time.Now().Before(deadline) {
		msg := new(mdns.Msg)
		msg.SetQuestion("probe.test.", mdns.TypeA)
		if _, _, err := c.Exchange(msg, addr); err == nil {
			return srv, addr
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Check if the server errored during startup.
	select {
	case err := <-errCh:
		t.Fatalf("server failed to start: %v", err)
	default:
	}

	t.Fatal("server did not become ready within 2 seconds")
	return nil, ""
}

// ephemeralAddr returns a "127.0.0.1:<port>" address using a kernel-assigned
// ephemeral port. The port is freed before returning so the DNS server can bind it.
func ephemeralAddr(t *testing.T) string {
	t.Helper()
	//nolint:noctx // Test listener does not require context cancellation
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get ephemeral port: %v", err)
	}
	addr := conn.LocalAddr().String()
	_ = conn.Close()
	return addr
}

// TestMatchesTLD validates the TLD matching logic.
func TestMatchesTLD(t *testing.T) {
	srv := &Server{tlds: []string{"localhost", "test"}}

	tests := []struct {
		name  string
		host  string
		match bool
	}{
		{"subdomain match", "app.localhost", true},
		{"exact TLD", "localhost", true},
		{"different TLD", "app.test", true},
		{"no match", "app.example.com", false},
		{"partial match", "app.localhostx", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := srv.matchesTLD(tt.host)
			if got != tt.match {
				t.Errorf("matchesTLD(%q) = %v, want %v", tt.host, got, tt.match)
			}
		})
	}
}

// TestHandleRequestNonQuery verifies that non-query opcodes get an empty response.
func TestHandleRequestNonQuery(t *testing.T) {
	engine := router.NewEngine()
	_, addr := startTestServer(t, engine, []string{"localhost"})

	c := new(mdns.Client)
	c.Timeout = 2 * time.Second

	// Send an IQUERY (inverse query) — should get empty response, not panic.
	msg := new(mdns.Msg)
	msg.SetQuestion("test.localhost.", mdns.TypeA)
	msg.Opcode = mdns.OpcodeIQuery

	resp, _, err := c.Exchange(msg, addr)
	if err != nil {
		// Some DNS implementations may reject non-standard opcodes;
		// the important thing is no panic or data race.
		t.Logf("non-query response error (acceptable): %v", err)
		return
	}

	if len(resp.Answer) > 0 {
		t.Error("non-query opcode should not produce answers")
	}
}

// TestNonARecordQuery verifies that non-A queries (e.g. AAAA, MX) get NOERROR (NODATA) for valid routes.
func TestNonARecordQuery(t *testing.T) {
	engine := router.NewEngine()
	_ = engine.AddRoute("myapp.localhost", "myapp", 3000, router.RouteStatic)

	_, addr := startTestServer(t, engine, []string{"localhost"})

	c := new(mdns.Client)
	c.Timeout = 2 * time.Second

	// Query AAAA for a domain that has an A record — should get NOERROR (NODATA).
	msg := new(mdns.Msg)
	msg.SetQuestion("myapp.localhost.", mdns.TypeAAAA)

	resp, _, err := c.Exchange(msg, addr)
	if err != nil {
		t.Fatalf("AAAA query failed: %v", err)
	}

	if resp.Rcode != mdns.RcodeSuccess {
		t.Errorf("AAAA query Rcode = %s, want NOERROR",
			mdns.RcodeToString[resp.Rcode])
	}

	if len(resp.Answer) > 0 {
		t.Error("AAAA query should not produce answers")
	}
}

func TestCaseSensitivity(t *testing.T) {
	engine := router.NewEngine()
	_ = engine.AddRoute("myapp.localhost", "myapp", 3000, router.RouteStatic)

	_, addr := startTestServer(t, engine, []string{"localhost"})

	c := new(mdns.Client)
	c.Timeout = 2 * time.Second

	// DNS is case-insensitive per RFC 4343.
	for _, qname := range []string{"MyApp.Localhost.", "MYAPP.LOCALHOST.", "myapp.LOCALHOST."} {
		t.Run(fmt.Sprintf("query=%s", qname), func(t *testing.T) {
			msg := new(mdns.Msg)
			msg.SetQuestion(qname, mdns.TypeA)

			resp, _, err := c.Exchange(msg, addr)
			if err != nil {
				t.Fatalf("query failed: %v", err)
			}

			if len(resp.Answer) == 0 {
				t.Errorf("expected A record for %s (case-insensitive), got none", qname)
			}
		})
	}
}
