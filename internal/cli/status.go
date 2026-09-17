package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"text/tabwriter"

	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/spf13/cobra"
)

// ErrDaemonNotRunning identifies a status request made while no daemon is reachable.
var ErrDaemonNotRunning = errors.New("DevTether daemon is not running")

// withRootDaemonHint appends the shared cross-UID recovery hint when a
// root-owned daemon appears to be running but is unreachable from this user.
// Every command that dials the IPC socket reports the same guidance.
func withRootDaemonHint(err error) error {
	if err == nil || !daemon.RootDaemonMayBeRunning() {
		return err
	}
	return fmt.Errorf("%w; a root-owned daemon appears to be running; rerun with sudo", err)
}

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
  - Config: The devtether.yaml file the daemon loaded

Returns exit code 1 if the daemon is not currently running.`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	client := daemon.NewClient()
	status, err := client.Status(cmd.Context())
	if err != nil {
		if netutil.IsPermissionError(err) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
			return withRootDaemonHint(ErrDaemonNotRunning)
		}
		return err
	}

	renderStatus(os.Stdout, status)
	return nil
}

// renderStatus writes the daemon status table.
func renderStatus(w io.Writer, status *daemon.StatusResponse) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
	_, _ = fmt.Fprintln(tw, "-----\t-----")
	_, _ = fmt.Fprintf(tw, "PID\t%d\n", status.PID)
	_, _ = fmt.Fprintf(tw, "Uptime\t%s\n", status.Uptime)
	_, _ = fmt.Fprintf(tw, "Routes\t%d\n", status.Routes)
	_, _ = fmt.Fprintf(tw, "Heap\t%.1f MB\n", float64(status.MemAlloc)/(1024*1024))
	_, _ = fmt.Fprintf(tw, "Config\t%s\n", status.ConfigPath)
	_ = tw.Flush()
}
