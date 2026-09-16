package cli

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"text/tabwriter"

	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the running daemon's status",
	Long: `Queries the DevTether IPC socket to fetch real-time metrics from the running daemon.
Displays a table with:
  - PID: The process ID of the daemon
  - Uptime: Duration the daemon has been running
  - Routes: Total number of active routes
  - Heap: Memory allocated by the daemon (in MB)

Returns exit code 1 if the daemon is not currently running.`,
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	client := daemon.NewClient()
	status, err := client.Status()
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
			fmt.Println("DevTether daemon is not running.")
			os.Exit(1)
		}
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "FIELD\tVALUE")
	_, _ = fmt.Fprintln(w, "-----\t-----")
	_, _ = fmt.Fprintf(w, "PID\t%d\n", status.PID)
	_, _ = fmt.Fprintf(w, "Uptime\t%s\n", status.Uptime)
	_, _ = fmt.Fprintf(w, "Routes\t%d\n", status.Routes)
	_, _ = fmt.Fprintf(w, "Heap\t%.1f MB\n", float64(status.MemAlloc)/(1024*1024))
	_ = w.Flush()
	return nil
}
