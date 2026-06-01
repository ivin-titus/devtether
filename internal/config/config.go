package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServiceConfig defines a single routed backend application
type ServiceConfig struct {
	Domain  string `yaml:"domain"`
	Command string `yaml:"command"`
}

// Config represents the top-level structure of devtether.yaml
type Config struct {
	Services map[string]ServiceConfig `yaml:"services"`
}

// LoadConfig reads a devtether.yaml file from disk and parses it into a Config struct
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate config
	for name, service := range cfg.Services {
		if service.Domain == "" {
			return nil, fmt.Errorf("service '%s' is missing a domain", name)
		}
		if service.Command == "" {
			return nil, fmt.Errorf("service '%s' is missing a command", name)
		}
	}

	return &cfg, nil
}
