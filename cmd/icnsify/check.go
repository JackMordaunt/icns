package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jackmordaunt/icns/v4"
	"github.com/jackmordaunt/icns/v4/ico"
)

// check reports what the platform that owns the format will make of an icon
// file. Findings are printed in order of severity, and the error reports the
// ones that change what is drawn.
func check(path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading %s: %w", name(path), err)
	}
	var lines []string
	serious := 0
	switch container(data, extension(filepath.Ext(path))) {
	case ".icns":
		problems, err := icns.Validate(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("reading %s: %w", name(path), err)
		}
		for _, p := range problems {
			lines = append(lines, p.String())
			if p.Severity != icns.Advice {
				serious++
			}
		}
	case ".ico":
		problems, err := ico.Validate(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("reading %s: %w", name(path), err)
		}
		for _, p := range problems {
			lines = append(lines, p.String())
			if p.Severity != ico.Advice {
				serious++
			}
		}
	default:
		return fmt.Errorf("%s is not an icns or ico file", name(path))
	}
	for _, line := range lines {
		fmt.Fprintf(os.Stdout, "%s: %s\n", name(path), line)
	}
	if serious > 0 {
		return fmt.Errorf("%s: %d of %d findings change what is drawn", name(path), serious, len(lines))
	}
	return nil
}

// container names the icon format the data holds, by the bytes it begins
// with and failing that by the extension it was given.
func container(data []byte, ext string) string {
	switch {
	case len(data) >= 4 && string(data[:4]) == "icns":
		return ".icns"
	case len(data) >= 4 && string(data[:4]) == "\x00\x00\x01\x00":
		return ".ico"
	}
	if containers[ext] {
		return ext
	}
	return ""
}

// name labels the file in a report, calling the one arriving on a pipe what
// it is rather than leaving it blank.
func name(path string) string {
	if path == "" {
		return "stdin"
	}
	return filepath.Base(path)
}
