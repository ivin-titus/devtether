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

func TestPrintRoutesUnknownFootnote(t *testing.T) {
	var buf bytes.Buffer
	printRoutes(&buf, []daemon.RouteResponse{
		{Domain: "app.localhost", ServiceName: "live", Port: 3000, Type: "live"},
		{Domain: "web.localhost", ServiceName: "static", Port: 8080, Type: "static", Status: "online"},
	})
	out := buf.String()
	if !strings.Contains(out, "UNKNOWN") {
		t.Fatalf("missing UNKNOWN row:\n%s", out)
	}
	if !strings.Contains(out, "not yet checked") || !strings.Contains(out, "not OFFLINE") {
		t.Fatalf("missing UNKNOWN footnote:\n%s", out)
	}
	// The footnote must not imply every proxied request is conclusive.
	if !strings.Contains(out, "not every proxied request is conclusive") {
		t.Fatalf("footnote missing conclusive-observation precision:\n%s", out)
	}

	// No footnote when every route has a conclusive observation.
	buf.Reset()
	printRoutes(&buf, []daemon.RouteResponse{
		{Domain: "web.localhost", ServiceName: "static", Port: 8080, Type: "static", Status: "online"},
	})
	if strings.Contains(buf.String(), "not yet checked") {
		t.Fatalf("footnote shown without UNKNOWN:\n%s", buf.String())
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
	if _, err := e.AddRoute("ok.localhost", "svc", livePort, router.RouteStatic); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddRoute("down.localhost", "svc", deadPort, router.RouteStatic); err != nil {
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

// TestStartupTargetsCarryIncarnation pins the construction-site contract:
// startup probe targets must carry the route's incarnation or their
// observations are silently rejected.
func TestStartupTargetsCarryIncarnation(t *testing.T) {
	e := router.NewEngine()
	inc, err := e.AddRoute("app.localhost", "svc", 8080, router.RouteStatic)
	if err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	targets := startupTargets(e)
	if len(targets) != 1 || targets[0].Incarnation != inc {
		t.Fatalf("startupTargets lost incarnation: %+v", targets)
	}
}

// startTestDaemon serves e over the real IPC socket in a short runtime dir
// and waits for Serve's cleanup on test exit so runtime files never leak
// into a later test's directory.
func startTestDaemon(t *testing.T, e *router.Engine) *health.Tracker {
	t.Helper()
	shortRuntimeDir(t)
	tr := health.NewTracker()
	for _, rv := range e.GetAllRoutes() {
		tr.Register(rv.Domain, rv.Incarnation)
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv := daemon.NewServer(e, tr, cancel, "devtether.yaml")
	ln, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("daemon Listen: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = srv.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return tr
}

func removeCmd(out *bytes.Buffer, stdin string, yes bool) *cobra.Command {
	cmd := testCmd(context.Background(), out)
	cmd.Flags().BoolP("yes", "y", false, "")
	if yes {
		_ = cmd.Flags().Set("yes", "true")
	}
	cmd.SetIn(strings.NewReader(stdin))
	return cmd
}

// TestRoutesHelpHealthWording pins the user-facing health explanation:
// UNKNOWN means no conclusive observation yet, and not every proxied
// request produces one.
func TestRoutesHelpHealthWording(t *testing.T) {
	for _, want := range []string{
		"not yet checked",
		"not OFFLINE",
		"conclusive observation",
		"not every proxied request produces a conclusive observation",
	} {
		if !strings.Contains(routesCmd.Long, want) {
			t.Fatalf("routes help missing %q", want)
		}
	}
	if strings.Contains(routesAddCmd.Long, "the first proxied request") {
		t.Fatal("routes add help implies the first request is always conclusive")
	}
	if !strings.Contains(routesAddCmd.Long, "conclusive observation") {
		t.Fatal("routes add help missing conclusive-observation wording")
	}
}

func TestRoutesSubcommandWiring(t *testing.T) {
	var add, remove *cobra.Command
	for _, c := range routesCmd.Commands() {
		switch c.Name() {
		case "add":
			add = c
		case "remove":
			remove = c
		}
	}
	if add == nil || remove == nil {
		t.Fatal("routes add/remove subcommands not registered")
	}
	if f := remove.Flags().ShorthandLookup("y"); f == nil || f.Name != "yes" {
		t.Fatal("remove must expose -y/--yes")
	}
	if remove.Flags().Lookup("force") != nil {
		t.Fatal("remove must not expose --force")
	}
	// Bare routes keeps its --check flag.
	if routesCmd.Flags().Lookup("check") == nil {
		t.Fatal("bare routes lost --check")
	}
}

func TestRoutesMutationsRequireDaemon(t *testing.T) {
	shortRuntimeDir(t)
	var buf bytes.Buffer
	if err := runRoutesAdd(testCmd(context.Background(), &buf), []string{"app.localhost", "3000"}); !errors.Is(err, ErrDaemonNotRunning) {
		t.Fatalf("add without daemon: %v", err)
	}
	if err := runRoutesRemove(removeCmd(&buf, "", true), []string{"app.localhost"}); !errors.Is(err, ErrDaemonNotRunning) {
		t.Fatalf("remove without daemon: %v", err)
	}
}

func TestRoutesAddInvalidPortArg(t *testing.T) {
	shortRuntimeDir(t)
	var buf bytes.Buffer
	err := runRoutesAdd(testCmd(context.Background(), &buf), []string{"app.localhost", "abc"})
	if err == nil || errors.Is(err, ErrDaemonNotRunning) {
		t.Fatalf("expected local port parse error, got %v", err)
	}
}

func TestRoutesAddCreatesLiveRoute(t *testing.T) {
	e := router.NewEngine()
	startTestDaemon(t, e)
	var buf bytes.Buffer
	if err := runRoutesAdd(testCmd(context.Background(), &buf), []string{"app.localhost", "3000"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	got, ok := e.Resolve("app.localhost")
	if !ok || got.Type != router.RouteLive || got.Port != 3000 {
		t.Fatalf("engine: %+v ok=%v", got, ok)
	}
	if !strings.Contains(buf.String(), "ephemeral") {
		t.Fatalf("missing ephemeral notice:\n%s", buf.String())
	}
	// Registration must not imply backend availability: the output has to
	// state the initial UNKNOWN health, the daemon-only scope, and that only
	// a conclusive observation (not every proxied request) resolves it.
	for _, want := range []string{"running daemon", "UNKNOWN", "never written to devtether.yaml", "conclusive observation"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("add output missing %q:\n%s", want, buf.String())
		}
	}
	if strings.Contains(buf.String(), "the first proxied request") {
		t.Fatalf("add output implies the first request is always conclusive:\n%s", buf.String())
	}
	// Incarnation is an internal conditional-delete token — it must stay
	// opaque in CLI output.
	if strings.Contains(buf.String(), "incarnation") {
		t.Fatalf("CLI output leaked incarnation:\n%s", buf.String())
	}
	// Duplicate surfaces a clear, domain-identifying error rather than the
	// raw daemon 409 body.
	err := runRoutesAdd(testCmd(context.Background(), &buf), []string{"app.localhost", "4000"})
	if err == nil || !strings.Contains(err.Error(), "already registered") || !strings.Contains(err.Error(), "app.localhost") {
		t.Fatalf("duplicate add: %v", err)
	}
}

// TestRoutesAddRejectsNonCanonicalDomain mirrors the remove-side contract:
// the literal argument is validated before normalization or IPC, so port
// suffixes, trailing dots, whitespace and wildcards never reach the daemon.
func TestRoutesAddRejectsNonCanonicalDomain(t *testing.T) {
	shortRuntimeDir(t)
	var buf bytes.Buffer
	for _, arg := range []string{"app.localhost:80", "app.localhost.", "app.localhost ", "*.localhost"} {
		err := runRoutesAdd(testCmd(context.Background(), &buf), []string{arg, "3000"})
		if err == nil {
			t.Fatalf("arg %q: expected validation error, got nil", arg)
		}
		if errors.Is(err, ErrDaemonNotRunning) {
			t.Fatalf("arg %q: validation must precede daemon contact, got %v", arg, err)
		}
	}
}

// TestRoutesAddAcceptsUppercaseHostname pins the intended case behavior:
// DNS names are case-insensitive, so a valid uppercase hostname is
// accepted and normalized into the engine.
func TestRoutesAddAcceptsUppercaseHostname(t *testing.T) {
	e := router.NewEngine()
	startTestDaemon(t, e)
	var buf bytes.Buffer
	if err := runRoutesAdd(testCmd(context.Background(), &buf), []string{"APP.localhost", "3000"}); err != nil {
		t.Fatalf("add uppercase: %v", err)
	}
	if _, ok := e.Resolve("app.localhost"); !ok {
		t.Fatal("uppercase domain was not normalized into the engine")
	}
}

func TestRoutesRemoveNonTTYWithoutYesFailsSafe(t *testing.T) {
	e := router.NewEngine()
	if _, err := e.AddRoute("app.localhost", "static", 3000, router.RouteStatic); err != nil {
		t.Fatal(err)
	}
	startTestDaemon(t, e)
	var buf bytes.Buffer
	// Even an affirmative answer on a non-terminal stdin must not mutate.
	err := runRoutesRemove(removeCmd(&buf, "y\n", false), []string{"app.localhost"})
	if err == nil || !strings.Contains(err.Error(), "-y/--yes") {
		t.Fatalf("expected non-TTY guidance error, got %v", err)
	}
	if _, ok := e.Resolve("app.localhost"); !ok {
		t.Fatal("non-TTY remove without -y mutated the engine")
	}
}

func TestRoutesRemoveNotFound(t *testing.T) {
	startTestDaemon(t, router.NewEngine())
	var buf bytes.Buffer
	err := runRoutesRemove(removeCmd(&buf, "", true), []string{"absent.localhost"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

// TestRoutesRemoveStaticAndLiveWithYes covers D14: both route types are
// removable from the running daemon with -y; static removal never touches
// devtether.yaml, and each type gets its tailored warning.
func TestRoutesRemoveStaticAndLiveWithYes(t *testing.T) {
	dir := t.TempDir()
	root, rootErr := os.OpenRoot(dir)
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	t.Cleanup(func() { _ = root.Close() })
	yamlBody := []byte("routes:\n  static.localhost: 3000\n")
	if writeErr := root.WriteFile("devtether.yaml", yamlBody, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	before, beforeErr := root.Stat("devtether.yaml")
	if beforeErr != nil {
		t.Fatal(beforeErr)
	}

	e := router.NewEngine()
	if _, err := e.AddRoute("static.localhost", "static", 3000, router.RouteStatic); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddRoute("live.localhost", "live", 4000, router.RouteLive); err != nil {
		t.Fatal(err)
	}
	startTestDaemon(t, e)

	var buf bytes.Buffer
	if err := runRoutesRemove(removeCmd(&buf, "", true), []string{"static.localhost"}); err != nil {
		t.Fatalf("remove static: %v", err)
	}
	if !strings.Contains(buf.String(), "returns on restart") {
		t.Fatalf("missing static warning:\n%s", buf.String())
	}
	buf.Reset()
	if err := runRoutesRemove(removeCmd(&buf, "", true), []string{"LIVE.localhost"}); err != nil {
		t.Fatalf("remove live: %v", err)
	}
	if !strings.Contains(buf.String(), "ephemeral") {
		t.Fatalf("missing live warning:\n%s", buf.String())
	}
	if n := len(e.GetAllRoutes()); n != 0 {
		t.Fatalf("expected empty engine, got %d", n)
	}
	after, afterErr := root.Stat("devtether.yaml")
	if afterErr != nil {
		t.Fatal(afterErr)
	}
	got, readErr := root.ReadFile("devtether.yaml")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, yamlBody) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("static removal modified devtether.yaml")
	}
}

// TestConfirmRouteRemovalAnswers covers the prompt reader once a TTY has been
// established: only y/yes accepts; decline, empty, and EOF abort.
func TestConfirmRouteRemovalAnswers(t *testing.T) {
	target := &daemon.RouteResponse{Domain: "app.localhost", Type: "live", Port: 3000}
	for input, want := range map[string]bool{
		"y\n": true, "YES\n": true, " yes \n": true,
		"n\n": false, "\n": false, "maybe\n": false, "": false, "y": false,
	} {
		var buf bytes.Buffer
		if got := confirmRouteRemoval(&buf, "app.localhost", target, strings.NewReader(input)); got != want {
			t.Fatalf("input %q: got %v, want %v", input, got, want)
		}
	}
}

// TestRoutesRemoveStaleConfirmation scripts the D15 race at the CLI's
// mutation step: the incarnation captured from ListRoutes goes stale when
// the route is replaced, the conditional DELETE returns 409, the CLI maps
// it to a re-confirm error, and the replacement survives untouched.
func TestRoutesRemoveStaleConfirmation(t *testing.T) {
	e := router.NewEngine()
	tr := startTestDaemon(t, e)
	client := daemon.NewClient()
	ctx := context.Background()

	if _, err := client.AddRoute(ctx, "app.localhost", 3000); err != nil {
		t.Fatal(err)
	}
	listed, err := client.ListRoutes(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: %+v %v", listed, err)
	}
	staleInc := listed[0].Incarnation

	// Replacement lands between listing and the confirmed DELETE.
	if _, ok := e.RemoveRoute("app.localhost", staleInc); !ok {
		t.Fatal("setup remove failed")
	}
	tr.Unregister("app.localhost", staleInc)
	replacement, err := client.AddRoute(ctx, "app.localhost", 4000)
	if err != nil {
		t.Fatal(err)
	}

	rmErr := client.RemoveRoute(ctx, "app.localhost", staleInc)
	mapped := routeRemoveError("app.localhost", rmErr)
	if mapped == nil || !strings.Contains(mapped.Error(), "re-confirm") {
		t.Fatalf("expected re-confirm error, got %v", mapped)
	}
	got, ok := e.Resolve("app.localhost")
	if !ok || got.Incarnation != replacement.Incarnation || got.Port != 4000 {
		t.Fatalf("replacement lost: %+v ok=%v", got, ok)
	}
	// Non-409 errors pass through unchanged.
	other := &daemon.APIError{StatusCode: 404, Body: "route not found"}
	if routeRemoveError("x.localhost", other) != error(other) {
		t.Fatal("non-409 error must pass through")
	}
}

// TestRoutesRemoveRejectsNonCanonicalDomain proves the CLI validates the
// literal argument before normalization or IPC: NormalizeHost would silently
// repair "app.localhost:80" (strip the port) and "app.localhost." (strip the
// trailing dot), so the literal must be rejected first. No daemon is needed
// — validation precedes any socket contact.
func TestRoutesRemoveRejectsNonCanonicalDomain(t *testing.T) {
	shortRuntimeDir(t)
	var buf bytes.Buffer
	for _, arg := range []string{"app.localhost:80", "app.localhost.", "app.localhost ", "*.localhost"} {
		err := runRoutesRemove(removeCmd(&buf, "", true), []string{arg})
		if err == nil {
			t.Fatalf("arg %q: expected validation error, got nil", arg)
		}
		if errors.Is(err, ErrDaemonNotRunning) {
			t.Fatalf("arg %q: validation must precede daemon contact, got %v", arg, err)
		}
	}
}

// TestRoutesRemoveInvalidDomainLeavesRoute confirms rejected arguments never
// reach the daemon mutation path: the route stays in the engine.
func TestRoutesRemoveInvalidDomainLeavesRoute(t *testing.T) {
	e := router.NewEngine()
	if _, err := e.AddRoute("app.localhost", "static", 3000, router.RouteStatic); err != nil {
		t.Fatal(err)
	}
	startTestDaemon(t, e)
	var buf bytes.Buffer
	for _, arg := range []string{"app.localhost:80", "app.localhost."} {
		if err := runRoutesRemove(removeCmd(&buf, "", true), []string{arg}); err == nil {
			t.Fatalf("arg %q: expected error", arg)
		}
	}
	if _, ok := e.Resolve("app.localhost"); !ok {
		t.Fatal("rejected remove arguments mutated the engine")
	}
}
