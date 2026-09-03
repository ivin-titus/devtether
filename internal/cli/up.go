package cli

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/ivin-titus/devtether/internal/dns"
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
	Run: runUp,
}

func runUp(cmd *cobra.Command, args []string) {
	log.SetFlags(log.Ldate | log.Ltime)
	log.Println("[devtether] starting...")

	// 1. Load and validate configuration.
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		// errors.Is unwraps through fmt.Errorf %w chains.
		// os.IsNotExist does NOT unwrap — never use it with wrapped errors.
		if errors.Is(err, os.ErrNotExist) {
			log.Fatalf("[devtether] %s not found. Create one with:\n\n  devtether init\n", configPath)
		}
		log.Fatalf("[devtether] config error: %v", err)
	}

	// 2. Initialize the routing engine.
	engine := router.NewEngine()

	// 3. Populate static routes (Engine 1).
	for domain, port := range cfg.Routes {
		if err := engine.AddRoute(domain, "static", port, router.RouteStatic); err != nil {
			log.Fatalf("[route] failed to register %s: %v", domain, err)
		}
		log.Printf("[route] %s → 127.0.0.1:%d", domain, port)
	}

	if len(cfg.Routes) == 0 {
		log.Println("[devtether] no routes defined in devtether.yaml — daemon will start with an empty routing table")
	}

	// 4. Create servers.
	dnsServer := dns.NewServer(cfg.DNS, engine)
	proxyServer := proxy.NewServer(cfg.Proxy, engine)
	ipcDaemon := daemon.NewServer(engine)

	// 5. Set up context with signal cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, gCtx := errgroup.WithContext(ctx)

	// DNS engine — non-fatal. If DNS can't bind, the proxy still works
	// with /etc/hosts or systemd-resolved.
	g.Go(func() error {
		if err := dnsServer.Start(gCtx); err != nil {
			log.Printf("[dns] failed to start: %v", err)
			log.Printf("[dns] the proxy will still work — configure DNS manually or use /etc/hosts")
		}
		return nil
	})

	// IPC daemon.
	g.Go(func() error {
		return ipcDaemon.Start(gCtx)
	})

	// Reverse proxy.
	g.Go(func() error {
		return proxyServer.Start(gCtx)
	})

	// Signal handler — the hard deadline timer only starts AFTER
	// a signal is received, not when the daemon boots.
	g.Go(func() error {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		select {
		case sig := <-sigCh:
			log.Printf("[devtether] received %v, shutting down...", sig)
			cancel()
			// Hard deadline: force exit if graceful shutdown hangs.
			time.AfterFunc(5*time.Second, func() {
				log.Fatal("[devtether] shutdown timed out — force exiting")
			})
		case <-gCtx.Done():
		}
		return nil
	})

	// 6. Block until all goroutines finish.
	if err := g.Wait(); err != nil {
		log.Fatalf("[devtether] fatal: %v", err)
	}

	log.Println("[devtether] stopped")
}
