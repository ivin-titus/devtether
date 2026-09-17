package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/spf13/cobra"
)

// DoctorFailuresError reports the number of checks that prevent startup.
type DoctorFailuresError struct {
	Count int
}

func (e *DoctorFailuresError) Error() string {
	return fmt.Sprintf("doctor found %d failed checks", e.Count)
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system environment for common issues",
	Long: `Runs diagnostic checks against your system environment without requiring a running daemon.

Checks performed:
  - Configuration syntax and parsing
  - Availability of the configured proxy port
  - Availability of the DNS port (53)
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
	log := logger.New("doctor")
	log.Debug("starting diagnostic checks", "configPath", configPath)

	fmt.Println("DevTether Doctor")
	fmt.Println()

	passed := 0
	warnings := 0
	failed := 0

	// Check 1: Config file
	cfg, configErr := config.LoadConfig(configPath)
	if configErr == nil {
		printCheck(true, "Config", fmt.Sprintf("%s is valid", configPath))
		passed++
	} else {
		if errors.Is(configErr, os.ErrNotExist) {
			printCheck(false, "Config", configMissingHint(configPath).Error())
		} else {
			printCheck(false, "Config", fmt.Sprintf("%s: %v", configPath, configErr))
		}
		failed++
	}

	// Check 2: setcap check (Linux only).
	hasSetcap := false
	if runtime.GOOS == "linux" {
		hasSetcap = checkSetcap(&passed, &warnings, &failed)
	}

	// Check 3: Effective proxy port availability.
	proxyPort := config.DefaultProxyPort
	if cfg != nil {
		proxyPort = cfg.Proxy.Port
	}
	checkProxyPort(proxyPort, hasSetcap, &passed, &warnings)

	// Check 4: Port 53 availability.
	if cfg != nil {
		var lc net.ListenConfig
		pc, dnsErr := lc.ListenPacket(context.Background(), "udp", cfg.DNS.Bind)
		if dnsErr == nil {
			_ = pc.Close()
			printCheck(true, "Port 53", fmt.Sprintf("available on %s", cfg.DNS.Bind))
			passed++
		} else {
			printWarn("Port 53", "unavailable (DevTether will use port 5353 fallback)")
			warnings++
		}

		// Check 5: resolve a configured managed route, never generic localhost.
		var domains []string
		for domain := range cfg.Routes {
			if strings.HasSuffix(domain, ".localhost") {
				domains = append(domains, domain)
			}
		}
		sort.Strings(domains)
		if len(domains) == 0 {
			printWarn("DNS", "no configured .localhost route to resolve")
			warnings++
		} else {
			addrs, lookupErr := net.DefaultResolver.LookupHost(context.Background(), domains[0])
			if lookupErr == nil && len(addrs) > 0 {
				printCheck(true, "DNS", fmt.Sprintf("%s resolves to %s", domains[0], addrs[0]))
				passed++
			} else {
				printCheck(false, "DNS", fmt.Sprintf("%s does not resolve", domains[0]))
				failed++
			}
		}
	}

	// Check 6: Stale socket.
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
		return &DoctorFailuresError{Count: failed}
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

// checkProxyPort verifies that the effective proxy port can be bound. The
// fallback narrative is derived from the configured port, never a literal 80.
func checkProxyPort(port int, hasSetcap bool, passed, warnings *int) {
	name := fmt.Sprintf("Port %d", port)
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		_ = ln.Close()
		printCheck(true, name, fmt.Sprintf("available on 127.0.0.1:%d", port))
		*passed++
		return
	}

	detail := "unavailable (DevTether will fall back to port 8080)"
	switch {
	case hasSetcap && netutil.IsPermissionError(err):
		detail = "unavailable despite cap_net_bind_service; check another process or capability policy"
	case netutil.IsAddrInUse(err):
		detail = "already in use by another process"
	}
	printWarn(name, detail)
	*warnings++
}

func printWarn(name, detail string) {
	fmt.Printf("  ⚠ %-10s %s\n", name, detail)
}

func checkSetcap(passed, warnings, failed *int) bool {
	exe, err := os.Executable()
	if err != nil {
		printCheck(false, "Setcap", fmt.Sprintf("cannot resolve binary path: %v", err))
		*failed++
		return false
	}

	//nolint:gosec // G204: getcap is a fixed binary name, exe is from os.Executable
	out, err := exec.CommandContext(context.Background(), "getcap", exe).Output()
	if err != nil {
		printWarn("Setcap", "getcap not available (install libcap2-bin)")
		*warnings++
		return false
	}

	if strings.Contains(string(out), "cap_net_bind_service") {
		printCheck(true, "Setcap", "cap_net_bind_service is set")
		*passed++
		return true
	} else {
		printWarn("Setcap", fmt.Sprintf("not set — run: sudo setcap cap_net_bind_service=+ep %q", exe))
		*warnings++
		return false
	}
}
