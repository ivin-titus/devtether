package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
		check   func(*Config) error
	}{
		{
			name: "minimal routes only",
			yaml: "routes:\n  app.localhost: 3000",
			check: func(c *Config) error {
				if len(c.Routes) != 1 {
					return fmt.Errorf("expected 1 route, got %d", len(c.Routes))
				}
				if c.Routes["app.localhost"] != 3000 {
					return fmt.Errorf("expected port 3000, got %d", c.Routes["app.localhost"])
				}
				return nil
			},
		},
		{
			name: "multiple routes",
			yaml: "routes:\n  a.localhost: 3000\n  b.localhost: 4000\n  c.a.localhost: 5000",
			check: func(c *Config) error {
				if len(c.Routes) != 3 {
					return fmt.Errorf("expected 3 routes, got %d", len(c.Routes))
				}
				return nil
			},
		},
		{
			name: "routes with proxy config",
			yaml: "routes:\n  a.localhost: 3000\nproxy:\n  port: 8080\n  timeouts:\n    read: 10s\n    write: 20s\n    idle: 30s",
			check: func(c *Config) error {
				if c.Proxy.Port != 8080 {
					return fmt.Errorf("expected proxy port 8080, got %d", c.Proxy.Port)
				}
				if c.Proxy.Timeouts.Read != "10s" {
					return fmt.Errorf("expected read timeout 10s, got %s", c.Proxy.Timeouts.Read)
				}
				return nil
			},
		},
		{
			name: "routes with dns config",
			yaml: "routes:\n  a.localhost: 3000\ndns:\n  tld: [\"localhost\", \"internal\"]\n  bind: \"127.0.0.1:5353\"",
			check: func(c *Config) error {
				if len(c.DNS.TLD) != 2 {
					return fmt.Errorf("expected 2 TLDs, got %d", len(c.DNS.TLD))
				}
				if c.DNS.Bind != "127.0.0.1:5353" {
					return fmt.Errorf("expected bind 127.0.0.1:5353, got %s", c.DNS.Bind)
				}
				return nil
			},
		},
		{
			name:    "invalid port zero",
			yaml:    "routes:\n  x.localhost: 0",
			wantErr: true,
			errMsg:  "out of valid range",
		},
		{
			name:    "invalid port too high",
			yaml:    "routes:\n  x.localhost: 99999",
			wantErr: true,
			errMsg:  "out of valid range",
		},
		{
			name:    "negative port",
			yaml:    "routes:\n  x.localhost: -1",
			wantErr: true,
			errMsg:  "out of valid range",
		},
		{
			name:    "empty domain",
			yaml:    "routes:\n  \"\": 3000",
			wantErr: true,
			errMsg:  "empty domain",
		},
		{
			name: "empty file",
			yaml: "",
			check: func(c *Config) error {
				if len(c.Routes) != 0 {
					return fmt.Errorf("expected 0 routes, got %d", len(c.Routes))
				}
				return nil
			},
		},
		{
			name:    "legacy services key",
			yaml:    "services:\n  api:\n    domain: x.localhost\n    command: npm run dev",
			wantErr: true,
			errMsg:  "legacy 'services:' format",
		},
		{
			name: "defaults applied when sections absent",
			yaml: "routes:\n  a.localhost: 3000",
			check: func(c *Config) error {
				if c.Proxy.Port != DefaultProxyPort {
					return fmt.Errorf("expected default proxy port %d, got %d", DefaultProxyPort, c.Proxy.Port)
				}
				if c.DNS.Bind != DefaultDNSBind {
					return fmt.Errorf("expected default DNS bind %s, got %s", DefaultDNSBind, c.DNS.Bind)
				}
				if len(c.DNS.TLD) != 1 || c.DNS.TLD[0] != "localhost" {
					return fmt.Errorf("expected default TLD [localhost], got %v", c.DNS.TLD)
				}
				if c.Proxy.Timeouts.Read != DefaultReadTimeout.String() {
					return fmt.Errorf("expected default read timeout, got %s", c.Proxy.Timeouts.Read)
				}
				return nil
			},
		},
		{
			name:    "orchestrate returns not implemented",
			yaml:    "orchestrate:\n  api:\n    domain: x.localhost\n    command: npm run dev",
			wantErr: true,
			errMsg:  "not yet implemented",
		},
		{
			name:    "tunnel returns not implemented",
			yaml:    "tunnel:\n  lan: true",
			wantErr: true,
			errMsg:  "not yet implemented",
		},
		{
			name:    "access returns not implemented",
			yaml:    "access:\n  enabled: true",
			wantErr: true,
			errMsg:  "not yet implemented",
		},
		{
			name:    "dns tld local rejected",
			yaml:    "routes:\n  a.localhost: 3000\ndns:\n  tld: [\"local\"]",
			wantErr: true,
			errMsg:  "reserved by mDNS",
		},
		{
			name:    "invalid proxy port",
			yaml:    "routes:\n  a.localhost: 3000\nproxy:\n  port: 99999",
			wantErr: true,
			errMsg:  "out of valid range",
		},
		{
			name:    "invalid timeout duration",
			yaml:    "routes:\n  a.localhost: 3000\nproxy:\n  timeouts:\n    read: not-a-duration",
			wantErr: true,
			errMsg:  "invalid duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "devtether.yaml")
			if err := os.WriteFile(configPath, []byte(tt.yaml), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			cfg, err := LoadConfig(configPath)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got: %v", tt.errMsg, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				if err := tt.check(cfg); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/path/that/does/not/exist.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback time.Duration
		want     time.Duration
	}{
		{"valid", "10s", 30 * time.Second, 10 * time.Second},
		{"empty uses fallback", "", 30 * time.Second, 30 * time.Second},
		{"invalid uses fallback", "garbage", 30 * time.Second, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDuration(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("ParseDuration(%q, %v) = %v, want %v", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}
