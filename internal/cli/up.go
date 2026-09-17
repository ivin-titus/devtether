package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/ivin-titus/devtether/internal/dns"
	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/ivin-titus/devtether/internal/proxy"
	"github.com/ivin-titus/devtether/internal/router"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(upCmd)
	upCmd.Flags().BoolP("detach", "d", false, "run the daemon in the background")

	if runtime.GOOS == "linux" {
		upCmd.Long += "\n\nNote: Binding to port 80 requires root privileges. It is highly recommended to\nuse 'sudo setcap cap_net_bind_service=+ep devtether' instead of running as root."
	} else {
		upCmd.Long += "\n\nNote: Binding to port 80 requires root privileges. Run 'sudo devtether up -d' to bind."
	}
}

var upCmd = &cobra.Command{
	Use:     "up",
	Aliases: []string{"start"},
	Short:   "Start the DevTether routing daemon",
	Long: `Loads devtether.yaml, registers static routes, and starts the DNS resolver,
reverse proxy, and IPC daemon. Press Ctrl+C for graceful shutdown.

Use -d to run in the background. Logs are written to .logs/devtether.log
(relative to devtether.yaml) or the path set in settings.log_path.
Background mode can also be enabled via settings.daemon in devtether.yaml.`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	log := logger.New("devtether")
	log.Debug("starting...")

	// 1. Load and validate configuration.
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return configMissingHint(configPath)
		}
		return fmt.Errorf("config error: %w", err)
	}

	// 1.5. Apply config-driven verbose if CLI flag wasn't explicitly set.
	if !cmd.Flags().Changed("verbose") && cfg.Settings.Verbose {
		verbose = true
	}

	// 1.6. Daemonize if requested via -d flag or settings.daemon config.
	detach, _ := cmd.Flags().GetBool("detach")
	if !cmd.Flags().Changed("detach") && os.Getenv("DEVTETHER_FOREGROUND") == "1" {
		detach = false
	} else if !cmd.Flags().Changed("detach") && cfg.Settings.Daemon {
		detach = true
	}
	if detach && os.Getenv("DEVTETHER_FORKED") == "" {
		if checkErr := daemon.CheckRunning(context.Background()); checkErr != nil {
			return checkErr
		}
		return daemonize(cfg)
	}

	// 2. Initialize the routing engine.
	engine := router.NewEngine()

	// 3. Populate static routes (Engine 1).
	routeLog := logger.New("route")
	domains := make([]string, 0, len(cfg.Routes))
	for domain := range cfg.Routes {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	for _, domain := range domains {
		port := cfg.Routes[domain]
		if addErr := engine.AddRoute(domain, "static", port, router.RouteStatic); addErr != nil {
			return fmt.Errorf("failed to register route %s: %w", domain, addErr)
		}
		routeLog.Debug(fmt.Sprintf("%s → 127.0.0.1:%d", domain, port))
	}

	if len(cfg.Routes) == 0 {
		log.Debug("no routes defined in devtether.yaml")
	}

	// 4. Set up context with signal cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4.5. Pre-flight check: ensure another daemon instance is not already running.
	if checkErr := daemon.CheckRunning(ctx); checkErr != nil {
		return withRootDaemonHint(checkErr)
	}

	// 5. Create servers.
	dnsServer := dns.NewServer(cfg.DNS, engine)
	proxyServer := proxy.NewServer(cfg.Proxy, engine)
	ipcDaemon := daemon.NewServer(engine, cancel, configPath)

	// --- DYNAMIC UI: STARTING ---
	dim, reset := "\033[90m", "\033[0m"
	if !term.IsTerminal(int(os.Stdout.Fd())) || os.Getenv("NO_COLOR") != "" {
		dim, reset = "", ""
	}

	v := buildVersion
	if v == "" {
		v = "dev"
	}
	fmt.Printf("\n  DevTether %s%s%s\n\n", dim, v, reset)
	fmt.Printf("  %sStarting...%s\r", dim, reset)

	// 6. Synchronous Binds (DNS, Proxy, and IPC).
	// We bind before starting goroutines to flush any fallback logs
	// *before* the UI prints its dynamic summary.
	dnsAddr := ""
	dnsConn, err := dnsServer.Listen(ctx)
	if err != nil {
		dnsLog := logger.New("dns")
		dnsLog.Debug("failed to bind", "error", err)
		dnsLog.Debug("the proxy will still work — configure DNS manually or use /etc/hosts")
	} else {
		dnsAddr = dnsConn.LocalAddr().String()
	}

	proxyListener, proxyAddr, err := proxyServer.Listen(ctx)
	if err != nil {
		if dnsConn != nil {
			_ = dnsConn.Close()
		}
		return fmt.Errorf("fatal proxy bind error: %w", err)
	}
	ipcListener, err := ipcDaemon.Listen(ctx)
	if err != nil {
		if dnsConn != nil {
			_ = dnsConn.Close()
		}
		_ = proxyListener.Close()
		return fmt.Errorf("fatal IPC bind error: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)
	workerDone := make(chan struct{}, 4)
	workersComplete := make(chan struct{})
	workerCount := 0
	runWorker := func(fn func() error) {
		workerCount++
		g.Go(func() error {
			defer func() { workerDone <- struct{}{} }()
			return fn()
		})
	}

	// DNS engine (Serving on bound packet connection) — non-fatal.
	if dnsConn != nil {
		runWorker(func() error {
			return dnsServer.Serve(gCtx, dnsConn)
		})
	}

	// IPC daemon.
	runWorker(func() error {
		return ipcDaemon.Serve(gCtx, ipcListener)
	})

	// Reverse proxy (Serving on bound listener).
	runWorker(func() error {
		return proxyServer.Serve(gCtx, proxyListener, proxyAddr)
	})

	if healthCheck := printStartupSummary(gCtx, cfg, startupInfo{
		proxyAddr: proxyAddr,
		bindErr:   proxyServer.BindError(),
		dnsAddr:   dnsAddr,
	}); healthCheck != nil {
		runWorker(healthCheck)
	}
	g.Go(func() error {
		for range workerCount {
			<-workerDone
		}
		close(workersComplete)
		return nil
	})

	// Signal handler.
	g.Go(func() error {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		select {
		case sig := <-sigCh:
			log.Info(fmt.Sprintf("received %v, shutting down...", sig))
			signal.Stop(sigCh)
			cancel()
			timer := time.NewTimer(15 * time.Second)
			defer timer.Stop()
			select {
			case <-workersComplete:
				return nil
			case <-timer.C:
				fmt.Fprintln(os.Stderr, "[devtether] shutdown timed out — force exiting")
				os.Exit(1) // Authorized watchdog exception: graceful shutdown is deadlocked.
				return nil
			}
		case <-gCtx.Done():
		}
		return nil
	})

	// 6.5. Readiness Notification.
	// If we were spawned by daemonize(), signal readiness to the parent via FD 3.
	// We do this here because DNS and Proxy have successfully bound their ports,
	// and all goroutines are active.
	if os.Getenv("DEVTETHER_FORKED") == "1" {
		pipe := os.NewFile(3, "pipe")
		if pipe != nil {
			// Write the bind results back to the parent as JSON.
			payload := readinessPayload{
				Port:           proxyAddr,
				PID:            os.Getpid(),
				ConfiguredPort: cfg.Proxy.Port,
				Reason:         classifyBindFailure(proxyServer.BindError()),
			}
			if err := json.NewEncoder(pipe).Encode(payload); err != nil {
				log.Error("failed to signal daemon readiness", err)
			}
			_ = pipe.Close()
		}
	}

	// 7. Block until all goroutines finish.
	if err := g.Wait(); err != nil {
		return fmt.Errorf("fatal daemon error: %w", err)
	}

	log.Debug("stopped")
	return nil
}

// net.DialTimeout wrapper for health check testing
func isBackendOnline(ctx context.Context, port int) bool {
	target := fmt.Sprintf("127.0.0.1:%d", port)
	dialer := net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err == nil {
		_ = conn.Close()
		return true
	}
	return false
}

// configMissingHint returns the shared recovery hint for a missing config file,
// keeping `up` and `doctor` guidance consistent.
func configMissingHint(path string) error {
	return fmt.Errorf("%s not found. Create one with:\n\n  devtether init", path)
}

// noRoutesHint points the user at their config file when it exists but defines
// no routes. `devtether init` cannot help here — it exits non-zero when the
// file already exists.
func noRoutesHint(path string) string {
	return fmt.Sprintf("Edit %s to add routes, then restart the daemon", path)
}

// dnsUnavailableNotice explains the consequence of a total DNS bind failure.
func dnsUnavailableNotice() string {
	return "DNS unavailable — managed *.localhost domains will not resolve. See README: Post-Install Setup."
}

// Stable reasons the proxy may fall back from its configured port.
const (
	fallbackReasonPrivilege   = "privilege"
	fallbackReasonOccupied    = "occupied"
	fallbackReasonUnavailable = "unavailable"
)

// classifyBindFailure maps a proxy bind error to a stable reason code that can
// cross the readiness pipe without OS-specific wording.
func classifyBindFailure(err error) string {
	switch {
	case err == nil:
		return ""
	case netutil.IsPermissionError(err):
		return fallbackReasonPrivilege
	case netutil.IsAddrInUse(err):
		return fallbackReasonOccupied
	default:
		return fallbackReasonUnavailable
	}
}

// proxyFallbackNotice returns a truthful explanation when the proxy bound a
// port other than the configured one, or "" when the configured port bound
// successfully. reason is a code produced by classifyBindFailure.
func proxyFallbackNotice(configuredPort int, boundAddr, reason string) string {
	if reason == "" {
		return ""
	}
	_, portStr, err := net.SplitHostPort(boundAddr)
	if err != nil {
		return ""
	}
	boundPort, err := strconv.Atoi(portStr)
	if err != nil || boundPort == configuredPort {
		return ""
	}
	switch reason {
	case fallbackReasonPrivilege:
		if runtime.GOOS == "linux" {
			return fmt.Sprintf("devtether fell back to port %d: binding port %d requires root access or setcap", boundPort, configuredPort)
		}
		return fmt.Sprintf("devtether fell back to port %d: binding port %d requires administrative privileges", boundPort, configuredPort)
	case fallbackReasonOccupied:
		return fmt.Sprintf("devtether fell back to port %d: port %d is already in use", boundPort, configuredPort)
	default:
		return fmt.Sprintf("devtether fell back to port %d: port %d is unavailable", boundPort, configuredPort)
	}
}

// readinessPayload is the JSON handshake sent from a forked child to its parent
// over the readiness pipe (FD 3).
type readinessPayload struct {
	Port           string `json:"port"`
	PID            int    `json:"pid"`
	ConfiguredPort int    `json:"configured_port"`
	Reason         string `json:"reason"`
}

// startupInfo carries the bind results rendered in the startup summary.
type startupInfo struct {
	proxyAddr string
	// bindErr is the proxy's configured-port bind failure, if any.
	bindErr error
	// dnsAddr is the bound DNS address, or "" when DNS is unavailable.
	dnsAddr string
}

func printStartupSummary(ctx context.Context, cfg *config.Config, info startupInfo) func() error {
	// --- DYNAMIC UI: STARTED ---
	dim, green, yellow, reset := "\033[90m", "\033[92m", "\033[33m", "\033[0m"
	if !term.IsTerminal(int(os.Stdout.Fd())) || os.Getenv("NO_COLOR") != "" {
		dim, green, yellow, reset = "", "", "", ""
	}

	fmt.Printf("  %sStarted%s    \n", green, reset)

	// Report the DNS engine state: without it, managed domains do not resolve.
	if info.dnsAddr == "" {
		fmt.Printf("  %s⚠ %s%s\n", yellow, dnsUnavailableNotice(), reset)
	} else {
		fmt.Printf("  %sDNS listening on %s%s\n", dim, info.dnsAddr, reset)
	}

	if len(cfg.Routes) == 0 {
		fmt.Printf("\n  %s○ No routes defined in %s%s\n", yellow, configPath, reset)
		fmt.Printf("    %s\n", noRoutesHint(configPath))
		fmt.Println()
		return nil
	}

	// Only mention a fallback when the configured port did not bind.
	if notice := proxyFallbackNotice(cfg.Proxy.Port, info.proxyAddr, classifyBindFailure(info.bindErr)); notice != "" {
		fmt.Printf("  %s> Note: %s.%s\n", dim, notice, reset)
	}

	// Derive the port used in the route URLs.
	_, proxyPort, _ := net.SplitHostPort(info.proxyAddr)
	fmt.Println()
	fmt.Println("Tethered Routes:")

	domains := make([]string, 0, len(cfg.Routes))
	maxDomainLen := 0
	maxUrlLen := 0

	for domain := range cfg.Routes {
		domains = append(domains, domain)
		if len(domain) > maxDomainLen {
			maxDomainLen = len(domain)
		}
		urlLen := len("http://" + domain)
		if proxyPort != "80" {
			urlLen += len(":" + proxyPort)
		}
		if urlLen > maxUrlLen {
			maxUrlLen = urlLen
		}
	}
	sort.Strings(domains)

	// Use int for status: 0=unknown, 1=online, 2=offline
	status := make(map[string]int)

	// Print the static routes table once
	for _, domain := range domains {
		port := cfg.Routes[domain]
		url := fmt.Sprintf("http://%s", domain)
		if proxyPort != "80" {
			url = fmt.Sprintf("http://%s:%s", domain, proxyPort)
		}
		fmt.Printf("  %-*s  %-*s  %s→ :%d%s\n", maxDomainLen, domain, maxUrlLen, url, dim, port, reset)
	}
	fmt.Println()

	return func() error {
		// Initial check runs immediately
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		routeLog := logger.New("route")

		check := func() {
			g, _ := errgroup.WithContext(ctx)
			type result struct {
				domain string
				online bool
				state  int
			}
			results := make([]result, len(domains))

			for i, domain := range domains {
				idx, d := i, domain
				g.Go(func() error {
					port := cfg.Routes[d]
					online := isBackendOnline(ctx, port)
					newState := 2 // offline
					if online {
						newState = 1 // online
					}
					results[idx] = result{
						domain: d,
						online: online,
						state:  newState,
					}
					return nil
				})
			}
			_ = g.Wait()

			// Print output deterministically and update state map
			for _, r := range results {
				if r.state != status[r.domain] {
					status[r.domain] = r.state
					if r.online {
						routeLog.Info(fmt.Sprintf("● %s is online", r.domain))
					} else {
						routeLog.Info(fmt.Sprintf("○ %s is offline", r.domain))
					}
				}
			}
		}

		check() // First immediate check
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				check()
			}
		}
	}
}

// daemonize spawns a detached child process of DevTether and exits the parent.
// The child's stdout/stderr are redirected to a log file.
// Readiness is synchronized via an anonymous pipe (FD 3) — no polling.
func daemonize(cfg *config.Config) error {
	// Resolve log directory.
	logDir := cfg.Settings.LogPath
	if logDir == "" {
		absConfig, err := filepath.Abs(configPath)
		if err != nil {
			return fmt.Errorf("failed to resolve config path: %w", err)
		}
		logDir = filepath.Join(filepath.Dir(absConfig), ".logs")
	}

	//nolint:gosec // Log directory uses 0700 per ADR-003 security model
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	logFile := filepath.Join(logDir, "devtether.log")
	//nolint:gosec // Log files use 0644 — readable by owner and group for debugging
	// Start each detached daemon with a bounded log lifetime. A fresh run owns a
	// fresh log rather than retaining unbounded output from previous runs.
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", logFile, err)
	}

	// Create anonymous pipe for readiness signaling.
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to create readiness pipe: %w", err)
	}

	args := []string{"up", "--config", configPath}
	if verbose {
		args = append(args, "--verbose")
	}

	//nolint:gosec // G204: os.Args[0] is a safe self-reference for daemon re-exec
	child := exec.CommandContext(context.Background(), os.Args[0], args...)
	child.Stdout = f
	child.Stderr = f
	child.ExtraFiles = []*os.File{writeEnd} // Maps to FD 3 in the child
	child.Env = append(os.Environ(), "DEVTETHER_FORKED=1")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := child.Start(); err != nil {
		_ = f.Close()
		_ = readEnd.Close()
		_ = writeEnd.Close()
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Parent closes its copy of the write end and the log file.
	_ = writeEnd.Close()
	_ = f.Close()
	if err := readEnd.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		_ = readEnd.Close()
		_ = child.Process.Kill()
		return fmt.Errorf("failed to set readiness deadline: %w", err)
	}

	// Block until the child writes to the pipe or exits.
	// If the child crashes before binding, Read() returns EOF instantly.
	buf := make([]byte, 512)
	n, readErr := readEnd.Read(buf)
	_ = readEnd.Close()

	if readErr != nil {
		if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
			_ = child.Process.Kill()
			_, _ = child.Process.Wait()
			return fmt.Errorf("daemon startup timed out — child was terminated; check logs at %s", logFile)
		}
		if errors.Is(readErr, io.EOF) {
			return fmt.Errorf("daemon crashed during startup — check logs at %s", logFile)
		}
		return fmt.Errorf("readiness check failed: %w", readErr)
	}

	var payload readinessPayload
	if err := json.Unmarshal(buf[:n], &payload); err == nil {
		if notice := proxyFallbackNotice(payload.ConfiguredPort, payload.Port, payload.Reason); notice != "" {
			fmt.Printf("⚠ %s\n", notice)
		}
	}

	fmt.Printf("DevTether daemon started (PID %d)\n", child.Process.Pid)
	fmt.Printf("Logs: %s\n", logFile)
	return nil
}
