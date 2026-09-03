package cli

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolP("force", "f", false, "overwrite existing devtether.yaml")
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a starter devtether.yaml in the current directory",
	Long:  `Creates a devtether.yaml with commented example routes. Refuses to overwrite an existing file unless --force is passed.`,
	Run:   runInit,
}

const defaultConfig = `# DevTether Configuration
# Maps your services to clean named domains.
# Docs: https://github.com/ivin-titus/devtether

routes:
  # myapp.localhost: 3000
  # api.localhost: 8080
`

func runInit(cmd *cobra.Command, args []string) {
	target := configPath
	force, _ := cmd.Flags().GetBool("force")

	if _, err := os.Stat(target); err == nil && !force {
		fmt.Printf("%s already exists. Use --force to overwrite.\n", target)
		return
	}

	if err := os.WriteFile(target, []byte(defaultConfig), 0644); err != nil {
		log.Fatalf("[init] failed to create %s: %v", target, err)
	}

	fmt.Printf("Created %s\n\nNext steps:\n", target)
	fmt.Println("  1. Edit the file and add your routes")
	fmt.Println("  2. Run: devtether up")
}
