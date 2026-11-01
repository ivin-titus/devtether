package main

import (
	"errors"
	"fmt"
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
		fmt.Fprintln(os.Stderr, "Error:", err)
		code := 1
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.Code
		}
		os.Exit(code)
	}
}
