package daemon

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestSocketClientAbsentLeafNoDial verifies the client-level fail-closed
// contract: with the runtime leaf absent, the dial path resolves the socket
// path first, surfaces ErrNoRuntimeDir (wrapping os.ErrNotExist), and never
// reaches the network dialer — a unix dial error can never carry ErrNoRuntimeDir,
// so the sentinel proves no connection attempt occurred.
func TestSocketClientAbsentLeafNoDial(t *testing.T) {
	xdg := xdgBase(t)
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	c := socketClient(time.Second)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("socketClient transport = %T, want *http.Transport", c.Transport)
	}
	conn, err := tr.DialContext(context.Background(), "unix", "devtether.sock")
	if conn != nil {
		_ = conn.Close()
		t.Fatal("DialContext returned a connection with an absent runtime leaf")
	}
	if !errors.Is(err, ErrNoRuntimeDir) {
		t.Fatalf("DialContext error = %v, want ErrNoRuntimeDir (no dial attempted)", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DialContext error = %v, want os.ErrNotExist wrapping preserved", err)
	}
}
