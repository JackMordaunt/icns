package icns

import (
	"fmt"
	"image"
	"image/color"
)

// Legacy icon types store their colour as three run-length encoded planes,
// one per channel, and their alpha in a separate mask element.

// unpackRLE expands the icns variant of PackBits until want bytes are
// produced. A lead byte below 128 introduces lead+1 literal bytes; a lead
// byte of 128 or above repeats the byte after it lead-125 times. Data that is
// already want bytes long is stored uncompressed and is returned as it is.
func unpackRLE(data []byte, want int) ([]byte, error) {
	if len(data) == want {
		return data, nil
	}
	out := make([]byte, 0, want)
	for i := 0; i < len(data) && len(out) < want; {
		lead := int(data[i])
		i++
		if lead < 128 {
			n := lead + 1
			if i+n > len(data) {
				return nil, fmt.Errorf("%w: literal run of %d bytes overruns the element", ErrMalformed, n)
			}
			out = append(out, data[i:i+n]...)
			i += n
			continue
		}
		if i == len(data) {
			return nil, fmt.Errorf("%w: repeat run with no byte to repeat", ErrMalformed)
		}
		for n := lead - 125; n > 0; n-- {
			out = append(out, data[i])
		}
		i++
	}
	if len(out) != want {
		return nil, fmt.Errorf("%w: expanded to %d bytes, want %d", ErrMalformed, len(out), want)
	}
	return out, nil
}

// packRLE compresses data into the icns variant of PackBits. A run of three
// or more equal bytes is worth encoding, since it costs two bytes either way,
// and anything shorter goes out as literals. Data that does not compress is
// returned unchanged, which the decoder recognises by its length.
func packRLE(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		run := 1
		for i+run < len(data) && run < maxRepeat && data[i+run] == data[i] {
			run++
		}
		if run >= 3 {
			out = append(out, byte(run+125), data[i])
			i += run
			continue
		}
		// Literals up to the next run of three, since that run encodes more
		// cheaply on its own.
		start := i
		for i < len(data) && i-start < maxLiteral {
			if i+2 < len(data) && data[i] == data[i+1] && data[i] == data[i+2] {
				break
			}
			i++
		}
		out = append(out, byte(i-start-1))
		out = append(out, data[start:i]...)
	}
	if len(out) >= len(data) {
		return data
	}
	return out
}

const (
	// maxLiteral is the longest literal run, from a lead byte of 127.
	maxLiteral = 128
	// maxRepeat is the longest repeat, from a lead byte of 255.
	maxRepeat = 130
)

// splitPlanes separates an image into the three colour planes and the alpha
// mask that the legacy elements store separately. The planes hold straight
// colour, so alpha is divided back out.
func splitPlanes(img image.Image, side int) (planes, mask []byte) {
	pixels := side * side
	planes = make([]byte, pixels*3)
	mask = make([]byte, pixels)
	origin := img.Bounds().Min
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			c := color.NRGBAModel.Convert(img.At(origin.X+x, origin.Y+y)).(color.NRGBA)
			i := y*side + x
			planes[i] = c.R
			planes[pixels+i] = c.G
			planes[pixels*2+i] = c.B
			mask[i] = c.A
		}
	}
	return planes, mask
}

// decodeRGB builds an image from run-length encoded colour planes and the
// raw alpha of the matching mask element. A missing mask leaves the icon
// opaque, which is how the icons that predate masks are meant to render.
func decodeRGB(data, mask []byte, side int) (image.Image, error) {
	pixels := side * side
	planes, err := unpackRLE(data, pixels*3)
	if err != nil {
		return nil, err
	}
	if mask != nil && len(mask) != pixels {
		return nil, fmt.Errorf("%w: mask holds %d bytes, want %d", ErrMalformed, len(mask), pixels)
	}
	// The planes carry straight colour and the mask carries alpha, so the
	// result is non-premultiplied.
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for i := 0; i < pixels; i++ {
		px := img.Pix[i*4 : i*4+4 : i*4+4]
		px[0] = planes[i]
		px[1] = planes[pixels+i]
		px[2] = planes[pixels*2+i]
		px[3] = 0xFF
		if mask != nil {
			px[3] = mask[i]
		}
	}
	return img, nil
}
