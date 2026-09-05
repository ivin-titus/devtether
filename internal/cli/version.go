package cli

import (
	"github.com/spf13/cobra"
)

// buildVersion, buildCommit, and buildDate are set by SetBuildInfo
// from main.go before Execute is called.
var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildDate    = "unknown"
)

// versionFormat is the canonical version output template.
// Used by both the "version" subcommand and the root "--version" flag.
const versionFormat = "devtether %s (commit: %s, built: %s)\n"

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit, and build date",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf(versionFormat, buildVersion, buildCommit, buildDate)
	},
}

