package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"

	"github.com/ivin-titus/devtether/internal/router"
)

// Server encapsulates the HTTP Reverse Proxy that drives traffic to targeted internal ports
type Server struct {
	Engine *router.Engine
}

// NewServer initializes the proxy logic linked with a Routing Engine map
func NewServer(engine *router.Engine) *Server {
	return &Server{
		Engine: engine,
	}
}

// ServeHTTP implements the http.Handler interface. It inspects the Host header
// and redirects traffic if a match in the Routing Engine is found.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host

	target := s.Engine.GetTarget(host)
	if target == nil {
		http.Error(w, "DevTether: Service not found for domain '"+host+"'", http.StatusNotFound)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target.URL)

	// Update headers to allow backend to know it was proxy'd
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Forwarded-Host", host)
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		log.Printf("[Proxy Error] %s routing to %s: %v", host, target.URL.String(), err)
		http.Error(w, "DevTether: Bad Gateway (Backend service may be down or starting)", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}
