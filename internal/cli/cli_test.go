package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/spf13/cobra"
)

func TestValidateLogLines(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		wantErr bool
	}{
		{name: "zero rejected", n: 0, wantErr: true},
		{name: "negative rejected", n: -1, wantErr: true},
		{name: "boundary one accepted", n: 1, wantErr: false},
		{name: "large accepted", n: 20, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLogLines(tt.n)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestConfigMissingHint(t *testing.T) {
	path := "/tmp/missing-dev-tether.yaml"
	got := configMissingHint(path).Error()
	if !strings.Contains(got, path) {
		t.Fatalf("expected hint to include path %q, got: %s", path, got)
	}
	if !strings.Contains(got, "devtether init") {
		t.Fatalf("expected hint to recommend devtether init, got: %s", got)
	}
}

func TestNoRoutesHint(t *testing.T) {
	path := "/home/dev/devtether.yaml"
	got := noRoutesHint(path)
	if !strings.Contains(got, path) {
		t.Fatalf("expected hint to include path %q, got: %s", path, got)
	}
	if !strings.Contains(got, "Edit") {
		t.Fatalf("expected hint to say 'Edit ... to add routes', got: %s", got)
	}
}

func TestDNSUnavailableNotice(t *testing.T) {
	got := dnsUnavailableNotice()
	if !strings.Contains(got, "DNS server unavailable") {
		t.Fatalf("expected notice to mention DNS server unavailable, got: %s", got)
	}
	if !strings.Contains(got, "modern OSes resolve *.localhost natively") {
		t.Fatalf("expected notice to mention native resolution, got: %s", got)
	}
}

// wrapSyscallErr produces a properly wrapped net.OpError → os.SyscallError chain
// that mirrors real socket bind failures, so the netutil classifiers match.
func wrapSyscallErr(errno syscall.Errno) error {
	return &net.OpError{
		Op:  "listen",
		Net: "tcp",
		Addr: &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 80,
		},
		Err: &os.SyscallError{Syscall: "bind", Err: errno},
	}
}

func TestClassifyBindFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "EACCES", err: wrapSyscallErr(syscall.EACCES), want: fallbackReasonPrivilege},
		{name: "EPERM", err: wrapSyscallErr(syscall.EPERM), want: fallbackReasonPrivilege},
		{name: "EADDRINUSE", err: wrapSyscallErr(syscall.EADDRINUSE), want: fallbackReasonOccupied},
		{name: "other", err: fmt.Errorf("something else"), want: fallbackReasonUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyBindFailure(tt.err)
			if got != tt.want {
				t.Errorf("classifyBindFailure(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestProxyFallbackNotice(t *testing.T) {
	tests := []struct {
		name           string
		configuredPort int
		boundAddr      string
		reason         string
		wantEmpty      bool
		wantContains   string
	}{
		{name: "no reason", configuredPort: 80, boundAddr: "127.0.0.1:8080", reason: "", wantEmpty: true},
		{name: "same port", configuredPort: 8080, boundAddr: "127.0.0.1:8080", reason: fallbackReasonPrivilege, wantEmpty: true},
		{name: "privilege", configuredPort: 80, boundAddr: "127.0.0.1:8080", reason: fallbackReasonPrivilege, wantContains: "fell back to port 8080"},
		{name: "occupied", configuredPort: 80, boundAddr: "127.0.0.1:8080", reason: fallbackReasonOccupied, wantContains: "already in use"},
		{name: "unavailable", configuredPort: 80, boundAddr: "127.0.0.1:8080", reason: fallbackReasonUnavailable, wantContains: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proxyFallbackNotice(tt.configuredPort, tt.boundAddr, tt.reason)
			if tt.wantEmpty && got != "" {
				t.Errorf("expected empty, got %q", got)
			}
			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("expected notice to contain %q, got %q", tt.wantContains, got)
			}
		})
	}
}

func TestCheckProxyPort(t *testing.T) {
	// Bind an ephemeral port to simulate "in use".
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind ephemeral port: %v", err)
	}
	defer func() { _ = ln.Close() }()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())

	var passed, warnings int

	// The occupied port should produce a warning, not a pass.
	var occupiedPort int
	_, _ = fmt.Sscanf(portStr, "%d", &occupiedPort)
	checkProxyPort(occupiedPort, false, &passed, &warnings)
	if warnings != 1 {
		t.Errorf("expected 1 warning for occupied port, got %d", warnings)
	}
	if passed != 0 {
		t.Errorf("expected 0 passed for occupied port, got %d", passed)
	}
}

func TestInitCmd(t *testing.T) {
	tempDir := t.TempDir()
	testConfigPath := filepath.Join(tempDir, "devtether.yaml")

	originalConfigPath := configPath
	configPath = testConfigPath
	defer func() { configPath = originalConfigPath }()

	// First run should succeed and create the file.
	cmd := &cobra.Command{}
	cmd.Flags().BoolP("force", "f", false, "")
	cmd.Flags().Bool("daemon", false, "")
	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("expected no error on first init, got: %v", err)
	}

	if _, statErr := os.Stat(testConfigPath); os.IsNotExist(statErr) {
		t.Fatalf("expected devtether.yaml to be created, but it was not")
	}

	// Second run should fail because the file already exists.
	cmd2 := &cobra.Command{}
	cmd2.Flags().BoolP("force", "f", false, "")
	cmd2.Flags().Bool("daemon", false, "")
	err = runInit(cmd2, []string{})
	if err == nil {
		t.Fatal("expected an error on second init, got nil")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error to mention 'already exists', got: %v", err)
	}

	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("expected error to mention '--force', got: %v", err)
	}
}

func TestBuildConfig(t *testing.T) {
	tests := []struct {
		name     string
		answers  initAnswers
		contains []string
		excludes []string
	}{
		{
			name:    "defaults",
			answers: initAnswers{},
			contains: []string{
				"# DevTether Configuration",
				"routes:",
				"# settings:",
				"#   daemon: false",
			},
			excludes: []string{
				"daemon: true",
			},
		},
		{
			name:    "daemon enabled",
			answers: initAnswers{daemon: true},
			contains: []string{
				"settings:",
				"daemon: true",
			},
			excludes: []string{
				"#   daemon: false",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := buildConfig(tt.answers)
			for _, s := range tt.contains {
				if !strings.Contains(config, s) {
					t.Errorf("expected config to contain %q, got:\n%s", s, config)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(config, s) {
					t.Errorf("expected config to NOT contain %q, got:\n%s", s, config)
				}
			}
		})
	}
}

func TestGatherInitAnswersNonInteractive(t *testing.T) {
	// Simulate non-TTY: pass a bytes.Reader as stdin (not a terminal).
	// Since os.Stdin.Fd() won't be a terminal in tests, prompts are skipped.
	cmd := &cobra.Command{}
	cmd.Flags().Bool("daemon", false, "")
	cmd.Flags().Bool("setcap", false, "")

	var w bytes.Buffer
	stdin := strings.NewReader("")

	answers, err := gatherInitAnswers(cmd, stdin, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Strict typed equality: non-interactive mode with no flags must produce
	// the exact zero value (except for fallback dnsPort) — no prompt may silently flip any field on.
	if want := (initAnswers{dnsPort: "5335"}); answers != want {
		t.Errorf("gatherInitAnswers() = %+v, want %+v", answers, want)
	}
	// Non-interactive runs must not emit prompts.
	if got := w.String(); got != "" {
		t.Errorf("non-interactive run wrote %q, want no output", got)
	}
}

func TestGatherInitAnswersWithFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("daemon", false, "")
	cmd.Flags().Bool("setcap", false, "")
	_ = cmd.Flags().Set("daemon", "true")

	var w bytes.Buffer
	stdin := strings.NewReader("")

	answers, err := gatherInitAnswers(cmd, stdin, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Strict typed equality: only the explicitly set flag may be true.
	if want := (initAnswers{daemon: true, dnsPort: "5335"}); answers != want {
		t.Errorf("gatherInitAnswers() = %+v, want %+v", answers, want)
	}
}

func TestGatherInitAnswersFlagsNeverTriggerPrompts(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("daemon", false, "")
	cmd.Flags().Bool("setcap", false, "")
	_ = cmd.Flags().Set("daemon", "false")
	_ = cmd.Flags().Set("setcap", "false")

	var w bytes.Buffer
	answers, err := gatherInitAnswers(cmd, strings.NewReader(""), &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answers != (initAnswers{dnsPort: "5335"}) {
		t.Errorf("gatherInitAnswers() = %+v, want zero value with fallback port", answers)
	}
	if got := w.String(); got != "" {
		t.Errorf("explicit flags wrote %q, want no prompt output", got)
	}
}

func TestIsTTY(t *testing.T) {
	tests := []struct {
		name string
		in   io.Reader
		want bool
	}{
		{name: "nil reader", in: nil, want: false},
		{name: "string reader", in: strings.NewReader(""), want: false},
		{name: "bytes buffer", in: &bytes.Buffer{}, want: false},
		{name: "regular file", in: func() io.Reader { f, _ := os.CreateTemp(t.TempDir(), "tty"); return f }(), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTTY(tt.in); got != tt.want {
				t.Errorf("isTTY() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDetectDNSProvider asserts the exact resolver descriptor selected for each
// platform, so a partial edit (wrong path, missing reload) fails loudly.
func TestDetectDNSProvider(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		paths  []string
		wantOK bool
		want   dnsProvider
	}{
		{
			name:   "macOS resolver",
			goos:   "darwin",
			wantOK: true,
			want: dnsProvider{
				name:    "macOS resolver",
				dir:     "/etc/resolver",
				path:    "/etc/resolver/localhost",
				content: "nameserver 127.0.0.1\nport 5335\n",
			},
		},
		{
			name:   "systemd-resolved preferred over dnsmasq",
			goos:   "linux",
			paths:  []string{"/etc/systemd/resolved.conf", "/etc/dnsmasq.d"},
			wantOK: true,
			want: dnsProvider{
				name:    "systemd-resolved",
				dir:     "/etc/systemd/resolved.conf.d",
				path:    "/etc/systemd/resolved.conf.d/devtether.conf",
				content: "[Resolve]\nDNS=127.0.0.1:5335\nDomains=~localhost\n",
				reload:  []string{"systemctl", "restart", "systemd-resolved"},
			},
		},
		{
			name:   "dnsmasq fallback without systemd-resolved",
			goos:   "linux",
			paths:  []string{"/etc/dnsmasq.d"},
			wantOK: true,
			want: dnsProvider{
				name:    "dnsmasq",
				path:    "/etc/dnsmasq.d/devtether.conf",
				content: "server=/localhost/127.0.0.1#5335\n",
				reload:  []string{"systemctl", "restart", "dnsmasq"},
			},
		},
		{
			name:   "linux without a known resolver",
			goos:   "linux",
			wantOK: false,
		},
		{
			name:   "unsupported platform",
			goos:   "windows",
			wantOK: false,
		},
		{
			name:   "freebsd unsupported even with linux paths",
			goos:   "freebsd",
			paths:  []string{"/etc/systemd/resolved.conf", "/etc/dnsmasq.d"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Table sanity: an "ok" case must declare a descriptor; a "not ok"
			// case must expect the zero descriptor.
			if tt.wantOK != (tt.want.path != "") {
				t.Fatalf("case %q: wantOK=%v is inconsistent with want=%+v", tt.name, tt.wantOK, tt.want)
			}
			if !tt.wantOK && !reflect.DeepEqual(tt.want, dnsProvider{}) {
				t.Fatalf("case %q: non-ok case must expect the zero descriptor", tt.name)
			}

			present := make(map[string]bool, len(tt.paths))
			for _, p := range tt.paths {
				present[p] = true
			}

			provider, ok := detectDNSProvider(tt.goos, func(p string) bool { return present[p] }, "5335")
			if ok != tt.wantOK {
				t.Fatalf("detectDNSProvider(%s) ok = %v, want %v", tt.goos, ok, tt.wantOK)
			}
			if !reflect.DeepEqual(provider, tt.want) {
				t.Errorf("detectDNSProvider(%s)\n got: %+v\nwant: %+v", tt.goos, provider, tt.want)
			}
			// Every descriptor must be actionable: a path and a config body.
			if ok && (provider.path == "" || provider.content == "") {
				t.Error("descriptor is missing a path or config body")
			}
		})
	}
}

// TestCompletionCommandDisabled is a regression test: Cobra's
// generated `completion` command must not be reachable from the CLI at all.
func TestCompletionCommandDisabled(t *testing.T) {
	if !rootCmd.CompletionOptions.DisableDefaultCmd {
		t.Error("expected rootCmd.CompletionOptions.DisableDefaultCmd to be true")
	}
	for _, c := range rootCmd.Commands() {
		if c.Name() == "completion" {
			t.Fatal("completion command is registered; it must stay hidden from users")
		}
	}
}

// TestInitHelpPrintsFlagsOnce guards the help output fix: Cobra's native flags block
// is the only place flags may appear for `init`.
func TestInitHelpPrintsFlagsOnce(t *testing.T) {
	if strings.Contains(initCmd.Long, "Supported Flags") {
		t.Error("initCmd.Long still contains the manual 'Supported Flags' block")
	}
	for _, flag := range []string{"--daemon", "--force", "--setcap"} {
		if strings.Contains(initCmd.Long, flag) {
			t.Errorf("initCmd.Long duplicates flag %q; Cobra already renders it", flag)
		}
	}

	usage := initCmd.Flags().FlagUsages()
	wantSetcap := 0
	if runtime.GOOS == "linux" {
		wantSetcap = 1
	}
	if got := strings.Count(usage, "--setcap"); got != wantSetcap {
		t.Errorf("--setcap appears %d times in flag usage, want %d", got, wantSetcap)
	}
	if got := strings.Count(usage, "--daemon"); got != 1 {
		t.Errorf("--daemon appears %d times in flag usage, want 1", got)
	}
	if got := strings.Count(usage, "--force"); got != 1 {
		t.Errorf("--force appears %d times in flag usage, want 1", got)
	}
}

// TestRootHelpIsCobraNative asserts the root help output contains no duplicated
// command cheat-sheet and advertises the real command surface.
func TestRootHelpIsCobraNative(t *testing.T) {
	origOut := rootCmd.OutOrStdout()
	t.Cleanup(func() {
		rootCmd.SetOut(origOut)
		rootCmd.SetArgs(nil)
	})

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})

	if err := Execute(); err != nil {
		t.Fatalf("--help returned an error: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "completion") {
		t.Error("root help advertises the completion command")
	}
	if strings.Contains(out, "Start in the background (logs to .logs/)") {
		t.Error("root help still contains the manual command cheat-sheet")
	}
	if !strings.Contains(out, "Available Commands:") {
		t.Error("root help is missing Cobra's native command list")
	}
	// Every user-facing command must be discoverable from the root help.
	for _, want := range []string{"up", "down", "status", "routes", "logs", "doctor", "init", "version"} {
		if !strings.Contains(out, want) {
			t.Errorf("root help does not mention command %q", want)
		}
	}
	// A command that is hidden or has no Short renders blank in help.
	for _, c := range rootCmd.Commands() {
		if c.Hidden && c.Name() != "help" {
			t.Errorf("command %q is hidden and cannot be discovered", c.Name())
		}
		if c.Short == "" {
			t.Errorf("command %q has no Short description for the command list", c.Name())
		}
	}
}

// TestBuildConfigRoundTripsThroughLoader proves the generated init template is
// always loadable by the schema that produced it (the schema-drift guard).
func TestBuildConfigRoundTripsThroughLoader(t *testing.T) {
	tests := []struct {
		name    string
		answers initAnswers
		want    config.SettingsConfig
	}{
		{
			name:    "foreground defaults",
			answers: initAnswers{dnsPort: "53"},
			want:    config.SettingsConfig{},
		},
		{
			name:    "daemon enabled",
			answers: initAnswers{daemon: true, dnsPort: "53"},
			want:    config.SettingsConfig{Daemon: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "devtether.yaml")
			//nolint:gosec // Test fixture mirrors the 0644 config convention
			if err := os.WriteFile(path, []byte(buildConfig(tt.answers)), 0644); err != nil {
				t.Fatalf("failed to write generated config: %v", err)
			}

			cfg, err := config.LoadConfig(path)
			if err != nil {
				t.Fatalf("generated template failed validation: %v", err)
			}
			if cfg.Settings != tt.want {
				t.Errorf("settings = %+v, want %+v", cfg.Settings, tt.want)
			}
			if len(cfg.Routes) != 0 {
				t.Errorf("template registered %d routes, want 0 (all examples are comments)", len(cfg.Routes))
			}
			if cfg.Proxy.Port != config.DefaultProxyPort {
				t.Errorf("proxy port = %d, want default %d", cfg.Proxy.Port, config.DefaultProxyPort)
			}
			if cfg.DNS.Bind != config.DefaultDNSBind {
				t.Errorf("dns bind = %q, want default %q", cfg.DNS.Bind, config.DefaultDNSBind)
			}
		})
	}
}

func TestVersionCmd(t *testing.T) {
	SetBuildInfo("v1.2.3", "abcdef", "2023-01-01")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"version"})

	err := Execute()

	output := buf.String()

	if err != nil {
		t.Fatalf("expected no error executing version cmd, got: %v", err)
	}

	if !strings.Contains(output, "v1.2.3") {
		t.Errorf("expected output to contain 'v1.2.3', got: %s", output)
	}
}

// TestLastStartupError is a regression test: when a detached daemon
// crashes during startup, the parent must surface the child's fatal error
// (e.g. the ADR-003 symlink refusal) instead of only pointing at the log.
func TestLastStartupError(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "fatal error line is surfaced without prefix",
			content: "  Starting...\nError: fatal IPC bind error: daemon: runtime directory /tmp/x is a symlink (possible attack)\n",
			want:    "fatal IPC bind error: daemon: runtime directory /tmp/x is a symlink (possible attack)",
		},
		{
			name:    "last error line wins",
			content: "Error: first warning\nError: fatal crash\n",
			want:    "fatal crash",
		},
		{
			name:    "no error line yields empty",
			content: "  Starting...\n  Started\n",
			want:    "",
		},
		{
			name:    "empty log yields empty",
			content: "",
			want:    "",
		},
		{
			name:    "bare Error prefix without message yields empty",
			content: "Error: \n",
			want:    "",
		},
		{
			// The daemon's progress banner ends with \r, so the fatal error
			// is concatenated onto the banner line in the log file.
			name:    "error after carriage-return banner is found",
			content: "  Starting...\rError: fatal IPC bind error: daemon: runtime directory /x is a symlink (possible attack)\n",
			want:    "fatal IPC bind error: daemon: runtime directory /x is a symlink (possible attack)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logFile := filepath.Join(t.TempDir(), "devtether.log")
			//nolint:gosec // Test-controlled path
			if err := os.WriteFile(logFile, []byte(tt.content), 0600); err != nil {
				t.Fatalf("failed to write test log: %v", err)
			}
			if got := lastStartupError(logFile); got != tt.want {
				t.Errorf("lastStartupError() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("missing log file yields empty", func(t *testing.T) {
		if got := lastStartupError(filepath.Join(t.TempDir(), "absent.log")); got != "" {
			t.Errorf("lastStartupError() = %q, want empty", got)
		}
	})

	t.Run("only the tail of a large log is read", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "devtether.log")
		padding := strings.Repeat("x\n", 5000) // > 4096-byte tail window
		content := padding + "Error: tail crash\n"
		//nolint:gosec // Test-controlled path
		if err := os.WriteFile(logFile, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write test log: %v", err)
		}
		if got := lastStartupError(logFile); got != "tail crash" {
			t.Errorf("lastStartupError() = %q, want %q", got, "tail crash")
		}
	})
}
