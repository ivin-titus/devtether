package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// configPath holds the path to devtether.yaml, settable via --config flag.
var configPath string

var rootCmd = &cobra.Command{
	Use:   "devtether",
	Short: "DevTether — the local development networking toolkit",
	Long: `DevTether is a modular, self-hosted developer networking toolkit that replaces
port memorization, reverse proxy configs, and ngrok subscriptions with clean
named domains — all from a single Go binary.

  devtether up        Start the routing daemon
  devtether routes    Show active routes
  devtether init      Create a starter devtether.yaml
  devtether version   Print version information

Documentation: https://github.com/ivin-titus/devtether`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
	// Cobra's default behavior prints errors to stderr and returns them.
	// With SilenceErrors=true, errors are only returned — main.go handles
	// exit codes without duplicate printing.
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "devtether.yaml",
		"path to devtether.yaml config file")
}

// SetBuildInfo configures the version information displayed by the root
// command and the version subcommand. Called from main before Execute.
func SetBuildInfo(version, commit, date string) {
	buildVersion = version
	buildCommit = commit
	buildDate = date
	rootCmd.Version = version
	rootCmd.SetVersionTemplate(
		fmt.Sprintf(versionFormat, version, commit, date),
	)
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

