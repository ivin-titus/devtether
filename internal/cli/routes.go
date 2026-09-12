package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"syscall"
	"text/tabwriter"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(routesCmd)
}

var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "Show all configured or active routes",
	Long: `If the DevTether daemon is running, fetches live routes from it.
Otherwise, reads routes directly from devtether.yaml.`,
	RunE: runRoutes,
}

func runRoutes(cmd *cobra.Command, args []string) error {
	// If --config is passed, skip daemon check completely.
	if !cmd.Flags().Changed("config") {
		// Try the daemon first.
		client := daemon.NewClient()
		routes, err := client.ListRoutes()
		if err == nil {
			sort.Slice(routes, func(i, j int) bool {
				return routes[i].Domain < routes[j].Domain
			})
			printRoutes(routes)
			return nil
		}

		// Only fall back to reading config file if daemon is unreachable.
		if !errors.Is(err, syscall.ECONNREFUSED) && !errors.Is(err, os.ErrNotExist) {
			return err
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

	// Convert config routes to the same format.
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
	printRoutes(configRoutes)
	return nil
}

func printRoutes(routes []daemon.RouteResponse) {
	if len(routes) == 0 {
		fmt.Println("No active routes.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "DOMAIN\tPORT\tTYPE")
	_, _ = fmt.Fprintln(w, "------\t----\t----")
	for _, r := range routes {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\n", r.Domain, r.Port, r.Type)
	}
	_ = w.Flush()
}
