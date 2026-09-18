package main

import (
	"fmt"
	"os"
)

// stdinIsPipe reports whether stdin is a pipe or redirected file rather than
// an interactive terminal.
func stdinIsPipe() (bool, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("getting info on stdin file descriptor: %w", err)
	}
	return info.Mode()&os.ModeCharDevice == 0, nil
}
