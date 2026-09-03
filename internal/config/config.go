// Package config provides the schema, parsing, validation, and defaults
// for the devtether.yaml configuration file.
//
// The config file uses a unified schema where each top-level key activates
// an independent engine. Absent keys mean that engine is not loaded.
// See ADR-005 for the design rationale.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level schema for devtether.yaml.
// Each top-level key activates an independent engine.
// Absent keys mean that engine is not loaded.
type Config struct {
	Routes      map[string]int            `yaml:"routes,omitempty"`
	Orchestrate map[string]ServiceConfig  `yaml:"orchestrate,omitempty"`
	Tunnel      *TunnelConfig             `yaml:"tunnel,omitempty"`
	Access      *AccessConfig             `yaml:"access,omitempty"`
	Proxy       ProxyConfig               `yaml:"proxy,omitempty"`
	DNS         DNSConfig                 `yaml:"dns,omitempty"`
}

// ServiceConfig defines an orchestrated service (Engine 2, Phase 2).
type ServiceConfig struct {
	Domain  string            `yaml:"domain"`
	Command string            `yaml:"command"`
	Cwd     string            `yaml:"cwd,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Restart string            `yaml:"restart,omitempty"`
}

// TunnelConfig defines tunneling options (Engine 3, Phase 3-4).
type TunnelConfig struct {
	LAN   bool   `yaml:"lan,omitempty"`
	Relay string `yaml:"relay,omitempty"`
}

// AccessConfig defines RBAC rules (Engine 4, Phase 5).
type AccessConfig struct {
	Enabled      bool     `yaml:"enabled,omitempty"`
	RequireToken []string `yaml:"require_token,omitempty"`
	Public       []string `yaml:"public,omitempty"`
}

// ProxyConfig defines reverse proxy behavior.
type ProxyConfig struct {
	Port     int           `yaml:"port,omitempty"`
	Timeouts TimeoutConfig `yaml:"timeouts,omitempty"`
}

// TimeoutConfig defines proxy connection timeouts.
type TimeoutConfig struct {
	Read  string `yaml:"read,omitempty"`
	Write string `yaml:"write,omitempty"`
	Idle  string `yaml:"idle,omitempty"`
}

// DNSConfig defines DNS engine behavior.
type DNSConfig struct {
	TLD  []string `yaml:"tld,omitempty"`
	Bind string   `yaml:"bind,omitempty"`
}

// legacyConfig is used solely to detect the old v0 schema and provide
// a helpful migration error.
type legacyConfig struct {
	Services map[string]interface{} `yaml:"services"`
}

// DefaultProxyPort is the default port for the reverse proxy.
const DefaultProxyPort = 80

// DefaultDNSBind is the default bind address for the DNS engine.
// Loopback-only by default per ADR-003 (secure by default).
const DefaultDNSBind = "127.0.0.1:53"

// DefaultReadTimeout is the default proxy read timeout.
const DefaultReadTimeout = 30 * time.Second

// DefaultWriteTimeout is the default proxy write timeout.
const DefaultWriteTimeout = 60 * time.Second

// DefaultIdleTimeout is the default proxy idle timeout.
const DefaultIdleTimeout = 120 * time.Second

// LoadConfig reads a devtether.yaml file from disk, parses it into a
// Config struct, applies defaults for missing fields, and validates
// all populated sections.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Detect legacy v0 schema before parsing the real config.
	if err := detectLegacySchema(data); err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// detectLegacySchema checks if the YAML contains the old v0 "services:" key
// and returns a helpful migration error.
func detectLegacySchema(data []byte) error {
	var legacy legacyConfig
	_ = yaml.Unmarshal(data, &legacy) // Let the real parser handle malformed YAML.
	if len(legacy.Services) > 0 {
		return fmt.Errorf(
			"devtether.yaml uses the legacy 'services:' format which is no longer supported.\n" +
				"Please migrate to the new schema:\n\n" +
				"  routes:\n" +
				"    your-app.localhost: 3000\n\n" +
				"See: https://github.com/ivin-titus/devtether#usage",
		)
	}
	return nil
}

// applyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) applyDefaults() {
	if c.Proxy.Port == 0 {
		c.Proxy.Port = DefaultProxyPort
	}
	if c.Proxy.Timeouts.Read == "" {
		c.Proxy.Timeouts.Read = DefaultReadTimeout.String()
	}
	if c.Proxy.Timeouts.Write == "" {
		c.Proxy.Timeouts.Write = DefaultWriteTimeout.String()
	}
	if c.Proxy.Timeouts.Idle == "" {
		c.Proxy.Timeouts.Idle = DefaultIdleTimeout.String()
	}
	if c.DNS.Bind == "" {
		c.DNS.Bind = DefaultDNSBind
	}
	if len(c.DNS.TLD) == 0 {
		c.DNS.TLD = []string{"localhost"}
	}
}

// validate checks all populated sections for correctness.
func (c *Config) validate() error {
	if err := c.validateRoutes(); err != nil {
		return err
	}
	if err := c.validateUnimplemented(); err != nil {
		return err
	}
	if err := c.validateProxy(); err != nil {
		return err
	}
	return c.validateDNS()
}

// validateRoutes checks that all static route entries are valid.
func (c *Config) validateRoutes() error {
	for domain, port := range c.Routes {
		if domain == "" {
			return fmt.Errorf("routes: empty domain name is not allowed")
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("routes: port %d for domain '%s' is out of valid range (1-65535)", port, domain)
		}
	}
	return nil
}

// validateUnimplemented returns clear errors for engines that are not
// yet implemented. Explicit is better than silent.
func (c *Config) validateUnimplemented() error {
	if len(c.Orchestrate) > 0 {
		return fmt.Errorf("'orchestrate:' engine is not yet implemented (coming in Phase 2). Remove this section to proceed")
	}
	if c.Tunnel != nil {
		return fmt.Errorf("'tunnel:' engine is not yet implemented (coming in Phase 3). Remove this section to proceed")
	}
	if c.Access != nil {
		return fmt.Errorf("'access:' engine is not yet implemented (coming in Phase 5). Remove this section to proceed")
	}
	return nil
}

// validateProxy checks proxy configuration values.
func (c *Config) validateProxy() error {
	if c.Proxy.Port < 1 || c.Proxy.Port > 65535 {
		return fmt.Errorf("proxy.port: %d is out of valid range (1-65535)", c.Proxy.Port)
	}
	if _, err := time.ParseDuration(c.Proxy.Timeouts.Read); err != nil {
		return fmt.Errorf("proxy.timeouts.read: invalid duration '%s': %w", c.Proxy.Timeouts.Read, err)
	}
	if _, err := time.ParseDuration(c.Proxy.Timeouts.Write); err != nil {
		return fmt.Errorf("proxy.timeouts.write: invalid duration '%s': %w", c.Proxy.Timeouts.Write, err)
	}
	if _, err := time.ParseDuration(c.Proxy.Timeouts.Idle); err != nil {
		return fmt.Errorf("proxy.timeouts.idle: invalid duration '%s': %w", c.Proxy.Timeouts.Idle, err)
	}
	return nil
}

// validateDNS checks DNS configuration values.
func (c *Config) validateDNS() error {
	for _, tld := range c.DNS.TLD {
		if tld == "" {
			return fmt.Errorf("dns.tld: empty TLD is not allowed")
		}
		if tld == "local" {
			return fmt.Errorf("dns.tld: 'local' is reserved by mDNS (RFC 6762) and will conflict with Avahi/Bonjour. Use 'localhost' or 'internal' instead")
		}
	}
	return nil
}

// ParseDuration is a helper that parses a timeout string from the config.
// Returns the parsed duration or the provided fallback if the string is empty.
func ParseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
