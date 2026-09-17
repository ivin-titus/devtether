package cli

import (
	"io"
	"os"

	"golang.org/x/term"
)

// isTTY reports whether r is an interactive terminal. Non-file readers are
// deliberately treated as non-interactive so injected test input never blocks.
func isTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
