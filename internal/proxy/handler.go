package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/ivin-titus/devtether/internal/router"
)

type contextKey string

const (
	hostCtxKey   contextKey = "host"
	targetCtxKey contextKey = "target"
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
	errorLog      *throttleCache
	errorCooldown time.Duration
}

// NewHandler creates a proxy handler with the given resolver.
func NewHandler(resolver router.Resolver) *Handler {
	h := &Handler{
		resolver:      resolver,
		errorLog:      &throttleCache{},
		errorCooldown: 10 * time.Second,
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second

	h.rp = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			target, ok := pr.In.Context().Value(targetCtxKey).(router.Target)
			if ok {
				pr.SetURL(target.URL)
				pr.SetXForwarded()
				host, _ := pr.In.Context().Value(hostCtxKey).(string)
				pr.Out.Header.Set("X-Forwarded-Host", host)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				return
			}
			// ADR-008: If headers were already sent (mid-stream crash),
			// abort the TCP connection cleanly — never corrupt the payload.
			if lrw, ok := w.(*loggingResponseWriter); ok && lrw.HeadersSent() {
				panic(http.ErrAbortHandler)
			}
			host, _ := req.Context().Value(hostCtxKey).(string)
			target, ok := req.Context().Value(targetCtxKey).(router.Target)

			var targetURL string
			var port int
			if ok {
				targetURL = target.URL.String()
				port = target.Port
			}
			if port == 0 {
				port = 80 // Fallback generic assumption
			}

			h.logErrorThrottled(host, targetURL, err)

			trace := ""
			if logger.IsVerbose() {
				trace = err.Error()
			}

			renderErrorPage(w, http.StatusBadGateway, ErrorPageData{
				StatusCode: http.StatusBadGateway,
				StatusText: "Bad Gateway",
				Message:    "DevTether received the request, but the backend service refused the connection.",
				Details: []ErrorDetail{
					{Label: "Requested:", Value: host},
					{Label: "Target:", Value: targetURL},
				},
				Hint: &ErrorHint{
					Title:   "Did you start your backend?",
					Message: fmt.Sprintf("Make sure your application is actually running and listening on port %d.", port),
				},
				ErrorTrace: trace,
			})
		},
	}

	return h
}

// ServeHTTP routes incoming HTTP requests to the correct backend
// based on the Host header.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := netutil.NormalizeHost(r.Host)

	if host == "" {
		renderErrorPage(w, http.StatusBadRequest, ErrorPageData{
			StatusCode: http.StatusBadRequest,
			StatusText: "Bad Request",
			Message:    "DevTether could not process the request because the Host header was missing or invalid.",
			Hint: &ErrorHint{
				Title:   "Invalid Request",
				Message: "Ensure your HTTP client is sending a valid Host header.",
			},
		})
		return
	}

	target, ok := h.resolver.Resolve(host)
	if !ok {
		renderErrorPage(w, http.StatusNotFound, ErrorPageData{
			StatusCode: http.StatusNotFound,
			StatusText: "Route Not Found",
			Message:    fmt.Sprintf("DevTether could not find a routing rule for '%s'.", r.Host),
			Hint: &ErrorHint{
				Title:   "Route Not Configured",
				Message: "Check your devtether.yaml file and ensure the route is defined and spelling is correct. Run 'devtether routes' to see active routes.",
			},
		})
		return
	}

	ctx := context.WithValue(r.Context(), hostCtxKey, host)
	ctx = context.WithValue(ctx, targetCtxKey, target)
	h.rp.ServeHTTP(w, r.WithContext(ctx))
}

// logErrorThrottled logs a proxy error at most once per cooldown period
// per target to prevent log spam when a backend is down.
func (h *Handler) logErrorThrottled(host, targetURL string, err error) {
	if h.errorLog.LoggedRecently(targetURL, h.errorCooldown) {
		return
	}
	logger.New("proxy").Debug(fmt.Sprintf("%s → %s", host, targetURL), "error", err)
}

var logCacheLimit = 1024

// throttleCache is a bounded cache that prevents memory leaks when
// logging errors for dynamically generated or malicious target URLs.
type throttleCache struct {
	mu    sync.Mutex
	items map[string]time.Time
}

// LoggedRecently returns true if the targetURL was logged within the cooldown period.
// It implements a hybrid TTL + random eviction strategy to stay within logCacheLimit.
func (c *throttleCache) LoggedRecently(targetURL string, cooldown time.Duration) bool {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.items == nil {
		c.items = make(map[string]time.Time)
	}

	if last, ok := c.items[targetURL]; ok && now.Sub(last) < cooldown {
		return true // Suppress — logged recently.
	}

	// If full, random eviction (Go maps randomize range order) O(1)
	if len(c.items) >= logCacheLimit {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}

	c.items[targetURL] = now
	return false
}

// renderErrorPage executes the HTML error template and writes the HTTP response.
func renderErrorPage(w http.ResponseWriter, statusCode int, data ErrorPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := errorPageTemplate.Execute(w, data); err != nil {
		logger.New("proxy").Error("failed to render error page", err)
	}
}

