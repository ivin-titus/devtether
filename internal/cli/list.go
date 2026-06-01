package cli

import (
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all currently routed services and their assigned ports",
	Run: func(cmd *cobra.Command, args []string) {
		client := daemon.NewClient()
		services, err := client.ListServices()
		if err != nil {
			log.Fatalf("Error fetching services: %v", err)
		}

		if len(services) == 0 {
			fmt.Println("No services are currently running or routed.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "DOMAIN\tSERVICE\tPORT")
		fmt.Fprintln(w, "------\t-------\t----")

		for _, svc := range services {
			fmt.Fprintf(w, "%s\t%s\t%d\n", svc.Domain, svc.ServiceName, svc.Port)
		}

		w.Flush()
	},
}
