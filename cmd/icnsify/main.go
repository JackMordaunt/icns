package main

import (
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackmordaunt/icns"
	"github.com/spf13/afero"

	"github.com/spf13/pflag"
)

var (
	fs     = afero.NewOsFs()
	piping bool
	input  io.Reader
	output io.Writer
)

func main() {
	var (
		inputPath  = pflag.StringP("input", "i", "", "Input image for conversion to icns (jpg|png)")
		outputPath = pflag.StringP("output", "o", "", "Output path, defaults to <path/to/image>.icns")
		resize     = pflag.IntP("resize", "r", 5, "Quality of resize algorithm. Values range from 0 to 5, fastest to slowest execution time. Defaults to slowest for best quality.")
	)
	pflag.Parse()
	in, out, algorithm := sanitiseInputs(*inputPath, *outputPath, *resize)
	if !piping {
		if in == "" {
			usage()
			os.Exit(0)
		}
		sourcef, err := fs.Open(in)
		if err != nil {
			log.Fatalf("opening source image: %v", err)
		}
		defer sourcef.Close()
		input = sourcef
		if err := fs.MkdirAll(filepath.Dir(out), 0755); err != nil {
			log.Fatalf("preparing output directory: %v", err)
		}
		outputf, err := fs.Create(out)
		if err != nil {
			log.Fatalf("creating icns file: %v", err)
		}
		defer outputf.Close()
		output = outputf
	}
	img, format, err := image.Decode(input)
	if err != nil {
		log.Fatalf("decoding image: %v", err)
	}
	enc := icns.NewEncoder(output).
		WithAlgorithm(algorithm).
		WithFormat(format)
	if err := enc.Encode(img); err != nil {
		log.Fatalf("encoding icns: %v", err)
	}
}

func sanitiseInputs(
	inputPath string,
	outputPath string,
	resize int,
) (string, string, icns.InterpolationFunction) {
	if inputPath != "" && outputPath == "" {
		outputPath = changeExtensionTo(inputPath, "icns")
	}
	if filepath.Ext(outputPath) == "" {
		outputPath += ".icns"
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
