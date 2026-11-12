package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client communicates with a running DevTether daemon over the Unix socket.
type Client struct {
	httpc *http.Client
}

// socketClient builds an HTTP client bound to the daemon's Unix socket.
func socketClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Resolve the validated runtime directory on every dial: an
				// absent or unsafe directory fails here, before any connection
				// attempt, so no client can reach an unvalidated socket path.
				socketPath, err := SocketPath()
				if err != nil {
					return nil, err
				}
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: timeout,
	}
}

// NewClient creates an IPC client that connects to the daemon's Unix socket.
func NewClient() *Client {
	return &Client{httpc: socketClient(5 * time.Second)}
}

// ListRoutes fetches all active routes from the running daemon.
func (c *Client) ListRoutes(ctx context.Context) ([]RouteResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/routes", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact devtether daemon (is it running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var routes []RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return nil, fmt.Errorf("failed to decode daemon response: %w", err)
	}

	return routes, nil
}

// Status fetches daemon status from the running daemon.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/status", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact devtether daemon (is it running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode daemon response: %w", err)
	}

	return &status, nil
}

// Shutdown requests the daemon to shut down gracefully.
func (c *Client) Shutdown(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/shutdown", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if nonce, nonceErr := readNonce(); nonceErr == nil {
		req.Header.Set("X-Devtether-Nonce", nonce)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("failed to contact devtether daemon (is it running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	return nil
}

// CheckRoutes asks the daemon to run a fresh health check over its active
// routes and returns the per-route results. It fails when no daemon is
// reachable — callers must not silently fall back to another implementation.
func (c *Client) CheckRoutes(ctx context.Context) ([]CheckResponse, error) {
	// The daemon-side check budget is 5s; leave headroom for IPC overhead.
	client := socketClient(10 * time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/routes/check", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact devtether daemon (is it running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var results []CheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode daemon response: %w", err)
	}

	return results, nil
}

// ShutdownAndWait sends a shutdown request and returns the response for the
// caller to drain. The server's /shutdown handler flushes the response and
// closes the connection immediately. The caller should drain the response body
// with io.Copy(io.Discard, resp.Body), close it, and then explicitly poll
// daemon.WaitForExit() to know when the daemon process has actually exited.
//
// The caller is responsible for closing resp.Body.
func (c *Client) ShutdownAndWait(ctx context.Context) (*http.Response, error) {
	// Use a standard client timeout since the HTTP request itself completes
	// immediately, delegating the long wait to WaitForExit.
	longClient := socketClient(20 * time.Second)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/shutdown", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if nonce, nonceErr := readNonce(); nonceErr == nil {
		req.Header.Set("X-Devtether-Nonce", nonce)
	}
	resp, err := longClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	return resp, nil
}

// APIError is returned when the daemon answers a mutating request with a
// non-success HTTP status. It carries the status code and a trimmed body so
// callers can distinguish validation, auth, conflict, and missing-route
// failures explicitly instead of string matching.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("daemon returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("daemon returned status %d: %s", e.StatusCode, e.Body)
}

// apiError drains a bounded amount of the error response body and wraps it
// in an APIError. The body is diagnostic only; IPC error bodies are tiny.
func apiError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		body = nil
	}
	return &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
}

// createRoutePayload is the strict JSON body accepted by POST /routes.
type createRoutePayload struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
}

// AddRoute asks the running daemon to create a live route for domain →
// 127.0.0.1:port. It returns the created route including its incarnation,
// which clients must retain for conditional removal. Live routes are
// ephemeral: they are discarded when the daemon stops.
func (c *Client) AddRoute(ctx context.Context, domain string, port int) (*RouteResponse, error) {
	payload, err := json.Marshal(createRoutePayload{Domain: domain, Port: port})
	if err != nil {
		return nil, fmt.Errorf("failed to encode route: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/routes", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if nonce, nonceErr := readNonce(); nonceErr == nil {
		req.Header.Set("X-Devtether-Nonce", nonce)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact devtether daemon (is it running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return nil, apiError(resp)
	}

	var route RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&route); err != nil {
		return nil, fmt.Errorf("failed to decode daemon response: %w", err)
	}
	return &route, nil
}

// RemoveRoute asks the running daemon to remove the route for domain, but
// only if its incarnation still equals inc — the incarnation obtained from
// ListRoutes. A mismatch means the route changed after it was listed; the
// caller must re-list and re-confirm rather than retry blindly.
func (c *Client) RemoveRoute(ctx context.Context, domain string, inc uint64) error {
	u := fmt.Sprintf("http://unix/routes/%s?inc=%d", url.PathEscape(domain), inc)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if nonce, nonceErr := readNonce(); nonceErr == nil {
		req.Header.Set("X-Devtether-Nonce", nonce)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("failed to contact devtether daemon (is it running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return apiError(resp)
	}
	return nil
}
