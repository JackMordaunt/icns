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
	)
	pflag.Parse()
	in, out := validate(*input, *output)
	sourcef, err := fs.Open(in)
	if err != nil {
		log.Fatalf("opening source image: %v", err)
	}
	defer sourcef.Close()
	img, _, err := image.Decode(sourcef)
	if err != nil {
		log.Fatalf("decoding image: %v", err)
	}
	outputf, err := fs.Create(out)
	if err != nil {
		log.Fatalf("creating icns file: %v", err)
	}
	defer outputf.Close()
	if err := icns.Encode(outputf, img); err != nil {
		log.Fatalf("encoding icns")
	}
}

func validate(inputPath, outputPath string) (string, string) {
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
	return inputPath, outputPath
}

func changeExtensionTo(path, ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return filepath.Base(path[:len(path)-len(filepath.Ext(path))] + ext)
}
