package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client communicates with a running DevTether daemon over the Unix socket.
type Client struct {
	httpc *http.Client
}

// NewClient creates an IPC client that connects to the daemon's Unix socket.
func NewClient() *Client {
	return &Client{
		httpc: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					d := net.Dialer{Timeout: 2 * time.Second}
					return d.DialContext(ctx, "unix", SocketPath())
				},
			},
			Timeout: 5 * time.Second,
		},
	}
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

// ShutdownAndWait sends a shutdown request and returns the response for the
// caller to drain. The server's /shutdown handler keeps the HTTP connection
// open during the entire graceful shutdown (including proxy drain). The caller
// should drain the response body with io.Copy(io.Discard, resp.Body) — when
// the read returns (EOF), the daemon has fully stopped.
//
// The caller is responsible for closing resp.Body.
func (c *Client) ShutdownAndWait(ctx context.Context) (*http.Response, error) {
	// Use a separate client with a longer timeout to accommodate the full
	// shutdown sequence: IPC shutdown (10s) + proxy drain (5s) + buffer.
	longClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(ctx, "unix", SocketPath())
			},
		},
		Timeout: 20 * time.Second,
	}

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
