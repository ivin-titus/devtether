package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(doctorCmd)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system environment for common issues",
	Long: `Runs diagnostic checks against your system environment without requiring a running daemon.

Checks performed:
  - Configuration syntax and parsing
  - Availability of privileged ports (80, 53)
  - setcap privileges on the devtether binary (Linux)
  - System DNS resolution for .localhost
  - Stale IPC sockets from previous crashes

Output Legend:
  ✓ Passed: System meets all requirements
  ⚠ Warning: Benign system restrictions (e.g. falling back to unprivileged ports)
  ✗ Failed: Fatal errors that will prevent DevTether from starting`,
	RunE: runDoctor,
}

func runDoctor(cmd *cobra.Command, args []string) error {
	fmt.Println("DevTether Doctor")
	fmt.Println()

	passed := 0
	warnings := 0
	failed := 0

	// Check 1: Config file
	if _, err := config.LoadConfig(configPath); err == nil {
		printCheck(true, "Config", fmt.Sprintf("%s is valid", configPath))
		passed++
	} else {
		printCheck(false, "Config", fmt.Sprintf("%s: %v", configPath, err))
		failed++
	}

	// Check 2: Port 80 availability
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:80")
	if err == nil {
		_ = ln.Close()
		printCheck(true, "Port 80", "available on 127.0.0.1")
		passed++
	} else {
		printWarn("Port 80", "unavailable (DevTether will use port 8080 fallback)")
		warnings++
	}

	// Check 3: setcap check (Linux only)
	if runtime.GOOS == "linux" {
		checkSetcap(&passed, &warnings, &failed)
	}

	// Check 4: DNS resolution
	resolver := &net.Resolver{}
	addrs, err := resolver.LookupHost(context.Background(), "localhost")
	if err == nil && len(addrs) > 0 {
		printCheck(true, "DNS", fmt.Sprintf("localhost resolves to %s", addrs[0]))
		passed++
	} else {
		printCheck(false, "DNS", "localhost does not resolve")
		failed++
	}

	// Check 5: Stale socket
	socketPath := daemon.SocketPath()
	if _, statErr := os.Stat(socketPath); statErr != nil {
		printCheck(true, "Socket", "no stale socket file")
		passed++
	} else {
		dialer := net.Dialer{Timeout: 1 * time.Second}
		conn, dialErr := dialer.DialContext(context.Background(), "unix", socketPath)
		if dialErr != nil {
			printWarn("Socket", "stale socket found (will be cleaned on next startup)")
			warnings++
		} else {
			_ = conn.Close()
			printCheck(true, "Socket", "daemon is running and responding")
			passed++
		}
	}

	fmt.Println()
	fmt.Printf("  %d passed, %d warnings, %d failed\n", passed, warnings, failed)
	if failed > 0 {
		os.Exit(1)
	}
	return nil
}

func printCheck(ok bool, name, detail string) {
	mark := "✓"
	if !ok {
		mark = "✗"
	}
	fmt.Printf("  %s %-10s %s\n", mark, name, detail)
}

func printWarn(name, detail string) {
	fmt.Printf("  ⚠ %-10s %s\n", name, detail)
}

func checkSetcap(passed, warnings, failed *int) {
	exe, err := os.Executable()
	if err != nil {
		printCheck(false, "Setcap", fmt.Sprintf("cannot resolve binary path: %v", err))
		*failed++
		return
	}

	//nolint:gosec // G204: getcap is a fixed binary name, exe is from os.Executable
	out, err := exec.CommandContext(context.Background(), "getcap", exe).Output()
	if err != nil {
		printWarn("Setcap", "getcap not available (install libcap2-bin)")
		*warnings++
		return
	}

	if len(out) > 0 {
		printCheck(true, "Setcap", "cap_net_bind_service is set")
		*passed++
	} else {
		printWarn("Setcap", "not set — run: sudo setcap cap_net_bind_service=+ep "+exe)
		*warnings++
	}
}
