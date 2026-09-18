package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackmordaunt/icns/v4"
	"github.com/jackmordaunt/icns/v4/ico"
)

// containers are the formats that hold an icon at several sizes, as opposed
// to the plain images they are built from and unpacked into.
var containers = map[string]bool{".icns": true, ".ico": true}

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
		inputPath    string
		outputPath   string
		outputFormat string
		resize       int
	)
	stringFlag(&inputPath, "input", "i", "",
		"Input image: artwork to pack, an icon file to unpack, or an iconset directory.")
	stringFlag(&outputPath, "output", "o", "",
		"Output path, defaults to the input named with the target's extension.")
	stringFlag(&outputFormat, "format", "f", "",
		"Output format: icns, ico, png or jpg. Defaults from the output path.")
	intFlag(&resize, "resize", "r", 5,
		"Quality of resize algorithm, 0 to 5 from fastest to slowest.")
	var showVersion bool
	boolFlag(&showVersion, "version", "v", "Print the version and exit.")
	flag.Usage = usage
	flag.Parse()

	if showVersion {
		fmt.Println(buildInfo())
		return nil
	}

	var (
		input  io.Reader
		output io.Writer
	)
	// An explicit --input wins; otherwise a non-terminal stdin means we are
	// part of a pipeline and both paths are ignored.
	piping := false
	if inputPath == "" {
		var err error
		if piping, err = stdinIsPipe(); err != nil {
			return err
		}
	}
	if outputFormat != "" && !writable(extension(outputFormat)) {
		return fmt.Errorf("cannot write %s: choose from icns, ico, png or jpg", outputFormat)
	}
	in, out, algorithm := sanitiseInputs(inputPath, outputPath, outputFormat, resize)
	if piping {
		input, output = os.Stdin, os.Stdout
	} else {
		if in == "" {
			usage()
			return errUsage
		}
		// A directory is an iconset: artwork per slot rather than one image
		// to resize for every size.
		if info, err := os.Stat(in); err == nil && info.IsDir() {
			return encodeIconSet(in, out, target(outputFormat, out, false, ""), algorithm)
		}
		sourcef, err := os.Open(in)
		if err != nil {
			return fmt.Errorf("opening source image: %w", err)
		}
		defer sourcef.Close()
		input = sourcef
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("preparing output directory: %w", err)
		}
		outputf, err := os.Create(out)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer outputf.Close()
		output = outputf
	}
	if containers[extension(filepath.Ext(in))] {
		by, err := io.ReadAll(input)
		if err != nil {
			return fmt.Errorf("probing file: reading file: %w", err)
		}
		if err := describe(extension(filepath.Ext(in)), bytes.NewReader(by)); err != nil {
			return fmt.Errorf("probing file: %w", err)
		}
		input = bytes.NewReader(by)
	}
	img, format, err := image.Decode(input)
	if err != nil {
		return fmt.Errorf("decoding input: %w", err)
	}
	switch kind := target(outputFormat, out, piping, format); kind {
	case ".icns":
		if err := icns.NewEncoder(output).WithAlgorithm(algorithm).Encode(img); err != nil {
			return fmt.Errorf("encoding icns: %w", err)
		}
	case ".ico":
		if err := ico.NewEncoder(output).WithAlgorithm(algorithm).Encode(img); err != nil {
			return fmt.Errorf("encoding ico: %w", err)
		}
	default:
		if err := encoders[kind](output, img); err != nil {
			return fmt.Errorf("encoding %s: %w", kind, err)
		}
	}
	return nil
}

// describe logs the icons a container holds.
func describe(ext string, r io.Reader) error {
	switch ext {
	case ".ico":
		d, err := ico.NewDecoder(r)
		if err != nil {
			return err
		}
		for _, icon := range d.Icons() {
			slog.Info("found", "icon", icon)
		}
	default:
		icons, err := icns.Probe(r)
		if err != nil {
			return err
		}
		for _, icon := range icons {
			slog.Info("found", "icon", icon)
		}
	}
	return nil
}

// target names the format to write. The --format flag decides it, then the
// output path, and failing both the conversion changes kind, since that is
// what converting an icon usually means: artwork becomes a container of
// icons and a container becomes a plain image. A pipe has no output path.
func target(want, out string, piping bool, got string) string {
	if want != "" {
		return extension(want)
	}
	if !piping {
		if ext := extension(filepath.Ext(out)); writable(ext) {
			return ext
		}
	}
	if containers[extension(got)] {
		return ".png"
	}
	return ".icns"
}

// extension normalises a format name or extension to a lower case extension.
func extension(name string) string {
	name = strings.ToLower(name)
	if name != "" && !strings.HasPrefix(name, ".") {
		name = "." + name
	}
	return name
}

// writable reports whether this program can write the format.
func writable(ext string) bool {
	return containers[ext] || encoders[ext] != nil
}

func sanitiseInputs(
	inputPath string,
	outputPath string,
	outputFormat string,
	resize int,
) (string, string, icns.InterpolationFunction) {
	ext := target(outputFormat, outputPath, false, extension(filepath.Ext(inputPath)))
	if outputPath == "" {
		outputPath = changeExtensionTo(inputPath, ext)
	}
	if filepath.Ext(outputPath) == "" {
		outputPath += ext
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
