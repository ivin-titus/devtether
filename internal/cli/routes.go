package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
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
}

var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "Show all configured or active routes",
	Long: `If the DevTether daemon is running, fetches live routes from it.
Otherwise, reads routes directly from devtether.yaml.

Route changes require a daemon restart (devtether down && devtether up);
live reload via IPC is not available in this release.

With --check, asks the daemon to probe every active route's backend once
and prints TCP connection latency per route. Requires a running daemon.

Exit codes: 0 = all backends online; 2 = one or more backends offline or
unchecked; 1 = execution or IPC failure.`,
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
	for _, r := range routes {
		status := strings.ToUpper(r.Status)
		if status == "" {
			status = "UNKNOWN"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n",
			r.Domain, r.Port, r.Type, status, lastChecked(r.LastChecked))
	}
	_ = tw.Flush()
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
