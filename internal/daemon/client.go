package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Client is used by the CLI commands to talk to the background DevTether daemon
type Client struct {
	httpc *http.Client
}

// NewClient establishes an HTTP client that communicates exclusively over the Unix socket.
func NewClient() *Client {
	return &Client{
		httpc: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("unix", SocketPath)
				},
			},
			Timeout: 5 * time.Second,
		},
	}
}

// AddService tells the daemon to start a command and map it to a domain.
func (c *Client) AddService(domain, command string) error {
	reqBody := AddRequest{Domain: domain, Command: command}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := c.httpc.Post("http://unix/services", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to contact devtether daemon (is it running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon returned error: %s", string(respBytes))
	}

	return nil
}

// RemoveService tells the daemon to stop a service and dump the route
func (c *Client) RemoveService(domain string) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://unix/services?domain=%s", domain), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("failed to contact devtether daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon returned error: %s", string(respBytes))
	}

	return nil
}

// ServiceResponse matches the daemon's list struct
type ServiceResponse struct {
	Domain      string `json:"domain"`
	ServiceName string `json:"serviceName"`
	Port        int    `json:"port"`
}

// ListServices fetches all active routed services from the daemon
func (c *Client) ListServices() ([]ServiceResponse, error) {
	resp, err := c.httpc.Get("http://unix/services")
	if err != nil {
		return nil, fmt.Errorf("failed to contact devtether daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon returned error: %s", string(respBytes))
	}

	var services []ServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&services); err != nil {
		return nil, fmt.Errorf("failed to decode daemon response: %w", err)
	}

	return services, nil
}
