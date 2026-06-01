package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devtether",
	Short: "DevTether — the local development networking toolkit",
	Long: `DevTether is a modular, self-hosted developer networking toolkit that replaces
port memorization, reverse proxy configs, and ngrok subscriptions with clean
named domains — all from a single Go binary.

  devtether up        Start the routing daemon
  devtether routes    Show active routes

Documentation: https://github.com/ivin-titus/devtether`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}
