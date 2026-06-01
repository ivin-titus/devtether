package dns

import (
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"
)

// Server encapsulates the embedded DNS server
type Server struct {
	server *dns.Server
}

// NewServer initializes the DNS server to listen on all interfaces UDP port 53
func NewServer() *Server {
	return &Server{
		server: &dns.Server{
			Addr: "0.0.0.0:53",
			Net:  "udp",
		},
	}
}

// Start boots the DNS listener globally for domains mapped to this router
func (s *Server) Start() error {
	dns.HandleFunc(".", handleDNSRequest)

	log.Printf("[DNS] Local DNS Resolver listening on %s\n", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil {
		log.Printf("[DNS] Permission denied on 0.0.0.0:53 (or port in use). Falling back to 127.0.0.1:53")

		// Recreate server struct as miekg/dns servers cannot be reused after failing to listen
		s.server = &dns.Server{
			Addr: "127.0.0.1:53",
			Net:  "udp",
		}

		if err2 := s.server.ListenAndServe(); err2 != nil {
			return fmt.Errorf("failed to start DNS server: %w", err2)
		}
	}
	return nil
}

// Stop gracefully shuts down the DNS server
func (s *Server) Stop() error {
	return s.server.Shutdown()
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	if r.Opcode != dns.OpcodeQuery {
		w.WriteMsg(m)
		return
	}

	for _, q := range m.Question {
		name := strings.ToLower(q.Name)

		// Check if it's an A record query for our supported wildcards
		if q.Qtype == dns.TypeA && (strings.HasSuffix(name, ".internal.") || strings.HasSuffix(name, ".local.")) {
			ip := getLocalIP()
			rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
			if err == nil {
				m.Answer = append(m.Answer, rr)
			}
		}
	}

	w.WriteMsg(m)
}

// getLocalIP returns the non-loopback local IP of the host
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return getFallbackIP()
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func getFallbackIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}
