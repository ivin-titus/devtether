package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(downCmd)
}

var downCmd = &cobra.Command{
	Use:     "down",
	Aliases: []string{"stop"},
	Short:   "Stop the running DevTether daemon",
	Long: `Sends a shutdown request to the background daemon via the IPC socket.
This triggers a graceful shutdown where all active listeners are closed before exiting.
The command blocks until the daemon has fully stopped.

This command is idempotent — it exits with code 0 even if the daemon is not running.`,
	RunE:    runDown,
}

func runDown(cmd *cobra.Command, args []string) error {
	client := daemon.NewClient()
	resp, err := client.ShutdownAndWait()
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
			fmt.Println("DevTether daemon is not running.")
			return nil
		}
		return err
	}
	// Drain the response body — blocks until the server closes the connection
	// (EOF), which signals that the entire graceful shutdown is complete.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	fmt.Println("DevTether daemon stopped.")
	return nil
}

