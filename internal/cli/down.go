package cli

import (
	"context"
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
	RunE: runDown,
}

func runDown(cmd *cobra.Command, args []string) error {
	client := daemon.NewClient()
	resp, err := client.ShutdownAndWait(cmd.Context())
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
			fmt.Println("DevTether daemon is not running.")
			return nil
		}
		// Any other IPC failure (permission denied, nonce mismatch → 403, …):
		// fall back to the PID file rather than returning a bare status code.
		if stopErr := stopViaPIDFallback(cmd.Context()); stopErr != nil {
			return fmt.Errorf("daemon did not accept the IPC shutdown request (%v): %w", err, stopErr)
		}
		fmt.Println("DevTether daemon stopped.")
		return nil
	}
	// Drain the response body — blocks until the server closes the connection
	// (EOF), which signals that the entire graceful shutdown is complete.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	fmt.Println("DevTether daemon stopped.")
	return nil
}

// stopViaPIDFallback stops the daemon through its PID file when the IPC
// shutdown request cannot be delivered, then waits for the lock to release.
func stopViaPIDFallback(ctx context.Context) error {
	pid := daemon.ReadPID()
	if pid == 0 {
		return errors.New("no PID fallback is available; rerun with sudo")
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to signal daemon process %d: %w", pid, err)
	}
	if err := daemon.WaitForExit(ctx); err != nil && !errors.Is(err, daemon.ErrNoDaemon) {
		return fmt.Errorf("daemon process %d did not exit: %w", pid, err)
	}
	return nil
}
