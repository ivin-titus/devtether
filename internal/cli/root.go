package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devtether",
	Short: "DevTether - The local development networking toolkit",
	Long: `DevTether is a modular developer networking toolkit that replaces port memorization,
reverse proxy configs, and ngrok subscriptions with clean named domains.

Documentation is available at https://github.com/ivin-titus/devtether`,
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
