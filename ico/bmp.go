package ico

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// TODO(jfm): can we just use the standard bmp decoder from x/image/bmp?

// encodeBMP writes the bitmap an icon holds: a header, the pixels bottom up
// in blue, green, red, alpha order, then the one bit mask that predates the
// alpha channel and that some of Windows still reads.
func encodeBMP(img image.Image, size int) []byte {
	var (
		maskStride = ((size + 31) / 32) * 4
		pixels     = size * size * 4
		out        = make([]byte, 0, headerSize+pixels+maskStride*size)
	)
	out = binary.LittleEndian.AppendUint32(out, headerSize)
	out = binary.LittleEndian.AppendUint32(out, uint32(size))
	// The height covers the pixels and the mask together.
	out = binary.LittleEndian.AppendUint32(out, uint32(size*2))
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, 32)
	out = binary.LittleEndian.AppendUint32(out, 0) // Uncompressed.
	out = binary.LittleEndian.AppendUint32(out, uint32(pixels+maskStride*size))
	out = binary.LittleEndian.AppendUint32(out, 0) // Pixels per metre, across.
	out = binary.LittleEndian.AppendUint32(out, 0) // Pixels per metre, down.
	out = binary.LittleEndian.AppendUint32(out, 0) // Colours used.
	out = binary.LittleEndian.AppendUint32(out, 0) // Colours that matter.

	origin := img.Bounds().Min
	mask := make([]byte, maskStride*size)
	for y := size - 1; y >= 0; y-- {
		for x := range size {
			c := color.NRGBAModel.Convert(img.At(origin.X+x, origin.Y+y)).(color.NRGBA)
			out = append(out, c.B, c.G, c.R, c.A)
			if c.A == 0 {
				// A set bit means the background shows through.
				row := (size - 1 - y) * maskStride
				mask[row+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	return append(out, mask...)
}

// decodeBMP reads the device independent bitmap an icon holds: a header, a
// colour table when the pixels are indexed, the pixels bottom up, and a one
// bit mask. The stored height covers the pixels and the mask together, so it
// is twice the height of the icon.
func decodeBMP(data []byte) (image.Image, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("%w: holds %d bytes, too few for a bitmap header", ErrMalformed, len(data))
	}
	var (
		infoSize    = int(binary.LittleEndian.Uint32(data[0:4]))
		w           = int(int32(binary.LittleEndian.Uint32(data[4:8])))
		storedH     = int(int32(binary.LittleEndian.Uint32(data[8:12])))
		bits        = int(binary.LittleEndian.Uint16(data[14:16]))
		compression = binary.LittleEndian.Uint32(data[16:20])
		colours     = int(binary.LittleEndian.Uint32(data[32:36]))
	)
	if compression != 0 {
		return nil, fmt.Errorf("%w: the bitmap is compressed", ErrUnsupportedFormat)
	}
	switch bits {
	case 1, 4, 8, 24, 32:
	default:
		return nil, fmt.Errorf("%w: %d bits per pixel", ErrUnsupportedFormat, bits)
	}
	h := storedH / 2
	if storedH <= 0 || storedH%2 != 0 || w <= 0 {
		return nil, fmt.Errorf("%w: the bitmap is %dx%d", ErrMalformed, w, storedH)
	}
	if infoSize < headerSize || infoSize > len(data) {
		return nil, fmt.Errorf("%w: the header claims %d bytes", ErrMalformed, infoSize)
	}

	// An indexed bitmap carries its own colour table, so nothing outside the
	// file is needed to read it.
	var palette []color.NRGBA
	offset := infoSize
	if bits <= 8 {
		entries := colours
		if entries == 0 {
			entries = 1 << bits
		}
		if offset+entries*4 > len(data) {
			return nil, fmt.Errorf("%w: the colour table of %d runs past the icon", ErrMalformed, entries)
		}
		palette = make([]color.NRGBA, entries)
		for i := range palette {
			at := offset + i*4
			palette[i] = color.NRGBA{R: data[at+2], G: data[at+1], B: data[at], A: 0xFF}
		}
		offset += entries * 4
	}

	var (
		stride     = ((w*bits + 31) / 32) * 4
		maskStride = ((w + 31) / 32) * 4
		pixelBytes = stride * h
	)
	if offset+pixelBytes > len(data) {
		return nil, fmt.Errorf("%w: %d bytes of pixels do not fit in the icon", ErrMalformed, pixelBytes)
	}
	pixels := data[offset : offset+pixelBytes]
	// The mask is optional in practice: a truncated icon still draws.
	var mask []byte
	if end := offset + pixelBytes + maskStride*h; end <= len(data) {
		mask = data[offset+pixelBytes : end]
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	var opaque bool
	for y := range h {
		row := pixels[(h-1-y)*stride:]
		for x := range w {
			c, err := pixelAt(row, x, bits, palette)
			if err != nil {
				return nil, err
			}
			if c.A != 0 {
				opaque = true
			}
			img.SetNRGBA(x, y, c)
		}
	}
	// A 32 bit icon carries its own alpha, unless whoever wrote it left the
	// channel empty and meant the mask to be read instead.
	if mask != nil && (bits != 32 || !opaque) {
		for y := range h {
			row := mask[(h-1-y)*maskStride:]
			for x := range w {
				c := img.NRGBAAt(x, y)
				c.A = 0xFF
				if row[x/8]&(0x80>>(x%8)) != 0 {
					c.A = 0
				}
				img.SetNRGBA(x, y, c)
			}
		}
	}
	return img, nil
}

// pixelAt reads one pixel out of a bitmap row.
func pixelAt(row []byte, x, bits int, palette []color.NRGBA) (color.NRGBA, error) {
	index := func(i byte) (color.NRGBA, error) {
		if int(i) >= len(palette) {
			return color.NRGBA{}, fmt.Errorf("%w: colour %d is outside a table of %d", ErrMalformed, i, len(palette))
		}
		return palette[i], nil
	}
	switch bits {
	case 32:
		return color.NRGBA{R: row[x*4+2], G: row[x*4+1], B: row[x*4], A: row[x*4+3]}, nil
	case 24:
		return color.NRGBA{R: row[x*3+2], G: row[x*3+1], B: row[x*3], A: 0xFF}, nil
	case 8:
		return index(row[x])
	case 4:
		if x%2 == 0 {
			return index(row[x/2] >> 4)
		}
		return index(row[x/2] & 0x0F)
	default:
		return index((row[x/8] >> (7 - x%8)) & 1)
	}
}
