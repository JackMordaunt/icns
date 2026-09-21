package icns

import (
	"fmt"
	"image"
	"image/color"
)

// The icons that predate Mac OS 8.5 store an index per pixel rather than a
// colour, against a table the system fixed, and keep their alpha as a one bit
// mask in the element named "#" for their size.

// macPalette4 is the sixteen colour table: white, the bright hues, the
// muted ones, then the greys down to black.
var macPalette4 = [16]color.NRGBA{
	{0xFF, 0xFF, 0xFF, 0xFF}, // white
	{0xFC, 0xF3, 0x05, 0xFF}, // yellow
	{0xFF, 0x64, 0x03, 0xFF}, // orange
	{0xDD, 0x09, 0x07, 0xFF}, // red
	{0xF2, 0x08, 0x84, 0xFF}, // magenta
	{0x47, 0x00, 0xA5, 0xFF}, // purple
	{0x00, 0x00, 0xD3, 0xFF}, // blue
	{0x02, 0xAB, 0xEA, 0xFF}, // cyan
	{0x1F, 0xB7, 0x14, 0xFF}, // green
	{0x00, 0x64, 0x12, 0xFF}, // dark green
	{0x56, 0x2C, 0x05, 0xFF}, // brown
	{0x90, 0x71, 0x3A, 0xFF}, // tan
	{0xC0, 0xC0, 0xC0, 0xFF}, // light grey
	{0x80, 0x80, 0x80, 0xFF}, // medium grey
	{0x40, 0x40, 0x40, 0xFF}, // dark grey
	{0x00, 0x00, 0x00, 0xFF}, // black
}

// macPalette8 is the 256 colour table: a six level colour cube with black
// left out, then ten shade ramps of red, green, blue and grey, and black
// last. White is index 0 and black is index 255.
var macPalette8 = buildPalette8()

func buildPalette8() [256]color.NRGBA {
	var (
		palette [256]color.NRGBA
		levels  = [6]uint8{0xFF, 0xCC, 0x99, 0x66, 0x33, 0x00}
		// The shades between the cube's levels, darkest last.
		shades = [10]uint8{0xEE, 0xDD, 0xBB, 0xAA, 0x88, 0x77, 0x55, 0x44, 0x22, 0x11}
		at     int
	)
	for _, r := range levels {
		for _, g := range levels {
			for _, b := range levels {
				if r == 0 && g == 0 && b == 0 {
					continue // Black is held back for the last index.
				}
				palette[at] = color.NRGBA{r, g, b, 0xFF}
				at++
			}
		}
	}
	for _, v := range shades {
		palette[at] = color.NRGBA{v, 0, 0, 0xFF}
		at++
	}
	for _, v := range shades {
		palette[at] = color.NRGBA{0, v, 0, 0xFF}
		at++
	}
	for _, v := range shades {
		palette[at] = color.NRGBA{0, 0, v, 0xFF}
		at++
	}
	for _, v := range shades {
		palette[at] = color.NRGBA{v, v, v, 0xFF}
		at++
	}
	palette[at] = color.NRGBA{0x00, 0x00, 0x00, 0xFF}
	return palette
}

// decodeIndexed builds an image from bits-per-pixel indices and a one bit
// mask. bits is 1, 4 or 8, and a nil mask leaves the icon opaque.
func decodeIndexed(data, mask []byte, w, h, bits int) (image.Image, error) {
	var (
		pixels = w * h
		need   = pixels * bits / 8
		bitmap = pixels / 8
	)
	if len(data) < need {
		return nil, fmt.Errorf("%w: holds %d bytes, want %d for %dx%d at %d bits", ErrMalformed, len(data), need, w, h, bits)
	}
	if mask != nil && len(mask) < bitmap {
		return nil, fmt.Errorf("%w: mask holds %d bytes, want %d", ErrMalformed, len(mask), bitmap)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range pixels {
		var c color.NRGBA
		switch bits {
		case 1:
			// A set bit is ink, which is black on white.
			c = macPalette4[0]
			if data[i/8]&(0x80>>(i%8)) != 0 {
				c = macPalette4[15]
			}
		case 4:
			index := data[i/2] >> 4
			if i%2 == 1 {
				index = data[i/2] & 0x0F
			}
			c = macPalette4[index]
		case 8:
			c = macPalette8[data[i]]
		}
		if mask != nil && mask[i/8]&(0x80>>(i%8)) == 0 {
			c.A = 0
		}
		img.SetNRGBA(i%w, i/w, c)
	}
	return img, nil
}
