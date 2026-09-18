package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/jackmordaunt/icns/v4"
)

func solid(side int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// TestPixelSizes covers the slots that land on the same number of pixels,
// where only one drawing can be kept.
func TestPixelSizes(t *testing.T) {
	t.Parallel()
	var (
		plain  = color.NRGBA{R: 0xFF, A: 0xFF}
		retina = color.NRGBA{B: 0xFF, A: 0xFF}
	)
	images := map[icns.Slot]image.Image{
		{Points: 16, Scale: 1}:  solid(16, plain),
		{Points: 16, Scale: 2}:  solid(32, retina),
		{Points: 32, Scale: 1}:  solid(32, plain),
		{Points: 512, Scale: 2}: solid(1024, retina),
	}
	got := pixelSizes(images)
	if len(got) != 3 {
		t.Fatalf("mapped to %d sizes, want 3", len(got))
	}
	for size, want := range map[uint]color.NRGBA{16: plain, 32: plain, 1024: retina} {
		img, ok := got[size]
		if !ok {
			t.Errorf("no drawing at %d pixels", size)
			continue
		}
		if img.Bounds().Dx() != int(size) {
			t.Errorf("the drawing at %d pixels is %d wide", size, img.Bounds().Dx())
		}
		if c := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); c != want {
			t.Errorf("the drawing at %d pixels is %v, want %v", size, c, want)
		}
	}
}
