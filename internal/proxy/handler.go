package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/ivin-titus/devtether/internal/router"
)

// Handler implements http.Handler. It inspects the Host header,
// resolves the target via the routing table, and reverse-proxies
// the request to the local backend.
type Handler struct {
	resolver router.Resolver

	// errorLog tracks recently logged backend errors to prevent log spam.
	// Key: target URL string, Value: last time the error was logged.
	errorLog   sync.Map
	errorCooldown time.Duration
}

// NewHandler creates a proxy handler with the given resolver.
func NewHandler(resolver router.Resolver) *Handler {
	return &Handler{
		resolver:      resolver,
		errorCooldown: 10 * time.Second,
	}
}

// ServeHTTP routes incoming HTTP requests to the correct backend
// based on the Host header.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := sanitizeHost(r.Host)

	if host == "" {
		http.Error(w, "devtether: missing or invalid Host header", http.StatusBadRequest)
		return
	}

	target := h.resolver.Resolve(host)
	if target == nil {
		http.Error(w, "devtether: no route found for '"+host+"'", http.StatusNotFound)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target.URL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Forwarded-Host", host)
		req.Header.Set("X-Forwarded-Proto", "http")
		if clientIP := r.RemoteAddr; clientIP != "" {
			if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
				req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
			} else {
				req.Header.Set("X-Forwarded-For", clientIP)
			}
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		h.logErrorThrottled(host, target.URL.String(), err)
		http.Error(w, "devtether: bad gateway (backend may be down or starting)", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

// logErrorThrottled logs a proxy error at most once per cooldown period
// per target to prevent log spam when a backend is down.
func (h *Handler) logErrorThrottled(host, targetURL string, err error) {
	now := time.Now()
	if lastRaw, ok := h.errorLog.Load(targetURL); ok {
		if last, ok := lastRaw.(time.Time); ok && now.Sub(last) < h.errorCooldown {
			return // Suppress — logged recently.
		}
	}
	h.errorLog.Store(targetURL, now)
	log.Printf("[proxy] %s → %s: %v", host, targetURL, err)
}

// sanitizeHost strips the port suffix from a Host header value and
// validates that it is not empty or malicious.
// Example: "portfolio.localhost:8080" → "portfolio.localhost"
func sanitizeHost(host string) string {
	// Strip port suffix if present.
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}

	return strings.ToLower(host)
}
