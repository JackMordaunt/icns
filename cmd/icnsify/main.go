package main

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackmordaunt/icns"
	"github.com/spf13/afero"

	"github.com/spf13/pflag"
)

func main() {
	var (
		fs               = afero.NewOsFs()
		input  io.Reader = os.Stdin
		output io.Writer = os.Stdout

		inputPath  = pflag.StringP("input", "i", "", "Input image for conversion to icns from jpg|png or visa versa.")
		outputPath = pflag.StringP("output", "o", "", "Output path, defaults to <path/to/image>.(icns|png) depending on input.")
		algorithm  = pflag.IntP("resize", "r", 5, "Quality of resize algorithm. Values range from 0 to 5, fastest to slowest execution time. Defaults to slowest for best quality.")
	)
	pflag.Parse()
	in, out := sanitize(*inputPath, *outputPath)
	if *inputPath != "" {
		var (
			closer func() error
			err    error
		)
		input, closer, err = func(path string) (io.Reader, func() error, error) {
			f, err := fs.Open(path)
			if err != nil {
				return nil, nil, fmt.Errorf("input file: %w", err)
			}
			if err := fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, nil, fmt.Errorf("preparing parent directory: %w", err)
			}
			return f, func() error { return f.Close() }, nil
		}(in)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		defer closer()
	}
	if *outputPath != "" {
		var (
			closer func() error
			err    error
		)
		output, closer, err = func(path string) (io.Writer, func() error, error) {
			f, err := fs.Create(path)
			if err != nil {
				return nil, nil, fmt.Errorf("output file: %w", err)
			}
			if err := fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, nil, fmt.Errorf("preparing parent directory: %w", err)
			}
			return f, func() error { return f.Close() }, nil
		}(out)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		defer closer()
	}
	img, format, err := image.Decode(input)
	if err != nil {
		log.Fatalf("decoding input: %v", err)
	}
	if format == "icns" {
		if err := png.Encode(output, img); err != nil {
			log.Fatalf("encoding png: %v", err)
		}
	} else {
		enc := icns.NewEncoder(output).
			WithAlgorithm(icns.InterpolationFunction(*algorithm))
		if err := enc.Encode(img); err != nil {
			log.Fatalf("encoding icns: %v", err)
		}
	}
}

// sanitize ensures the inputs are valid.
func sanitize(
	in string,
	out string,
) (string, string) {
	if out == "" {
		out = stripExtension(in)
	} else {
		out = stripExtension(out)
	}
	if filepath.Ext(in) == ".icns" {
		out += ".png"
	}
	if filepath.Ext(in) != ".icns" {
		out += ".icns"
	}
	return in, out
}

func replaceExtension(path, ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return filepath.Base(path[:len(path)-len(filepath.Ext(path))] + ext)
}

func stripExtension(path string) string {
	return path[:len(path)-len(filepath.Ext(path))]
}
