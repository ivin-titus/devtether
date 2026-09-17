package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPIDHandlesEmptyFile(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(PIDPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PIDPath(), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := ReadPID(); got != 0 {
		t.Errorf("ReadPID() = %d, want 0 for empty file", got)
	}
}
