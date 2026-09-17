package cli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

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
	if !strings.Contains(got, "DNS unavailable") {
		t.Fatalf("expected notice to mention DNS unavailable, got: %s", got)
	}
	if !strings.Contains(got, "managed") {
		t.Fatalf("expected notice to mention managed domains, got: %s", got)
	}
	if !strings.Contains(got, "Post-Install") {
		t.Fatalf("expected notice to point at README Post-Install, got: %s", got)
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

	// In non-interactive mode with no flags, defaults should be false.
	if answers.daemon {
		t.Error("expected daemon to be false in non-interactive mode")
	}
	if answers.setcap {
		t.Error("expected setcap to be false in non-interactive mode")
	}
}

func TestGatherInitAnswersWithFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("daemon", false, "")
	_ = cmd.Flags().Set("daemon", "true")

	var w bytes.Buffer
	stdin := strings.NewReader("")

	answers, err := gatherInitAnswers(cmd, stdin, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !answers.daemon {
		t.Error("expected daemon to be true when --daemon flag is set")
	}
}

func TestIsTTY(t *testing.T) {
	if isTTY(strings.NewReader("")) {
		t.Error("isTTY(non-file reader) = true, want false")
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
