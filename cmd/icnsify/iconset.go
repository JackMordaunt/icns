package main

import (
	"cmp"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/jackmordaunt/icns/v4"
	"github.com/jackmordaunt/icns/v4/ico"
)

// encodeIconSet writes an icon container from an iconset directory, the
// layout iconutil accepts: one PNG per slot, named icon_16x16.png,
// icon_16x16@2x.png and so on. Files that are not named that way are ignored.
func encodeIconSet(dir, out, target string, algorithm icns.InterpolationFunction) error {
	if !containers[target] {
		return fmt.Errorf("an iconset directory makes an icns or an ico, not %s", target)
	}
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
	if target == ".ico" {
		if err := ico.NewEncoder(f).WithAlgorithm(algorithm).EncodeSizes(pixelSizes(images)); err != nil {
			return fmt.Errorf("encoding ico: %w", err)
		}
		return nil
	}
	if err := icns.NewEncoder(f).WithAlgorithm(algorithm).EncodeSlots(images); err != nil {
		return fmt.Errorf("encoding icns: %w", err)
	}
	return nil
}

// pixelSizes maps iconset slots onto the sizes an ico holds. Two slots can be
// the same number of pixels, 16x16@2x and 32x32 both being 32, and the
// drawing made without scaling is the one that belongs at that size.
func pixelSizes(images map[icns.Slot]image.Image) map[uint]image.Image {
	slots := make([]icns.Slot, 0, len(images))
	for slot := range images {
		slots = append(slots, slot)
	}
	// Highest scale first, so a plain drawing displaces a retina one.
	slices.SortFunc(slots, func(a, b icns.Slot) int {
		return cmp.Compare(b.Scale, a.Scale)
	})
	out := make(map[uint]image.Image, len(slots))
	for _, slot := range slots {
		out[slot.Pixels()] = images[slot]
	}
	return out
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
