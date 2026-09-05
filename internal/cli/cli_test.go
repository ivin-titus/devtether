package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestInitCmd(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	testConfigPath := filepath.Join(tempDir, "devtether.yaml")

	// Temporarily override the configPath variable
	originalConfigPath := configPath
	configPath = testConfigPath
	defer func() { configPath = originalConfigPath }()

	// First run should succeed and create the file
	err := runInit(&cobra.Command{}, []string{})
	if err != nil {
		t.Fatalf("expected no error on first init, got: %v", err)
	}

	if _, statErr := os.Stat(testConfigPath); os.IsNotExist(statErr) {
		t.Fatalf("expected devtether.yaml to be created, but it was not")
	}

	// Second run should fail because the file already exists
	err = runInit(&cobra.Command{}, []string{})
	if err == nil {
		t.Fatal("expected an error on second init, got nil")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error to mention 'already exists', got: %v", err)
	}
}

func TestVersionCmd(t *testing.T) {
	// Temporarily override the version variables
	SetBuildInfo("v1.2.3", "abcdef", "2023-01-01")

	// Redirect stdout to capture version output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	
	// Set args to invoke version command
	rootCmd.SetArgs([]string{"version"})
	
	err := Execute()
	if err != nil {
		t.Fatalf("expected no error executing version cmd, got: %v", err)
	}
	
	output := buf.String()
	if !strings.Contains(output, "v1.2.3") {
		t.Errorf("expected output to contain 'v1.2.3', got: %s", output)
	}
}
