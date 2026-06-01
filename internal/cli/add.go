package cli

import (
	"fmt"
	"log"

	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add [domain] [command]",
	Short: "Dynamically add and start a new service to a running DevTether daemon",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		domain := args[0]
		command := args[1]
		fmt.Printf("Adding %s -> %s\n", domain, command)

		client := daemon.NewClient()
		if err := client.AddService(domain, command); err != nil {
			log.Fatalf("Error: %v\n", err)
		}

		fmt.Println("Success! Service is now routed and running.")
	},
}
