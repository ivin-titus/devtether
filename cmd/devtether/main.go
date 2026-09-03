package main

import (
	"os"

	"github.com/ivin-titus/devtether/internal/cli"
)

// Build-time variables injected by GoReleaser via ldflags.
// Defaults are for `go build` from source.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cli.SetBuildInfo(version, commit, date)
	if err := cli.Execute(); err != nil {
		// SilenceErrors is set on rootCmd, so the error is returned
		// without printing. We just set the exit code here.
		os.Exit(1)
	}
}
