package main

import (
	"image"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackmordaunt/icns"
	"github.com/spf13/afero"

	"github.com/spf13/pflag"
)

var fs = afero.NewOsFs()

func main() {
	var (
		input  = pflag.StringP("input", "i", "", "Input image for conversion to icns (jpg|png)")
		output = pflag.StringP("output", "o", "", "Output path, defaults to <path/to/image>.icns")
		resize = pflag.IntP("resize", "r", 5, "Quality of resize algorithm. Values range from 0 to 5, fastest to highest quality. Defaults to highest quality.")
	)
	pflag.Parse()
	in, out, algorithm := sanitiseInputs(*input, *output, *resize)
	sourcef, err := fs.Open(in)
	if err != nil {
		log.Fatalf("opening source image: %v", err)
	}
	defer sourcef.Close()
	img, _, err := image.Decode(sourcef)
	if err != nil {
		log.Fatalf("decoding image: %v", err)
	}
	if err := fs.MkdirAll(filepath.Dir(out), 0755); err != nil {
		log.Fatalf("preparing output directory: %v", err)
	}
	outputf, err := fs.Create(out)
	if err != nil {
		log.Fatalf("creating icns file: %v", err)
	}
	defer outputf.Close()
	if err := icns.EncodeWithInterpolationFunction(
		outputf,
		img,
		algorithm,
	); err != nil {
		log.Fatalf("encoding icns: %v", err)
	}
}

func sanitiseInputs(
	inputPath string,
	outputPath string,
	resize int,
) (string, string, icns.InterpolationFunction) {
	if inputPath == "" {
		pflag.Usage()
		os.Exit(0)
	}
	if outputPath == "" {
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
