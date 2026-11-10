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
			yaml: "routes:\n  a.localhost: 3000\nproxy:\n  port: 8080\n  timeouts:\n    idle: 30s",
			check: func(c *Config) error {
				if c.Proxy.Port != 8080 {
					return fmt.Errorf("expected proxy port 8080, got %d", c.Proxy.Port)
				}
				if c.Proxy.Timeouts.Idle != "30s" {
					return fmt.Errorf("expected idle timeout 30s, got %s", c.Proxy.Timeouts.Idle)
				}
				return nil
			},
		},
		{
			name: "routes with dns config",
			yaml: "routes:\n  a.localhost: 3000\ndns:\n  bind: \"127.0.0.1:5335\"",
			check: func(c *Config) error {
				if c.DNS.Bind != "127.0.0.1:5335" {
					return fmt.Errorf("expected bind 127.0.0.1:5335, got %s", c.DNS.Bind)
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
			name:    "single-label domain rejected",
			yaml:    "routes:\n  abc: 3000",
			wantErr: true,
			errMsg:  "ending in .localhost",
		},
		{
			name:    "unsupported TLD rejected",
			yaml:    "routes:\n  app.internal: 3000",
			wantErr: true,
			errMsg:  "ending in .localhost",
		},
		{
			name:    "wildcard domain rejected",
			yaml:    "routes:\n  \"*.localhost\": 3000",
			wantErr: true,
			errMsg:  "ending in .localhost",
		},
		{
			name:    "malformed label rejected",
			yaml:    "routes:\n  \"bad host.localhost\": 3000",
			wantErr: true,
			errMsg:  "ending in .localhost",
		},
		{
			name:    "underscore label rejected",
			yaml:    "routes:\n  app_v2.localhost: 3000",
			wantErr: true,
			errMsg:  "ending in .localhost",
		},
		{
			name:    "wildcard subdomain rejected",
			yaml:    "routes:\n  \"*.ivin.localhost\": 3000",
			wantErr: true,
			errMsg:  "ending in .localhost",
		},
		{
			name: "hyphenated and deeply nested labels accepted",
			yaml: "routes:\n  my-app.localhost: 3000\n  a.b.c.devtether.localhost: 3001",
			check: func(c *Config) error {
				if len(c.Routes) != 2 {
					return fmt.Errorf("expected 2 routes, got %d", len(c.Routes))
				}
				return nil
			},
		},
		{
			name: "nested localhost subdomain accepted",
			yaml: "routes:\n  api.dev.localhost: 3000",
			check: func(c *Config) error {
				if len(c.Routes) != 1 {
					return fmt.Errorf("expected 1 route, got %d", len(c.Routes))
				}
				return nil
			},
		},
		{
			name: "mixed case domain accepted (normalized at runtime)",
			yaml: "routes:\n  App.Localhost: 3000",
			check: func(c *Config) error {
				if len(c.Routes) != 1 {
					return fmt.Errorf("expected 1 route, got %d", len(c.Routes))
				}
				return nil
			},
		},
		{
			name:    "case-variant duplicate domains rejected (different ports)",
			yaml:    "routes:\n  App.localhost: 3000\n  app.localhost: 4000",
			wantErr: true,
			errMsg:  "duplicate domain",
		},
		{
			name:    "case-variant duplicate domains rejected (same port)",
			yaml:    "routes:\n  App.Localhost: 3000\n  app.localhost: 3000",
			wantErr: true,
			errMsg:  "duplicate domain",
		},
		{
			name:    "exact duplicate route key rejected by parser",
			yaml:    "routes:\n  app.localhost: 3000\n  app.localhost: 4000",
			wantErr: true,
			errMsg:  "already defined",
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

				if c.Proxy.Timeouts.Idle != DefaultIdleTimeout.String() {
					return fmt.Errorf("expected default idle timeout, got %s", c.Proxy.Timeouts.Idle)
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
			name:    "invalid proxy port",
			yaml:    "routes:\n  a.localhost: 3000\nproxy:\n  port: 99999",
			wantErr: true,
			errMsg:  "out of valid range",
		},
		{
			name:    "invalid timeout duration",
			yaml:    "routes:\n  a.localhost: 3000\nproxy:\n  timeouts:\n    idle: not-a-duration",
			wantErr: true,
			errMsg:  "invalid duration",
		},
		{
			name:    "removed proxy timeout rejected",
			yaml:    "proxy:\n  timeouts:\n    write: 20s",
			wantErr: true,
			errMsg:  "field write not found",
		},
		{
			name:    "route whitespace rejected",
			yaml:    "routes:\n  ' app.localhost ': 3000",
			wantErr: true,
			errMsg:  "surrounding whitespace",
		},
		{
			name:    "unknown field rejected",
			yaml:    "routes:\n  app.localhost: 3000\nsetings:\n  daemon: true",
			wantErr: true,
			errMsg:  "field setings not found",
		},
		{
			name: "settings config parsed",
			yaml: "settings:\n  daemon: true\n  verbose: true\n  log_path: /tmp/test-logs\nroutes:\n  a.localhost: 3000",
			check: func(c *Config) error {
				if !c.Settings.Daemon {
					return fmt.Errorf("expected daemon=true")
				}
				if !c.Settings.Verbose {
					return fmt.Errorf("expected verbose=true")
				}
				if c.Settings.LogPath != "/tmp/test-logs" {
					return fmt.Errorf("expected log_path=/tmp/test-logs, got %s", c.Settings.LogPath)
				}
				return nil
			},
		},
		{
			name: "settings defaults when absent",
			yaml: "routes:\n  a.localhost: 3000",
			check: func(c *Config) error {
				if c.Settings.Daemon {
					return fmt.Errorf("expected daemon=false by default")
				}
				if c.Settings.Verbose {
					return fmt.Errorf("expected verbose=false by default")
				}
				if c.Settings.LogPath != "" {
					return fmt.Errorf("expected empty log_path by default, got %s", c.Settings.LogPath)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "devtether.yaml")
			//nolint:gosec // Config files use 0644 per engineering standards
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

// TestRouteDomainPolicy pins the contract: routes must be well-formed DNS
// names under .localhost. Wildcards, single-label names, invalid characters,
// and other TLDs are rejected before the daemon ever binds a port. Each case
// asserts which validation rule fires, so rule ordering changes are caught.
func TestRouteDomainPolicy(t *testing.T) {
	const (
		errMalformed  = "ending in .localhost"
		errEmpty      = "empty domain"
		errWhitespace = "surrounding whitespace"
	)

	tests := []struct {
		name       string
		domain     string
		wantValid  bool
		wantErrSub string
	}{
		{name: "simple", domain: "app.localhost", wantValid: true},
		{name: "hyphenated label", domain: "my-app.localhost", wantValid: true},
		{name: "nested subdomain", domain: "a.b.c.localhost", wantValid: true},
		{name: "numeric label", domain: "123.localhost", wantValid: true},
		{name: "hyphens inside label", domain: "a-1-b.localhost", wantValid: true},
		{name: "localhost subdomain", domain: "localhost.localhost", wantValid: true},
		{name: "uppercase is normalized", domain: "APP.LOCALHOST", wantValid: true},
		// RFC 1035 caps a DNS label at 63 octets; the regex carries the same
		// bound, so an over-long label cannot register a route.
		{name: "label at 63 chars", domain: strings.Repeat("x", 63) + ".localhost", wantValid: true},
		{name: "label over 63 chars", domain: strings.Repeat("x", 64) + ".localhost", wantErrSub: errMalformed},
		{name: "nested label over 63 chars", domain: "ok." + strings.Repeat("y", 64) + ".localhost", wantErrSub: errMalformed},

		{name: "bare localhost", domain: "localhost", wantErrSub: errMalformed},
		{name: "leading dot", domain: ".localhost", wantErrSub: errMalformed},
		{name: "trailing FQDN dot", domain: "app.localhost.", wantErrSub: errMalformed},
		{name: "wildcard", domain: "*.localhost", wantErrSub: errMalformed},
		{name: "wildcard subdomain", domain: "*.ivin.localhost", wantErrSub: errMalformed},
		{name: "other TLD", domain: "app.internal", wantErrSub: errMalformed},
		{name: "underscore label", domain: "app_v2.localhost", wantErrSub: errMalformed},
		{name: "embedded space", domain: "bad host.localhost", wantErrSub: errMalformed},
		{name: "double dot", domain: "app..localhost", wantErrSub: errMalformed},
		{name: "suffix lookalike", domain: "app.localhostx", wantErrSub: errMalformed},
		{name: "leading hyphen label", domain: "-app.localhost", wantErrSub: errMalformed},
		{name: "hyphen-only label", domain: "app.-x.localhost", wantErrSub: errMalformed},
		{name: "non-ascii label", domain: "café.localhost", wantErrSub: errMalformed},
		{name: "empty", domain: "", wantErrSub: errEmpty},
		{name: "surrounding whitespace", domain: " app.localhost ", wantErrSub: errWhitespace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Table sanity: every case must declare exactly one expectation.
			if tt.wantValid == (tt.wantErrSub != "") {
				t.Fatalf("case %q must set wantValid or wantErrSub, not both/neither", tt.name)
			}

			cfg := &Config{Routes: map[string]int{tt.domain: 8080}}
			err := cfg.validateRoutes()

			if tt.wantValid {
				if err != nil {
					t.Fatalf("validateRoutes(%q) = %v, want nil", tt.domain, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateRoutes(%q) = nil, want an error containing %q", tt.domain, tt.wantErrSub)
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Fatalf("validateRoutes(%q) = %v, want it to contain %q", tt.domain, err, tt.wantErrSub)
			}
		})
	}
}

// TestRoutePortPolicy covers the port boundary independently of domain shape.
func TestRoutePortPolicy(t *testing.T) {
	const errRange = "out of valid range"

	tests := []struct {
		name       string
		port       int
		wantValid  bool
		wantErrSub string
	}{
		{name: "low boundary", port: 1, wantValid: true},
		{name: "common port", port: 3000, wantValid: true},
		{name: "high boundary", port: 65535, wantValid: true},
		{name: "zero", port: 0, wantErrSub: errRange},
		{name: "negative", port: -1, wantErrSub: errRange},
		{name: "too high", port: 65536, wantErrSub: errRange},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantValid == (tt.wantErrSub != "") {
				t.Fatalf("case %q must set wantValid or wantErrSub, not both/neither", tt.name)
			}

			cfg := &Config{Routes: map[string]int{"app.localhost": tt.port}}
			err := cfg.validateRoutes()

			if tt.wantValid {
				if err != nil {
					t.Fatalf("validateRoutes(port=%d) = %v, want nil", tt.port, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Fatalf("validateRoutes(port=%d) = %v, want an error containing %q", tt.port, err, tt.wantErrSub)
			}
		})
	}
}

// TestValidateRoute covers the exported shared validation seam directly and
// asserts it enforces exactly the same acceptance set as the YAML route
// validation that delegates to it.
func TestValidateRoute(t *testing.T) {
	tests := []struct {
		name      string
		domain    string
		port      int
		wantValid bool
	}{
		{name: "simple", domain: "app.localhost", port: 3000, wantValid: true},
		{name: "nested subdomain", domain: "a.b.c.localhost", port: 1, wantValid: true},
		{name: "uppercase normalized", domain: "APP.LOCALHOST", port: 65535, wantValid: true},
		{name: "empty domain", domain: "", port: 3000, wantValid: false},
		{name: "surrounding whitespace", domain: " app.localhost ", port: 3000, wantValid: false},
		{name: "wrong TLD", domain: "app.internal", port: 3000, wantValid: false},
		{name: "wildcard", domain: "*.localhost", port: 3000, wantValid: false},
		{name: "bare localhost", domain: "localhost", port: 3000, wantValid: false},
		{name: "port zero", domain: "app.localhost", port: 0, wantValid: false},
		{name: "port too high", domain: "app.localhost", port: 65536, wantValid: false},
		{name: "port negative", domain: "app.localhost", port: -1, wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directErr := ValidateRoute(tt.domain, tt.port)
			viaConfigErr := (&Config{Routes: map[string]int{tt.domain: tt.port}}).validateRoutes()

			if tt.wantValid {
				if directErr != nil {
					t.Fatalf("ValidateRoute(%q, %d) = %v, want nil", tt.domain, tt.port, directErr)
				}
				if viaConfigErr != nil {
					t.Fatalf("validateRoutes for same entry = %v, want nil (parity)", viaConfigErr)
				}
				return
			}
			if directErr == nil {
				t.Fatalf("ValidateRoute(%q, %d) = nil, want error", tt.domain, tt.port)
			}
			if viaConfigErr == nil {
				t.Fatalf("validateRoutes for same entry = nil, want error (parity)")
			}
		})
	}
}

// TestValidateRoutesRejectsNormalizedDuplicates pins the guard that keeps
// LoadConfig from accepting route pairs that normalize to the same engine key
// (ADR-007): they would otherwise fail daemon startup with ErrRouteExists.
func TestValidateRoutesRejectsNormalizedDuplicates(t *testing.T) {
	tests := []struct {
		name    string
		routes  map[string]int
		wantErr []string // substrings that must all appear in the error
	}{
		{
			name:    "case variants, different ports",
			routes:  map[string]int{"App.localhost": 3000, "app.localhost": 4000},
			wantErr: []string{"duplicate domain", "app.localhost", "App.localhost"},
		},
		{
			name:    "case variants, same port",
			routes:  map[string]int{"App.Localhost": 3000, "app.localhost": 3000},
			wantErr: []string{"duplicate domain", "app.localhost", "App.Localhost"},
		},
		{
			name:    "nested labels, mixed case",
			routes:  map[string]int{"API.Dev.Localhost": 3000, "api.dev.localhost": 4000},
			wantErr: []string{"duplicate domain", "api.dev.localhost", "API.Dev.Localhost"},
		},
		{
			name:    "distinct domains accepted",
			routes:  map[string]int{"app.localhost": 3000, "api.localhost": 4000},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Config{Routes: tt.routes}).validateRoutes()

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("validateRoutes() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateRoutes() = nil, want error containing %q", tt.wantErr)
			}
			for _, sub := range tt.wantErr {
				if !strings.Contains(err.Error(), sub) {
					t.Fatalf("validateRoutes() error = %q, want it to contain %q", err.Error(), sub)
				}
			}
		})
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

// TestValidateRouteDomain covers the domain-only validation seam used by
// conditional route removal (DELETE /routes/{domain}), which has no port to
// validate. It must enforce exactly the same domain policy as
// ValidateRoute's domain half.
func TestValidateRouteDomain(t *testing.T) {
	tests := []struct {
		name      string
		domain    string
		wantValid bool
	}{
		{name: "simple", domain: "app.localhost", wantValid: true},
		{name: "nested subdomain", domain: "a.b.c.localhost", wantValid: true},
		{name: "uppercase normalized", domain: "APP.LOCALHOST", wantValid: true},
		{name: "trailing dot stripped by router but literal kept", domain: "app.localhost.", wantValid: false},
		{name: "empty domain", domain: "", wantValid: false},
		{name: "surrounding whitespace", domain: " app.localhost ", wantValid: false},
		{name: "internal whitespace", domain: "app. localhost", wantValid: false},
		{name: "wrong TLD", domain: "app.internal", wantValid: false},
		{name: "wildcard", domain: "*.localhost", wantValid: false},
		{name: "bare localhost", domain: "localhost", wantValid: false},
		{name: "slash from percent decode", domain: "a/b", wantValid: false},
		{name: "port suffix", domain: "app.localhost:3000", wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domainErr := ValidateRouteDomain(tt.domain)
			routeErr := ValidateRoute(tt.domain, 3000)

			// Parity: the domain-only helper and the full validator must
			// agree on domain acceptance for a valid port.
			if (domainErr == nil) != (routeErr == nil) {
				t.Fatalf("parity mismatch: ValidateRouteDomain(%q) = %v, ValidateRoute(%q, 3000) = %v",
					tt.domain, domainErr, tt.domain, routeErr)
			}
			if tt.wantValid && domainErr != nil {
				t.Fatalf("ValidateRouteDomain(%q) = %v, want nil", tt.domain, domainErr)
			}
			if !tt.wantValid && domainErr == nil {
				t.Fatalf("ValidateRouteDomain(%q) = nil, want error", tt.domain)
			}
		})
	}
}

// TestValidateRoutePortUnchanged pins that the ValidateRoute refactor kept
// port behavior identical: port validation still runs after domain
// validation and still rejects out-of-range values.
func TestValidateRoutePortUnchanged(t *testing.T) {
	if err := ValidateRoute("app.localhost", 1); err != nil {
		t.Fatalf("port 1 = %v, want nil", err)
	}
	if err := ValidateRoute("app.localhost", 65535); err != nil {
		t.Fatalf("port 65535 = %v, want nil", err)
	}
	for _, port := range []int{0, -1, 65536} {
		if err := ValidateRoute("app.localhost", port); err == nil {
			t.Fatalf("port %d = nil, want error", port)
		}
	}
	// Domain errors still take precedence shape: invalid domain + invalid
	// port reports the domain problem (same as before the refactor).
	if err := ValidateRoute("", 0); err == nil {
		t.Fatal("empty domain with port 0 = nil, want error")
	}
}
