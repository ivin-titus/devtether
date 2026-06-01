package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/ivin-titus/devtether/internal/portman"
	"github.com/ivin-titus/devtether/internal/process"
	"github.com/ivin-titus/devtether/internal/router"
)

const SocketPath = "/tmp/devtether.sock"

// Server runs an HTTP API over a UNIX socket for IPC communication with CLI
type Server struct {
	router *router.Engine
	pm     *portman.Manager
	sup    *process.Supervisor
}

// NewServer initializes the IPC Daemon
func NewServer(r *router.Engine, p *portman.Manager, s *process.Supervisor) *Server {
	return &Server{
		router: r,
		pm:     p,
		sup:    s,
	}
}

// Start begins listening on the Unix domain socket for CLI commands
func (s *Server) Start() error {
	// Clean up dead socket if exists
	if err := os.RemoveAll(SocketPath); err != nil {
		return fmt.Errorf("failed to clear old socket: %w", err)
	}

	listener, err := net.Listen("unix", SocketPath)
	if err != nil {
		return fmt.Errorf("failed to bind unix socket: %w", err)
	}

	// Make socket accessible to host user
	if err := os.Chmod(SocketPath, 0777); err != nil {
		log.Printf("[Daemon] Warning: Failed to chmod socket: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/services", s.handleServices)

	log.Printf("[Daemon] IPC Server listening on %s\n", SocketPath)
	return http.Serve(listener, mux)
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listServices(w, r)
	case http.MethodPost:
		s.addService(w, r)
	case http.MethodDelete:
		s.removeService(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

type AddRequest struct {
	Domain  string `json:"domain"`
	Command string `json:"command"`
}

func (s *Server) addService(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	port, err := s.pm.GetFreePort()
	if err != nil {
		http.Error(w, "Failed to allocate port", http.StatusInternalServerError)
		return
	}

	if err := s.sup.StartService(req.Domain, req.Command, port); err != nil {
		s.pm.ReleasePort(port)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.router.AddRoute(req.Domain, req.Domain, port); err != nil {
		_ = s.sup.StopService(req.Domain)
		s.pm.ReleasePort(port)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "Service %s successfully mapped to %s on port %d", req.Domain, req.Domain, port)
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	routes := s.router.GetAllRoutes()

	type ServiceResponse struct {
		Domain      string `json:"domain"`
		ServiceName string `json:"serviceName"`
		Port        int    `json:"port"`
	}

	var response []ServiceResponse
	for domain, target := range routes {
		response = append(response, ServiceResponse{
			Domain:      domain,
			ServiceName: target.ServiceName,
			Port:        target.Port,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) removeService(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "domain parameter required", http.StatusBadRequest)
		return
	}

	target := s.router.GetTarget(domain)
	if target != nil {
		_ = s.sup.StopService(target.ServiceName)
		s.pm.ReleasePort(target.Port)
		s.router.RemoveRoute(domain)
	}

	w.WriteHeader(http.StatusOK)
}
