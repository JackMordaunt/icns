package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackmordaunt/icns/v3"
	"github.com/spf13/afero"

	"github.com/spf13/pflag"
)

var fs = afero.NewOsFs()

// errUsage signals that no work was requested; usage has been printed.
var errUsage = errors.New("usage")

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, errUsage) {
			slog.Error("icnsify failed", "err", err)
		}
		os.Exit(1)
	}
}

func run() error {
	var (
		inputPath = pflag.StringP(
			"input",
			"i",
			"",
			"Input image for conversion to icns from jpg|png or visa versa.",
		)
		outputPath = pflag.StringP(
			"output",
			"o",
			"",
			"Output path, defaults to <path/to/image>.(icns|png) depending on input.",
		)
		resize = pflag.IntP(
			"resize",
			"r",
			5,
			"Quality of resize algorithm. Values range from 0 to 5, fastest to slowest execution time. Defaults to slowest for best quality.",
		)
	)
	pflag.Parse()

	var (
		input  io.Reader
		output io.Writer
	)
	// An explicit --input wins; otherwise a non-terminal stdin means we are
	// part of a pipeline and both paths are ignored.
	piping := false
	if *inputPath == "" {
		var err error
		if piping, err = stdinIsPipe(); err != nil {
			return err
		}
	}
	in, out, algorithm := sanitiseInputs(*inputPath, *outputPath, *resize)
	if piping {
		input, output = os.Stdin, os.Stdout
	} else {
		if in == "" {
			usage()
			return errUsage
		}
		sourcef, err := fs.Open(in)
		if err != nil {
			return fmt.Errorf("opening source image: %w", err)
		}
		defer sourcef.Close()
		input = sourcef
		if err := fs.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("preparing output directory: %w", err)
		}
		outputf, err := fs.Create(out)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer outputf.Close()
		output = outputf
	}
	if filepath.Ext(in) == ".icns" {
		by, err := io.ReadAll(input)
		if err != nil {
			return fmt.Errorf("probing file: reading file: %w", err)
		}
		icons, err := icns.Probe(bytes.NewReader(by))
		if err != nil {
			return fmt.Errorf("probing file: %w", err)
		}
		for _, icon := range icons {
			slog.Info("found", "icon", icon)
		}
		input = bytes.NewReader(by)
	}
	img, format, err := image.Decode(input)
	if err != nil {
		return fmt.Errorf("decoding input: %w", err)
	}
	if format == "icns" {
		imageType := strings.ToLower(filepath.Ext(out))
		if _, ok := encoders[imageType]; !ok {
			imageType = ".png"
		}
		if err := encoders[imageType](output, img); err != nil {
			return fmt.Errorf("encoding %s: %w", imageType, err)
		}
		return nil
	}
	enc := icns.NewEncoder(output).WithAlgorithm(algorithm)
	if err := enc.Encode(img); err != nil {
		return fmt.Errorf("encoding icns: %w", err)
	}
	return nil
}

func sanitiseInputs(
	inputPath string,
	outputPath string,
	resize int,
) (string, string, icns.InterpolationFunction) {
	if filepath.Ext(inputPath) == ".icns" {
		if outputPath == "" {
			outputPath = changeExtensionTo(inputPath, "png")
		}
		if filepath.Ext(outputPath) == "" {
			outputPath += ".png"
		}
	}
	if filepath.Ext(inputPath) != ".icns" {
		if outputPath == "" {
			outputPath = changeExtensionTo(inputPath, "icns")
		}
		if filepath.Ext(outputPath) == "" {
			outputPath += ".icns"
		}
	}
	if resize < 0 {
		resize = 0
	}
	if resize > 5 {
		resize = 5
	}
	return inputPath, outputPath, icns.InterpolationFunction(resize)
}

func changeExtensionTo(path, ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return filepath.Base(path[:len(path)-len(filepath.Ext(path))] + ext)
}

type encoderFunc func(io.Writer, image.Image) error

func encodeJPEG(w io.Writer, m image.Image) error {
	return jpeg.Encode(w, m, &jpeg.Options{Quality: 100})
}

var encoders = map[string]encoderFunc{
	".png":  png.Encode,
	".jpg":  encodeJPEG,
	".jpeg": encodeJPEG,
}
