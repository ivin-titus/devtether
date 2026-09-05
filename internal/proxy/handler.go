package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/ivin-titus/devtether/internal/router"
)

// Handler implements http.Handler. It inspects the Host header,
// resolves the target via the routing table, and reverse-proxies
// the request to the local backend.
type Handler struct {
	resolver router.Resolver

	// rp is the shared reverse proxy engine.
	rp *httputil.ReverseProxy

	// errorLog tracks recently logged backend errors to prevent log spam.
	// Key: target URL string, Value: last time the error was logged.
	errorLog      sync.Map
	errorCooldown time.Duration
}

// NewHandler creates a proxy handler with the given resolver.
func NewHandler(resolver router.Resolver) *Handler {
	h := &Handler{
		resolver:      resolver,
		errorCooldown: 10 * time.Second,
	}

	h.rp = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			host := sanitizeHost(pr.In.Host)
			target := h.resolver.Resolve(host)
			if target != nil {
				pr.SetURL(target.URL)
				pr.SetXForwarded()
				// Override X-Forwarded-Host to use our sanitized host string
				pr.Out.Header.Set("X-Forwarded-Host", host)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			host := sanitizeHost(req.Host)
			target := h.resolver.Resolve(host)

			var targetURL string
			var port int
			if target != nil {
				targetURL = target.URL.String()
				port = target.Port
			}
			if port == 0 {
				port = 80 // Fallback generic assumption
			}

			h.logErrorThrottled(host, targetURL, err)

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)

			trace := ""
			if logger.IsVerbose() {
				trace = err.Error()
			}

			data := ErrorPageData{
				Host:       host,
				Target:     targetURL,
				TargetPort: fmt.Sprintf("%d", port),
				ErrorTrace: trace,
			}

			if err := errorPageTemplate.Execute(w, data); err != nil {
				logger.New("proxy").Error("failed to render error page", err)
			}
		},
	}

	return h
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

	h.rp.ServeHTTP(w, r)
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
	logger.New("proxy").Debug(fmt.Sprintf("%s → %s", host, targetURL), "error", err)
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
