package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var logLines int
var followLogs bool

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().IntVarP(&logLines, "lines", "n", 20, "number of lines to show from the end")
	logsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "follow log output (like tail -f)")
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show daemon log output",
	Long: `Displays the DevTether daemon log file.

Without flags, prints the last 20 lines. Use --lines/-n N to print the last N
lines (N must be at least 1). Use -f/--follow to stream the log in real time
(like tail -f).

Follow mode prints "DevTether daemon shut down." only when a running daemon
exits while you are watching. If no daemon is running, it reports that the log
shown belongs to the last run and exits 0.

Note: Logs are only written when the daemon is started in the background
via 'devtether up -d'. If started in the foreground, logs are written
directly to stdout/stderr.

Log file location: settings.log_path in devtether.yaml, or .logs/devtether.log
relative to the config file.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return validateLogLines(logLines)
	},
	RunE: runLogs,
}

// validateLogLines rejects non-positive --lines values at the flag boundary.
func validateLogLines(n int) error {
	if n < 1 {
		return fmt.Errorf("--lines must be at least 1 (got %d)", n)
	}
	return nil
}

func runLogs(cmd *cobra.Command, args []string) error {
	logFile, err := resolveLogFile()
	if err != nil {
		return err
	}

	f, err := os.Open(logFile) //nolint:gosec // Log file path is resolved from config
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no log file found — logs are only created when running in daemon mode (devtether up -d)")
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if followLogs {
		// Follow mode: print the backlog, then only keep watching if a daemon
		// is actually running for this user.
		if err := printLastLines(f, logLines); err != nil {
			return fmt.Errorf("failed to read log file: %w", err)
		}
		running, probeErr := daemon.DaemonRunning()
		if probeErr != nil {
			return withRootDaemonHint(probeErr)
		}
		if !running {
			fmt.Println("No running daemon detected; showing the log of the last run.")
			return nil
		}
		return tailFollow(cmd.Context(), f)
	}

	// Show last N lines.
	if err := printLastLines(f, logLines); err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}
	return nil
}

// printLastLines traverses backwards to find the Nth newline and prints the rest.
func printLastLines(f *os.File, n int) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	end := info.Size()
	if end == 0 {
		return nil
	}

	const chunkSize int64 = 4096
	buf := make([]byte, chunkSize)
	newlines := 0
	targetOffset := int64(0)

	for end > 0 {
		start := end - chunkSize
		if start < 0 {
			start = 0
		}
		readSize := end - start
		var readErr error
		if _, readErr = f.ReadAt(buf[:readSize], start); readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}

		// Count newlines backwards
		for i := int(readSize) - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				// Skip the very last newline if the file ends with one
				if end == info.Size() && i == int(readSize)-1 {
					continue
				}
				newlines++
				if newlines == n {
					targetOffset = start + int64(i) + 1
					break
				}
			}
		}

		if newlines == n {
			break
		}
		end = start
	}

	if _, err = f.Seek(targetOffset, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(os.Stdout, f)
	return err
}

// resolveLogFile determines the log file path from config or default.
func resolveLogFile() (string, error) {
	cfg, err := config.LoadConfig(configPath)
	if err == nil && cfg.Settings.LogPath != "" {
		return filepath.Join(cfg.Settings.LogPath, "devtether.log"), nil
	}

	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve config path: %w", err)
	}
	return filepath.Join(filepath.Dir(absConfig), ".logs", "devtether.log"), nil
}

// tailFollow continuously reads new data from the file and prints it.
// It exits cleanly when the daemon shuts down, detected by polling the lock file.
func tailFollow(ctx context.Context, f *os.File) error {
	// Create a cancellable context for the tail loop.
	tailCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create log watcher: %w", err)
	}
	defer func() { _ = watcher.Close() }()
	if err := watcher.Add(f.Name()); err != nil {
		return fmt.Errorf("watch log file: %w", err)
	}

	// exited is closed only when a daemon actually released its lock, so the
	// reader never claims a shutdown for a context cancellation or for a
	// daemon that belongs to another user.
	exited := make(chan struct{})

	g, gCtx := errgroup.WithContext(tailCtx)
	g.Go(func() error {
		waitErr := daemon.WaitForExit(gCtx)
		switch {
		case errors.Is(waitErr, context.Canceled):
			return nil
		case errors.Is(waitErr, daemon.ErrLockUnreadable):
			return waitErr
		case waitErr == nil, errors.Is(waitErr, daemon.ErrNoDaemon):
			close(exited)
			cancel()
			return nil
		default:
			return waitErr
		}
	})
	trigger := make(chan struct{}, 1)
	g.Go(func() error {
		for {
			select {
			case <-gCtx.Done():
				return nil
			case eventErr := <-watcher.Errors:
				if eventErr != nil {
					return fmt.Errorf("watch log file: %w", eventErr)
				}
			case <-watcher.Events:
				select {
				case trigger <- struct{}{}:
				default:
				}
			}
		}
	})

	g.Go(func() error {
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				_, _ = os.Stdout.Write(buf[:n])
			}
			if err == io.EOF {
				select {
				case <-gCtx.Done():
					select {
					case <-exited:
						fmt.Println("\nDevTether daemon shut down.")
					default:
					}
					return nil
				case <-trigger:
					// Check for file truncation (rotation) on wake
					var stat os.FileInfo
					if stat, _ = f.Stat(); stat != nil {
						currentOffset, _ := f.Seek(0, io.SeekCurrent)
						if stat.Size() < currentOffset {
							_, _ = f.Seek(0, io.SeekStart)
							fmt.Println("--- Log Truncated ---")
						}
					}
				}
				continue
			}
			if err != nil {
				return fmt.Errorf("log read error: %w", err)
			}
		}
	})
	return g.Wait()
}

