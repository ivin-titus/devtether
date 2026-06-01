package cli

import (
	"fmt"
	"log"

	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:   "remove [domain]",
	Short: "Stop a service and remove its route from the running daemon",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		domain := args[0]
		fmt.Printf("Removing %s...\n", domain)

		client := daemon.NewClient()
		if err := client.RemoveService(domain); err != nil {
			log.Fatalf("Error: %v\n", err)
		}

		fmt.Println("Success! Service stopped and route removed.")
	},
}
