package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/daemon"
	"github.com/spf13/cobra"
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
	Long: `Displays the DevTether daemon log file. By default, shows the last 20 lines.
Use --lines/-n to control how many lines to show from the backlog.
Use -f/--follow to stream the log in real time (like tail -f).

Note: Logs are only written when the daemon is started in the background
via 'devtether up -d'. If started in the foreground, logs are written
directly to stdout/stderr.

Log file location: settings.log_path in devtether.yaml, or .logs/devtether.log
relative to the config file.`,
	RunE: runLogs,
}

func runLogs(cmd *cobra.Command, args []string) error {
	logFile, err := resolveLogFile()
	if err != nil {
		return err
	}

	f, err := os.Open(logFile) //nolint:gosec // Log file path is resolved from config
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no log file found — logs are only created when running in daemon mode (devtether up -d)")
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if followLogs || logLines == 0 {
		// Follow mode: print entire file then tail.
		if _, copyErr := io.Copy(os.Stdout, f); copyErr != nil {
			return fmt.Errorf("failed to read log file: %w", copyErr)
		}
		return tailFollow(cmd.Context(), f)
	}

	// Show last N lines.
	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	lines := splitTail(content, logLines)
	_, _ = os.Stdout.Write(lines)
	return nil
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
// It exits cleanly when the daemon shuts down, detected via a blocking
// flock on the daemon's lock file (event-driven, no polling).
func tailFollow(ctx context.Context, f *os.File) error {
	// Create a cancellable context for the tail loop.
	tailCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Launch a background goroutine that blocks until the daemon exits.
	// WaitForExit uses flock(LOCK_EX) which blocks in the kernel until
	// the daemon releases its lock (on any exit, including SIGKILL).
	go func() {
		_ = daemon.WaitForExit(tailCtx)
		cancel()
	}()

	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			_, _ = os.Stdout.Write(buf[:n])
		}
		if err == io.EOF {
			// Check if the daemon has exited before sleeping.
			select {
			case <-tailCtx.Done():
				fmt.Println("\nDevTether daemon shut down.")
				return nil
			default:
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if err != nil {
			return fmt.Errorf("log read error: %w", err)
		}

		// Check context between reads.
		select {
		case <-tailCtx.Done():
			fmt.Println("\nDevTether daemon shut down.")
			return nil
		default:
		}
	}
}

// splitTail returns the last n lines from content.
func splitTail(content []byte, n int) []byte {
	if len(content) == 0 {
		return content
	}

	// Walk backwards counting newlines.
	count := 0
	i := len(content) - 1

	// Skip trailing newline.
	if content[i] == '\n' {
		i--
	}

	for i >= 0 {
		if content[i] == '\n' {
			count++
			if count == n {
				return content[i+1:]
			}
		}
		i--
	}

	// Fewer than n lines — return everything.
	return content
}

