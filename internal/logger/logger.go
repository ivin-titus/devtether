package logger

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"
)

var outLog = log.New(os.Stdout, "", 0)

type devHandler struct {
	slog.Handler
	verbose bool
}

func (h *devHandler) Handle(ctx context.Context, r slog.Record) error {
	if !h.verbose {
		msg := r.Message
		if r.Level >= slog.LevelError {
			msg = "ERROR: " + msg
		} else if r.Level >= slog.LevelWarn {
			msg = "WARN: " + msg
		}

		var attrs []string
		r.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, fmt.Sprintf("%s=%v", a.Key, a.Value.Any()))
			return true
		})

		if len(attrs) > 0 {
			isTerm := term.IsTerminal(int(os.Stdout.Fd()))

			if isTerm {
				msg = fmt.Sprintf("%s \033[90m(%s)\033[0m", msg, strings.Join(attrs, " "))
			} else {
				msg = fmt.Sprintf("%s (%s)", msg, strings.Join(attrs, " "))
			}
		}

		outLog.Println(msg)
		return nil
	}

	return h.Handler.Handle(ctx, r)
}

// Setup configures the global slog logger.
// In standard mode (verbose=false), it filters out Debug logs and prints pristine output.
func Setup(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	baseHandler := slog.NewTextHandler(os.Stdout, opts)
	handler := &devHandler{
		Handler: baseHandler,
		verbose: verbose,
	}

	slog.SetDefault(slog.New(handler))
}

// Logger is a component-scoped logger that strictly enforces the [component]
// prefix pattern defined in the engineering standards.
type Logger struct {
	component string
	slog      *slog.Logger
}

// New creates a new Logger mapped to a specific component.
// Example: logger.New("dns") -> emits logs prefixed with "[dns]"
func New(component string) *Logger {
	return &Logger{
		component: fmt.Sprintf("[%s]", component),
		slog:      slog.Default(),
	}
}

// IsVerbose returns true if the global logger is running in debug mode.
func IsVerbose() bool {
	return slog.Default().Enabled(context.Background(), slog.LevelDebug)
}

// Info logs an informational message.
func (l *Logger) Info(msg string, args ...any) {
	l.slog.Info(fmt.Sprintf("%s %s", l.component, msg), args...)
}

// Debug logs a debug message. These are suppressed unless --verbose is enabled.
func (l *Logger) Debug(msg string, args ...any) {
	l.slog.Debug(fmt.Sprintf("%s %s", l.component, msg), args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...any) {
	l.slog.Warn(fmt.Sprintf("%s %s", l.component, msg), args...)
}

// Error logs an error message. It automatically appends the err as a structured attribute if not nil.
func (l *Logger) Error(msg string, err error, args ...any) {
	if err != nil {
		args = append(args, slog.Any("error", err))
	}
	l.slog.Error(fmt.Sprintf("%s %s", l.component, msg), args...)
}
