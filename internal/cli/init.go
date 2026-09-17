package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolP("force", "f", false, "overwrite existing devtether.yaml")
	initCmd.Flags().Bool("daemon", false, "enable background daemon mode in generated config")
	if runtime.GOOS == "linux" {
		initCmd.Flags().Bool("setcap", false, "apply setcap for privileged port binding")
		initCmd.Long += "\n  --setcap    Apply setcap for privileged port binding"
	}
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a starter devtether.yaml in the current directory",
	Long: `Interactively creates a devtether.yaml tailored to your system.

In non-interactive environments (piped stdin, CI/CD), all prompts are skipped
and safe defaults are used unless overridden by flags.

Supported Flags:
  --daemon    Enable background daemon mode in the generated config
  --force, -f Overwrite an existing devtether.yaml`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	log := logger.New("init")
	target := configPath
	force, _ := cmd.Flags().GetBool("force")

	log.Debug("starting initialization wizard", "target", target, "force", force)

	// Idempotency guard (P3-1): keep exit 1 for script safety.
	if _, err := os.Stat(target); err == nil && !force {
		log.Debug("config file already exists, aborting")
		return fmt.Errorf("%s already exists. Skipping initialization. (rerun with --force to overwrite)", target)
	}

	// Gather configuration answers from flags or interactive prompts.
	log.Debug("gathering configuration choices")
	answers, err := gatherInitAnswers(cmd, os.Stdin, os.Stderr)
	if err != nil {
		return err
	}
	log.Debug("choices gathered", "daemon", answers.daemon, "setcap", answers.setcap)

	// Generate and write the config file.
	config := buildConfig(answers)

	log.Debug("writing config file", "target", target)
	//nolint:gosec // Config files use 0644 per engineering standards
	if err := os.WriteFile(target, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to create %s: %w", target, err)
	}

	fmt.Fprintf(os.Stderr, "Created %s\n\nNext steps:\n", target)
	fmt.Fprintln(os.Stderr, "  1. Edit the file and add your routes")
	fmt.Fprintln(os.Stderr, "  2. Run: devtether up")

	// Apply setcap if requested (Linux only).
	if answers.setcap {
		if err := applySetcap(os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "\n⚠ setcap failed: %v\n", err)
			exe, exeErr := os.Executable()
			if exeErr != nil {
				fmt.Fprintln(os.Stderr, "  You can apply it manually: sudo setcap cap_net_bind_service=+ep devtether")
			} else {
				fmt.Fprintf(os.Stderr, "  You can apply it manually: sudo setcap cap_net_bind_service=+ep %q\n", exe)
			}
		} else {
			fmt.Fprintln(os.Stderr, "\n✓ setcap applied — DevTether can bind port 80 without sudo.")
		}
	}

	return nil
}

// initAnswers holds the collected configuration choices.
type initAnswers struct {
	daemon bool
	setcap bool
}

// gatherInitAnswers resolves configuration choices from flags or interactive
// prompts. Flags always take priority. If stdin is not a TTY, prompts are
// skipped and safe defaults are used.
func gatherInitAnswers(cmd *cobra.Command, stdin io.Reader, w io.Writer) (initAnswers, error) {
	var a initAnswers
	interactive := isTTY(stdin)
	reader := bufio.NewReader(stdin)

	// Daemon mode.
	if cmd.Flags().Changed("daemon") {
		a.daemon, _ = cmd.Flags().GetBool("daemon")
	} else if interactive {
		a.daemon = promptYN(reader, w, "Run in background by default (daemon mode)?", false)
	}

	// Setcap (Linux only).
	if runtime.GOOS == "linux" {
		if cmd.Flags().Changed("setcap") {
			a.setcap, _ = cmd.Flags().GetBool("setcap")
		} else if interactive {
			a.setcap = promptYN(reader, w, "Apply setcap for port 80 binding (requires sudo)?", false)
		}
	}

	return a, nil
}

// promptYN prints a yes/no question and returns the user's answer.
// The defaultVal is used if the user presses Enter without input.
func promptYN(reader *bufio.Reader, w io.Writer, question string, defaultVal bool) bool {
	hint := "[y/N]"
	if defaultVal {
		hint = "[Y/n]"
	}
	_, _ = fmt.Fprintf(w, "%s %s ", question, hint)

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))

	if line == "" {
		return defaultVal
	}
	return line == "y" || line == "yes"
}

// buildConfig generates a devtether.yaml string based on the collected answers.
//
// This template is the single source of truth for the Engine 1 schema: every
// supported key is listed, and optional/advanced keys are commented out so the
// generated file always loads with the binary that produced it.
func buildConfig(a initAnswers) string {
	var b strings.Builder
	b.WriteString("# DevTether Configuration\n")
	b.WriteString("# Maps your services to clean named domains.\n")
	b.WriteString("# Uncomment the optional sections below as needed.\n")
	b.WriteString("# Docs: https://github.com/ivin-titus/devtether\n\n")
	b.WriteString("# Routes: map a domain to the local port your service listens on.\n")
	b.WriteString("routes:\n")
	b.WriteString("  # myapp.localhost: 3000\n")
	b.WriteString("  # api.localhost: 8000\n")

	// Engine settings.
	if a.daemon {
		b.WriteString("\nsettings:\n")
		b.WriteString("  daemon: true         # Run in background by default (or use: devtether up -d)\n")
		b.WriteString("  # verbose: false     # Enable debug-level logging\n")
		b.WriteString("  # log_path: \"./.logs\"  # Log directory when running detached\n")
	} else {
		b.WriteString("\n# settings:\n")
		b.WriteString("#   daemon: false        # Run in background by default (or use: devtether up -d)\n")
		b.WriteString("#   verbose: false       # Enable debug-level logging\n")
		b.WriteString("#   log_path: \"./.logs\"  # Log directory when running detached\n")
	}

	// Reverse proxy.
	b.WriteString("\n# proxy:\n")
	b.WriteString("#   port: 80             # Proxy port (binding 80 needs root or setcap)\n")
	b.WriteString("#   timeouts:\n")
	b.WriteString("#     idle: 120s         # Idle keep-alive timeout\n")

	// DNS resolver.
	b.WriteString("\n# dns:\n")

	b.WriteString("#   bind: \"127.0.0.1:53\"  # Loopback-only by default; port 53 needs root or setcap\n")

	return b.String()
}

// applySetcap runs sudo setcap on the current binary (Linux only).
func applySetcap(w io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve binary path: %w", err)
	}
	if rel, relErr := filepath.Rel(os.TempDir(), exe); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		_, _ = fmt.Fprintf(w, "\n⚠ %s is under %s; its capability may be lost when the temporary binary is removed.\n", exe, os.TempDir())
	}

	_, _ = fmt.Fprintf(w, "\nApplying setcap to %s...\n", exe)

	//nolint:gosec // Intentional privileged operation requested by the user.
	cmd := exec.CommandContext(context.Background(), "sudo", "setcap", "cap_net_bind_service=+ep", exe)
	cmd.Stdin = os.Stdin
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
