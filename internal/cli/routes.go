package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/spf13/cobra"
)

// ExitError is a command failure carrying a specific process exit code,
// mapped to os.Exit in main without bypassing Cobra's RunE error path.
type ExitError struct {
	Code int
	msg  string
}

// Error returns the diagnostic message printed on stderr.
func (e *ExitError) Error() string { return e.msg }

func init() {
	rootCmd.AddCommand(routesCmd)
	routesCmd.Flags().Bool("check", false, "run a fresh health check against active route backends via the daemon")
	routesCmd.AddCommand(routesAddCmd, routesRemoveCmd)
	routesRemoveCmd.Flags().BoolP("yes", "y", false, "skip the removal confirmation prompt")
}

var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "Show all configured or active routes",
	Long: `If the DevTether daemon is running, fetches live routes from it.
Otherwise, reads routes directly from devtether.yaml.

Routes can be mutated at runtime without a daemon restart:
'routes add' creates an ephemeral live route that is discarded on daemon
stop and never written to devtether.yaml. 'routes remove' removes a static
or live route from the running daemon only — a removed static route returns
on the next daemon start because devtether.yaml is never modified.

Health status is observational and separate from route registration and
DNS resolution: a route can be registered and resolvable while its backend
health is still UNKNOWN. UNKNOWN means "not yet checked" — no conclusive
health observation exists yet; it is not OFFLINE. Live routes added after
startup do not receive the daemon's startup health check. A conclusive
observation may come from proxied traffic or an explicit 'routes --check';
not every proxied request produces a conclusive observation.

With --check, asks the daemon to probe every active route's backend once
and prints TCP connection latency per route. Requires a running daemon.

Exit codes: 1 = execution or IPC failure. With --check: 0 = all backends
online; 2 = one or more backends offline or unchecked.`,
	RunE: runRoutes,
}

func runRoutes(cmd *cobra.Command, args []string) error {
	check, _ := cmd.Flags().GetBool("check")
	if check {
		return runRoutesCheck(cmd)
	}

	// If --config is passed, skip daemon check completely.
	if !cmd.Flags().Changed("config") {
		// Try the daemon first.
		client := daemon.NewClient()
		routes, err := client.ListRoutes(cmd.Context())
		if err == nil {
			sort.Slice(routes, func(i, j int) bool {
				return routes[i].Domain < routes[j].Domain
			})
			printRoutes(cmd.OutOrStdout(), routes)
			return nil
		}

		// Only fall back to reading config file if daemon is unreachable.
		if !errors.Is(err, syscall.ECONNREFUSED) && !errors.Is(err, os.ErrNotExist) {
			return withRootDaemonHint(err)
		}
	}

	// Daemon not running or config flag passed — fall back to reading config file.
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		// errors.Is unwraps through fmt.Errorf %w chains.
		// os.IsNotExist does NOT unwrap — never use it with wrapped errors.
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("No %s found and daemon is not running.\n", configPath)
			return nil
		}
		return fmt.Errorf("config error: %w", err)
	}

	if len(cfg.Routes) == 0 {
		fmt.Println("No routes defined in devtether.yaml.")
		return nil
	}

	// Convert config routes to the same format. Health status is not inferred
	// without a daemon — the empty Status renders as UNKNOWN.
	var configRoutes []daemon.RouteResponse
	for domain, port := range cfg.Routes {
		configRoutes = append(configRoutes, daemon.RouteResponse{
			Domain:      domain,
			ServiceName: "static",
			Port:        port,
			Type:        "static",
		})
	}
	sort.Slice(configRoutes, func(i, j int) bool {
		return configRoutes[i].Domain < configRoutes[j].Domain
	})
	printRoutes(cmd.OutOrStdout(), configRoutes)
	return nil
}

// runRoutesCheck forces a fresh daemon-side health check and renders a
// per-route table plus a summary. It never falls back to config: a daemon-
// backed check cannot be faked offline.
func runRoutesCheck(cmd *cobra.Command) error {
	client := daemon.NewClient()
	results, err := client.CheckRoutes(cmd.Context())
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
			daemon.CleanStaleSocket()
			return ErrDaemonNotRunning
		}
		return withRootDaemonHint(err)
	}

	out := cmd.OutOrStdout()
	printCheckResults(out, results)

	var offline, unknown int
	for _, r := range results {
		switch r.Status {
		case "offline":
			offline++
		case "unknown":
			unknown++
		}
	}
	_, _ = fmt.Fprintf(out, "\nSummary: %d online, %d offline, %d unknown\n",
		len(results)-offline-unknown, offline, unknown)

	if offline+unknown > 0 {
		return &ExitError{Code: 2, msg: "one or more backends are unavailable or unchecked"}
	}
	return nil
}

func printRoutes(w io.Writer, routes []daemon.RouteResponse) {
	if len(routes) == 0 {
		_, _ = fmt.Fprintln(w, "No active routes.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tPORT\tTYPE\tSTATUS\tCHECKED")
	_, _ = fmt.Fprintln(tw, "------\t----\t----\t------\t-------")
	unknown := false
	for _, r := range routes {
		status := strings.ToUpper(r.Status)
		if status == "" {
			status = "UNKNOWN"
		}
		if status == "UNKNOWN" {
			unknown = true
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n",
			r.Domain, r.Port, r.Type, status, lastChecked(r.LastChecked))
	}
	_ = tw.Flush()
	if unknown {
		_, _ = fmt.Fprintln(w, "\nUNKNOWN means \"not yet checked\" — no conclusive health observation exists yet; it is not OFFLINE. A conclusive observation may come from proxied traffic or 'devtether routes --check'; not every proxied request is conclusive.")
	}
}

// lastChecked renders an RFC3339Nano timestamp as a compact relative age.
func lastChecked(ts string) string {
	if ts == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return "—"
	}
	return time.Since(t).Round(time.Second).String() + " ago"
}

func printCheckResults(w io.Writer, results []daemon.CheckResponse) {
	if len(results) == 0 {
		_, _ = fmt.Fprintln(w, "No active routes.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tBACKEND\tSTATUS\tCONN LATENCY\tDETAIL")
	_, _ = fmt.Fprintln(tw, "------\t-------\t------\t------------\t------")
	for _, r := range results {
		latency := "—"
		if r.Latency > 0 {
			latency = fmt.Sprintf("%.2f ms", r.Latency)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			r.Domain, r.Backend, strings.ToUpper(r.Status), latency, r.Reason)
	}
	_ = tw.Flush()
}

var routesAddCmd = &cobra.Command{
	Use:   "add <domain> <port>",
	Short: "Add a live route to the running daemon",
	Long: `Adds an ephemeral live route mapping <domain> to 127.0.0.1:<port> on
the running daemon. Requires a running daemon; there is no config-file
fallback because live routes are never written to devtether.yaml.

Live routes are discarded when the daemon stops — re-add them after a
restart (or define them under "routes:" in devtether.yaml for static
routes). Health starts UNKNOWN until a conclusive observation is recorded
from proxied traffic or 'routes --check'; adding a route does not verify
the backend.`,
	Args: cobra.ExactArgs(2),
	RunE: runRoutesAdd,
}

// runRoutesAdd creates a live route via the daemon IPC API. Mutations are
// daemon-only by design: a config-file fallback would silently persist
// nothing, so a missing daemon is a hard error.
func runRoutesAdd(cmd *cobra.Command, args []string) error {
	domain, portStr := args[0], args[1]
	// Validate the literal argument before normalization or IPC, mirroring
	// routes remove: the CLI contract rejects non-canonical input (port
	// suffixes, trailing dots, wildcards) instead of silently repairing it.
	if err := config.ValidateRouteDomain(domain); err != nil {
		return err
	}
	domain = netutil.NormalizeHost(domain)
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be an integer", portStr)
	}

	client := daemon.NewClient()
	route, err := client.AddRoute(cmd.Context(), domain, port)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
			daemon.CleanStaleSocket()
			return ErrDaemonNotRunning
		}
		return withRootDaemonHint(routeAddError(domain, err))
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "Added live route %s → 127.0.0.1:%d to the running daemon\n",
		route.Domain, route.Port)
	_, _ = fmt.Fprintln(out, "Live routes are ephemeral: discarded on daemon stop, never written to devtether.yaml.")
	_, _ = fmt.Fprintln(out, "Health starts UNKNOWN until a conclusive observation is recorded from proxied traffic or 'devtether routes --check'.")
	return nil
}

var routesRemoveCmd = &cobra.Command{
	Use:   "remove <domain>",
	Short: "Remove a route from the running daemon",
	Long: `Removes a route from the running daemon only — devtether.yaml is never
modified. Static routes return on the next daemon start; live routes stay
removed until re-added. Requires a running daemon.

Removal always asks for confirmation unless -y/--yes is given. The daemon
verifies the route incarnation captured at listing time, so a route that
changed after listing is never removed silently.`,
	Args: cobra.ExactArgs(1),
	RunE: runRoutesRemove,
}

// runRoutesRemove conditionally removes a route from the running daemon.
// The incarnation comes from the same ListRoutes row as the type warning,
// and the DELETE is conditional on it — a stale confirmation cannot remove
// a replacement route (the daemon answers 409 and we stop).
func runRoutesRemove(cmd *cobra.Command, args []string) error {
	// Validate the literal argument before normalization or IPC: the CLI
	// contract rejects non-canonical input (port suffixes, trailing dots)
	// instead of silently repairing it.
	if err := config.ValidateRouteDomain(args[0]); err != nil {
		return err
	}
	domain := netutil.NormalizeHost(args[0])
	yes, _ := cmd.Flags().GetBool("yes")

	client := daemon.NewClient()
	routes, err := client.ListRoutes(cmd.Context())
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
			daemon.CleanStaleSocket()
			return ErrDaemonNotRunning
		}
		return withRootDaemonHint(err)
	}

	var target *daemon.RouteResponse
	for i := range routes {
		if routes[i].Domain == domain {
			target = &routes[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("route %q not found on the running daemon", domain)
	}

	out := cmd.OutOrStdout()
	switch target.Type {
	case "static":
		_, _ = fmt.Fprintf(out, "Warning: %s is a static route from devtether.yaml — removal affects only the running daemon; the route returns on restart and the file is not modified.\n", domain)
	case "live":
		_, _ = fmt.Fprintf(out, "Warning: %s is a live route — it is ephemeral and will not return after a daemon restart.\n", domain)
	}

	if !yes {
		stdin := cmd.InOrStdin()
		if !isTTY(stdin) {
			// Fail safe: never mutate on behalf of a non-interactive caller
			// who did not explicitly pass -y.
			return fmt.Errorf("refusing to remove route %q without confirmation: stdin is not a terminal (pass -y/--yes to skip)", domain)
		}
		if !confirmRouteRemoval(out, domain, target, stdin) {
			return fmt.Errorf("aborted: route %q not removed", domain)
		}
	}

	if err := client.RemoveRoute(cmd.Context(), domain, target.Incarnation); err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
			daemon.CleanStaleSocket()
			return ErrDaemonNotRunning
		}
		return withRootDaemonHint(routeRemoveError(domain, err))
	}

	if target.Type == "static" {
		_, _ = fmt.Fprintf(out, "Removed route %s — it returns on daemon restart (devtether.yaml unchanged).\n", domain)
	} else {
		_, _ = fmt.Fprintf(out, "Removed route %s.\n", domain)
	}
	return nil
}

// confirmRouteRemoval prompts for yes/no confirmation on an interactive
// stdin. It returns false — with no mutation — when the prompt cannot be
// read (EOF) or the answer is not yes. Callers gate this behind -y/--yes
// and a prior TTY check so scripted use stays possible.
func confirmRouteRemoval(out io.Writer, domain string, target *daemon.RouteResponse, stdin io.Reader) bool {
	_, _ = fmt.Fprintf(out, "Remove route %s (%s, port %d)? [y/N] ", domain, target.Type, target.Port)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "Confirmation aborted: %v\n", err)
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// routeAddError maps an AddRoute failure to a user-facing error. A 409
// means the domain is already registered on the running daemon; say so
// plainly instead of surfacing the raw daemon body.
func routeAddError(domain string, err error) error {
	var apiErr *daemon.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		return fmt.Errorf("route %q is already registered on the running daemon", domain)
	}
	return err
}

// routeRemoveError maps a RemoveRoute failure to a user-facing error. A 409
// means the route changed after it was listed: the user must re-confirm,
// never silently retry with a fresh incarnation.
func routeRemoveError(domain string, err error) error {
	var apiErr *daemon.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		return fmt.Errorf("route %q changed after it was listed — re-run 'devtether routes remove %s' to review and re-confirm", domain, domain)
	}
	return err
}
