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
					var d net.Dialer
					return d.DialContext(ctx, "unix", SocketPath())
				},
			},
			Timeout: 5 * time.Second,
		},
	}
}

// ListRoutes fetches all active routes from the running daemon.
func (c *Client) ListRoutes() ([]RouteResponse, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://unix/routes", nil)
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
