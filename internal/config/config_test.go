package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	validYAML := []byte(`
services:
  api:
    domain: api.internal
    command: npm run dev
  web:
    domain: web.local
    command: python main.py
`)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "devtether.yaml")
	if err := os.WriteFile(configPath, validYAML, 0644); err != nil {
		t.Fatalf("Failed to write temporary test file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(cfg.Services) != 2 {
		t.Fatalf("Expected 2 services, got: %d", len(cfg.Services))
	}

	apiSvc, exists := cfg.Services["api"]
	if !exists {
		t.Fatalf("Expected service 'api' to exist")
	}

	if apiSvc.Domain != "api.internal" {
		t.Errorf("Expected domain 'api.internal', got '%s'", apiSvc.Domain)
	}

	if apiSvc.Command != "npm run dev" {
		t.Errorf("Expected command 'npm run dev', got '%s'", apiSvc.Command)
	}
}

func TestLoadConfig_MissingField(t *testing.T) {
	invalidYAML := []byte(`
services:
  broken:
    domain: broken.internal
    # missing command
`)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "devtether.yaml")
	if err := os.WriteFile(configPath, invalidYAML, 0644); err != nil {
		t.Fatalf("Failed to write temporary test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatalf("Expected error due to missing command, got nil")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/path/that/does/not/exist.yaml")
	if err == nil {
		t.Fatalf("Expected error for missing file, got nil")
	}
}
