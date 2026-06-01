package main

import (
	"log"

	"github.com/ivin-titus/devtether/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		log.Fatalf("Error executing devtether: %v", err)
	}
}
