package cli

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/ivin-titus/devtether/internal/dns"
	"github.com/ivin-titus/devtether/internal/portman"
	"github.com/ivin-titus/devtether/internal/process"
	"github.com/ivin-titus/devtether/internal/proxy"
	"github.com/ivin-titus/devtether/internal/router"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func init() {
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the DevTether daemon and proxy",
	Long:  `Starts the daemon which initializes the DNS resolver, Reverse Proxy, Process Supervisor, and IPC API.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("Starting DevTether...")

		cfg, err := config.LoadConfig("devtether.yaml")
		if err != nil && !os.IsNotExist(err) {
			log.Fatalf("Error loading config: %v", err)
		}

		pm := portman.NewManager()
		engine := router.NewEngine()
		sup := process.NewSupervisor()

		if cfg != nil {
			log.Printf("Loaded %d services from devtether.yaml\n", len(cfg.Services))
			for name, svc := range cfg.Services {
				port, err := pm.GetFreePort()
				if err != nil {
					log.Fatalf("Failed to allocate port for %s: %v", name, err)
				}

				if err := engine.AddRoute(svc.Domain, name, port); err != nil {
					log.Fatalf("Failed to register route for %s: %v", name, err)
				}

				if err := sup.StartService(name, svc.Command, port); err != nil {
					log.Printf("Warning: Failed to auto-start %s: %v", name, err)
				}
			}
		} else {
			log.Println("No devtether.yaml found. Starting empty router (use 'devtether add' to attach services).")
		}

		dnsServer := dns.NewServer()
		proxyServer := proxy.NewServer(engine)
		ipcDaemon := daemon.NewServer(engine, pm, sup)

		ctx, cancel := context.WithCancel(context.Background())
		g, gCtx := errgroup.WithContext(ctx)

		// 1. Start DNS
		g.Go(func() error {
			return dnsServer.Start()
		})

		// 2. Start IPC API
		g.Go(func() error {
			return ipcDaemon.Start()
		})

		// 3. Start Proxy
		g.Go(func() error {
			return proxyServer.Start()
		})

		// 4. Listen for OS Interrupts (Ctrl+C)
		g.Go(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)
			select {
			case sig := <-c:
				log.Printf("\nReceived signal %v, shutting down gently...", sig)
				dnsServer.Stop()
				sup.StopAllServices()
				os.RemoveAll(daemon.SocketPath) // Clean up Unix socket
				cancel()
			case <-gCtx.Done():
			}
			return nil
		})

		if err := g.Wait(); err != nil && err != context.Canceled {
			log.Fatalf("Fatal error running DevTether: %v", err)
		}
	},
}
