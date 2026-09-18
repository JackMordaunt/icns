package ico

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"github.com/jackmordaunt/icns/v4/internal/resample"
)

const (
	// directorySize is the header that counts the icons.
	directorySize = 6
	// entrySize is one icon's row in the directory.
	entrySize = 16
	// headerSize is the BITMAPINFOHEADER every bitmap icon begins with.
	headerSize = 40
	// pngAbove is the size from which a PNG is written instead of a bitmap:
	// a 256 pixel bitmap is a quarter of a megabyte on its own.
	pngAbove = 256
)

// write encodes an icon at every size up to the source, taking artwork from
// images where a size has its own.
func (enc *Encoder) write(images map[uint]image.Image, source image.Image) error {
	if resample.BiggestSide(source) < smallest {
		return ErrImageTooSmall{image: source, need: smallest}
	}
	type icon struct {
		size uint
		data []byte
	}
	var icons []icon
	for _, size := range sizes {
		if size > resample.BiggestSide(source) {
			continue
		}
		art := source
		if supplied, ok := images[size]; ok {
			art = supplied
		}
		scaled := resample.Square(art, size, enc.Algorithm)
		data, err := encodeIcon(scaled, int(size))
		if err != nil {
			return err
		}
		icons = append(icons, icon{size: size, data: data})
	}
	if len(icons) == 0 {
		return ErrImageTooSmall{image: source, need: smallest}
	}

	out := make([]byte, 0, directorySize+entrySize*len(icons))
	out = binary.LittleEndian.AppendUint16(out, 0) // Reserved.
	out = binary.LittleEndian.AppendUint16(out, 1) // An icon, not a cursor.
	out = binary.LittleEndian.AppendUint16(out, uint16(len(icons)))
	offset := directorySize + entrySize*len(icons)
	for _, ic := range icons {
		// 256 does not fit in a byte and is written as zero.
		side := byte(ic.size)
		out = append(out, side, side, 0, 0)
		out = binary.LittleEndian.AppendUint16(out, 1)  // Colour planes.
		out = binary.LittleEndian.AppendUint16(out, 32) // Bits per pixel.
		out = binary.LittleEndian.AppendUint32(out, uint32(len(ic.data)))
		out = binary.LittleEndian.AppendUint32(out, uint32(offset))
		offset += len(ic.data)
	}
	if _, err := enc.Wr.Write(out); err != nil {
		return err
	}
	for _, ic := range icons {
		if _, err := enc.Wr.Write(ic.data); err != nil {
			return err
		}
	}
	return nil
}

// encodeIcon stores one icon, as a PNG at the largest size and as a bitmap
// below it.
func encodeIcon(img image.Image, size int) ([]byte, error) {
	if size >= pngAbove {
		buf := bytes.NewBuffer(nil)
		if err := png.Encode(buf, withAlpha{img}); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	return encodeBMP(img, size), nil
}

// withAlpha reports that an image has transparency whatever its pixels hold,
// so image/png gives the icon an alpha channel. Windows reads a PNG icon only
// when it is stored as 32 bit RGBA, and the encoder writes 24 bit truecolour
// for an image that is entirely opaque.
type withAlpha struct{ image.Image }

func (withAlpha) Opaque() bool { return false }

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
		for x := 0; x < size; x++ {
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
