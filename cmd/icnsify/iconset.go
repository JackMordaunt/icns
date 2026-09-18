package main

import (
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jackmordaunt/icns/v4"
)

// encodeIconSet writes an icns from an iconset directory, the layout iconutil
// accepts: one PNG per slot, named icon_16x16.png, icon_16x16@2x.png and so
// on. Files that are not named that way are ignored.
func encodeIconSet(dir, out string, algorithm icns.InterpolationFunction) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading iconset: %w", err)
	}
	images := make(map[icns.Slot]image.Image, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		slot, ok := icns.ParseSlot(entry.Name())
		if !ok {
			continue
		}
		img, err := readPNG(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		images[slot] = img
		slog.Info("found", "slot", slot, "file", entry.Name())
	}
	if len(images) == 0 {
		return fmt.Errorf("no iconset images in %s", dir)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("preparing output directory: %w", err)
	}
	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()
	if err := icns.NewEncoder(f).WithAlgorithm(algorithm).EncodeSlots(images); err != nil {
		return fmt.Errorf("encoding icns: %w", err)
	}
	return nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening iconset image: %w", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", filepath.Base(path), err)
	}
	return img, nil
}
