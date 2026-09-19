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
	}
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a starter devtether.yaml in the current directory",
	Long: `Interactively creates a devtether.yaml tailored to your system.

In non-interactive environments (piped stdin, CI/CD), all prompts are skipped
and safe defaults are used unless overridden by flags.

On supported systems the wizard can also point your system resolver at
DevTether so that *.localhost resolves automatically (systemd-resolved or
dnsmasq on Linux, /etc/resolver on macOS).`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	log := logger.New("init")
	target := configPath
	force, _ := cmd.Flags().GetBool("force")

	log.Debug("starting initialization wizard", "target", target, "force", force)

	// Idempotency guard: keep exit 1 for script safety.
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
	log.Debug("choices gathered", "daemon", answers.daemon, "setcap", answers.setcap, "dns", answers.dns)

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

	// Configure the host resolver if requested.
	if answers.dns {
		applied, dnsErr := applyDNSConfig(os.Stderr, answers.dnsPort)
		switch {
		case dnsErr != nil:
			fmt.Fprintf(os.Stderr, "\n⚠ DNS configuration failed: %v\n", dnsErr)
			fmt.Fprintln(os.Stderr, "  See README: Post-Install Setup to configure it manually.")
		case applied:
			fmt.Fprintln(os.Stderr, "\n✓ System DNS configured — *.localhost will resolve to DevTether.")
		}
	}

	return nil
}

// initAnswers holds the collected configuration choices.
type initAnswers struct {
	daemon  bool
	setcap  bool
	dns     bool
	dnsPort string
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

	// The DNS engine defaults to a safe unprivileged port.
	// We no longer tie the DNS port to setcap, as setcap is exclusively for Port 80.
	a.dnsPort = "5335"

	// System DNS configuration (interactive only — it is a system-wide change).
	if interactive {
		if _, ok := detectDNSProvider(runtime.GOOS, pathExists, a.dnsPort); ok {
			a.dns = promptYN(reader, w, fmt.Sprintf("Configure system DNS (127.0.0.1:%s) so *.localhost resolves automatically? (requires sudo)", a.dnsPort), false)
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
	b.WriteString("#   bind: \"127.0.0.1:5335\"  # Safe unprivileged default; fully customizable\n")

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
	return runSudo(w, os.Stdin, "setcap", "cap_net_bind_service=+ep", exe)
}

// runSudo executes a privileged command via sudo. stderr stays visible in the
// wizard output; stdout is discarded so helpers such as tee stay quiet.
func runSudo(w io.Writer, stdin io.Reader, args ...string) error {
	//nolint:gosec // Intentional privileged operation explicitly requested by the user.
	cmd := exec.CommandContext(context.Background(), "sudo", args...)
	cmd.Stdin = stdin
	cmd.Stdout = io.Discard
	cmd.Stderr = w
	return cmd.Run()
}

// dnsProvider describes a system resolver that can forward *.localhost to
// DevTether.
type dnsProvider struct {
	name    string   // resolver name shown to the user
	dir     string   // directory to create first (empty = nothing to create)
	path    string   // configuration file to write
	content string   // configuration file contents
	reload  []string // sudo command that activates the change (empty = not needed)
}

// detectDNSProvider picks the resolver configuration for the host platform.
// goos and exists are injected so the mapping stays testable on any machine.
func detectDNSProvider(goos string, exists func(string) bool, port string) (dnsProvider, bool) {
	switch goos {
	case "darwin":
		return dnsProvider{
			name:    "macOS resolver",
			dir:     "/etc/resolver",
			path:    "/etc/resolver/localhost",
			content: fmt.Sprintf("nameserver 127.0.0.1\nport %s\n", port),
		}, true
	case "linux":
		if exists("/etc/systemd/resolved.conf") {
			return dnsProvider{
				name:    "systemd-resolved",
				dir:     "/etc/systemd/resolved.conf.d",
				path:    "/etc/systemd/resolved.conf.d/devtether.conf",
				content: fmt.Sprintf("[Resolve]\nDNS=127.0.0.1:%s\nDomains=~localhost\n", port),
				reload:  []string{"systemctl", "restart", "systemd-resolved"},
			}, true
		}
		if exists("/etc/dnsmasq.d") {
			return dnsProvider{
				name:    "dnsmasq",
				path:    "/etc/dnsmasq.d/devtether.conf",
				content: fmt.Sprintf("server=/localhost/127.0.0.1#%s\n", port),
				reload:  []string{"systemctl", "restart", "dnsmasq"},
			}, true
		}
	}
	return dnsProvider{}, false
}

// pathExists reports whether a path is present on the host filesystem.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// applyDNSConfig points the host resolver at DevTether. It reports whether a
// configuration was written; unsupported systems are reported, not failed.
func applyDNSConfig(w io.Writer, port string) (bool, error) {
	provider, ok := detectDNSProvider(runtime.GOOS, pathExists, port)
	if !ok {
		_, _ = fmt.Fprintln(w, "\n⚠ No supported DNS resolver detected — configure *.localhost manually (see README: Post-Install Setup).")
		return false, nil
	}

	_, _ = fmt.Fprintf(w, "\nConfiguring %s for *.localhost...\n", provider.name)
	if provider.dir != "" {
		if err := runSudo(w, nil, "mkdir", "-p", provider.dir); err != nil {
			return false, fmt.Errorf("create %s: %w", provider.dir, err)
		}
	}
	if err := runSudo(w, strings.NewReader(provider.content), "tee", provider.path); err != nil {
		return false, fmt.Errorf("write %s: %w", provider.path, err)
	}
	if len(provider.reload) > 0 {
		if err := runSudo(w, nil, provider.reload...); err != nil {
			return false, fmt.Errorf("reload %s: %w", provider.name, err)
		}
	}
	return true, nil
}
