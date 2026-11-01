package cli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/ivin-titus/devtether/internal/health"
	"github.com/ivin-titus/devtether/internal/router"
	"github.com/spf13/cobra"
)

func testCmd(ctx context.Context, out *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cmd.SetOut(out)
	return cmd
}

func listenEphemeral(t *testing.T) net.Listener {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return l
}

func reservePort(t *testing.T) int {
	t.Helper()
	l := listenEphemeral(t)
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// shortRuntimeDir points XDG_RUNTIME_DIR at a short /tmp path: macOS Unix
// sockets are limited to ~104 chars, which t.TempDir() paths exceed.
func shortRuntimeDir(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "dt")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
}

func TestPrintRoutesWithHealth(t *testing.T) {
	var buf bytes.Buffer
	printRoutes(&buf, []daemon.RouteResponse{
		{Domain: "b.localhost", ServiceName: "svc", Port: 8081, Type: "static", Status: "offline"},
		{Domain: "a.localhost", ServiceName: "svc", Port: 8080, Type: "static", Status: "online",
			LastChecked: time.Now().Add(-3 * time.Second).UTC().Format(time.RFC3339Nano)},
	})
	out := buf.String()
	if !strings.Contains(out, "STATUS") || !strings.Contains(out, "CHECKED") {
		t.Fatalf("missing health columns:\n%s", out)
	}
	if !strings.Contains(out, "OFFLINE") || !strings.Contains(out, "ONLINE") {
		t.Fatalf("missing statuses:\n%s", out)
	}
	if !strings.Contains(out, "ago") {
		t.Fatalf("missing relative timestamp:\n%s", out)
	}
	// Deterministic: input order is preserved by caller (daemon sorts).
	if strings.Index(out, "b.localhost") > strings.Index(out, "a.localhost") {
		t.Fatal("rows not rendered in input order")
	}
}

func TestPrintRoutesEmptyStatusIsUnknown(t *testing.T) {
	var buf bytes.Buffer
	printRoutes(&buf, []daemon.RouteResponse{
		{Domain: "app.localhost", ServiceName: "static", Port: 8080, Type: "static"},
	})
	if !strings.Contains(buf.String(), "UNKNOWN") {
		t.Fatalf("config-derived route must render UNKNOWN:\n%s", buf.String())
	}
}

func TestPrintCheckResults(t *testing.T) {
	var buf bytes.Buffer
	printCheckResults(&buf, []daemon.CheckResponse{
		{Domain: "up.localhost", Backend: "127.0.0.1:9000", Status: "online", Latency: 1.5},
		{Domain: "down.localhost", Backend: "127.0.0.1:9001", Status: "offline", Reason: "connection refused"},
	})
	out := buf.String()
	for _, want := range []string{"DOMAIN", "BACKEND", "CONN LATENCY", "up.localhost", "connection refused"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRunRoutesCheckDaemonDown(t *testing.T) {
	shortRuntimeDir(t)
	var buf bytes.Buffer
	err := runRoutesCheck(testCmd(context.Background(), &buf))
	if !errors.Is(err, ErrDaemonNotRunning) {
		t.Fatalf("expected ErrDaemonNotRunning, got %v", err)
	}
}

func TestRunRoutesCheckExitCodes(t *testing.T) {
	shortRuntimeDir(t)

	// Daemon with one live backend and one dead backend.
	e := router.NewEngine()
	l := listenEphemeral(t)
	livePort := l.Addr().(*net.TCPAddr).Port
	defer func() { _ = l.Close() }()
	deadPort := reservePort(t)
	if err := e.AddRoute("ok.localhost", "svc", livePort, router.RouteStatic); err != nil {
		t.Fatal(err)
	}
	if err := e.AddRoute("down.localhost", "svc", deadPort, router.RouteStatic); err != nil {
		t.Fatal(err)
	}

	srv := daemon.NewServer(e, health.NewTracker(), func() {}, "devtether.yaml")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("daemon Listen: %v", err)
	}
	go func() { _ = srv.Serve(ctx, ln) }()

	var buf bytes.Buffer
	err = runRoutesCheck(testCmd(ctx, &buf))
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Summary: 1 online, 1 offline, 0 unknown") {
		t.Fatalf("unexpected summary:\n%s", out)
	}
}
