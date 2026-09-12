package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"syscall"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/ivin-titus/devtether/internal/dns"
	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/ivin-titus/devtether/internal/proxy"
	"github.com/ivin-titus/devtether/internal/router"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func init() {
	rootCmd.AddCommand(upCmd)
}

var upCmd = &cobra.Command{
	Use:     "up",
	Aliases: []string{"start"},
	Short:   "Start the DevTether routing daemon",
	Long: `Loads devtether.yaml, registers static routes, and starts the DNS resolver,
reverse proxy, and IPC daemon. Press Ctrl+C for graceful shutdown.`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	logger.Setup(verbose)
	log := logger.New("devtether")
	log.Debug("starting...")

	// 1. Load and validate configuration.
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found. Create one with:\n\n  devtether init", configPath)
		}
		return fmt.Errorf("config error: %w", err)
	}

	// 2. Initialize the routing engine.
	engine := router.NewEngine()

	// 3. Populate static routes (Engine 1).
	routeLog := logger.New("route")
	for domain, port := range cfg.Routes {
		if addErr := engine.AddRoute(domain, "static", port, router.RouteStatic); addErr != nil {
			return fmt.Errorf("failed to register route %s: %w", domain, addErr)
		}
		routeLog.Debug(fmt.Sprintf("%s → 127.0.0.1:%d", domain, port))
	}

	if len(cfg.Routes) == 0 {
		log.Debug("no routes defined in devtether.yaml")
	}

	// 4. Create servers.
	dnsServer := dns.NewServer(cfg.DNS, engine)
	proxyServer := proxy.NewServer(cfg.Proxy, engine)
	ipcDaemon := daemon.NewServer(engine)

	// 5. Set up context with signal cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 5.5. Pre-flight check: ensure another daemon instance is not already running.
	if checkErr := daemon.CheckRunning(ctx); checkErr != nil {
		return checkErr
	}

	// --- DYNAMIC UI: STARTING ---
	v := buildVersion
	if v == "" {
		v = "dev"
	}
	fmt.Printf("\n  DevTether \033[90m%s\033[0m\n\n", v)
	fmt.Printf("  \033[90mStarting...\033[0m\r")

	// 6. Synchronous Binds (DNS & Proxy).
	// We bind before starting goroutines to flush any fallback logs
	// *before* the UI prints its dynamic summary.
	dnsConn, err := dnsServer.Listen(ctx)
	if err != nil {
		dnsLog := logger.New("dns")
		dnsLog.Debug("failed to bind", "error", err)
		dnsLog.Debug("the proxy will still work — configure DNS manually or use /etc/hosts")
	}

	proxyListener, proxyAddr, err := proxyServer.Listen(ctx)
	if err != nil {
		if dnsConn != nil {
			_ = dnsConn.Close()
		}
		return fmt.Errorf("fatal proxy bind error: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)

	// DNS engine (Serving on bound packet connection) — non-fatal.
	if dnsConn != nil {
		g.Go(func() error {
			return dnsServer.Serve(gCtx, dnsConn)
		})
	}

	// IPC daemon.
	g.Go(func() error {
		return ipcDaemon.Start(gCtx)
	})

	// Reverse proxy (Serving on bound listener).
	g.Go(func() error {
		return proxyServer.Serve(gCtx, proxyListener, proxyAddr)
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
			// Hard deadline: force exit if graceful shutdown hangs.
			time.AfterFunc(5*time.Second, func() {
				fmt.Fprintln(os.Stderr, "[devtether] shutdown timed out — force exiting")
				os.Exit(1)
			})
		case <-gCtx.Done():
		}
		return nil
	})

	printStartupSummary(gCtx, cfg, proxyAddr)

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

func printStartupSummary(ctx context.Context, cfg *config.Config, proxyAddr string) {
	// --- DYNAMIC UI: STARTED ---
	fmt.Printf("  \033[92mStarted\033[0m    \n")

	if len(cfg.Routes) == 0 {
		fmt.Println("\n  \033[33m○ No routes defined in devtether.yaml\033[0m")
		fmt.Println("    Create one with: devtether init")
		fmt.Println()
		return
	}

	// Determine proxy port logic for URLs and Fallback warnings
	_, proxyPort, _ := net.SplitHostPort(proxyAddr)
	if proxyPort != "80" {
		if runtime.GOOS == "linux" {
			fmt.Printf("  \033[90m> Note: devtether fell back to port %s due to lack of root access or missing setcap settings.\033[0m\n", proxyPort)
		} else {
			fmt.Printf("  \033[90m> Note: devtether fell back to port %s due to lack of administrative privileges.\033[0m\n", proxyPort)
		}
	}
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
		fmt.Printf("  %-*s  %-*s  \033[90m→ :%d\033[0m\n", maxDomainLen, domain, maxUrlLen, url, port)
	}
	fmt.Println()

	// Launch realtime health checker
	go func() {
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
				return
			case <-ticker.C:
				check()
			}
		}
	}()
}
