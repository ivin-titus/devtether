package main

import (
	"os"

	"github.com/ivin-titus/devtether/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		// Cobra already prints the error, so just set the exit code.
		os.Exit(1)
	}
}
