package ico

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"

	"github.com/jackmordaunt/icns/v4/internal/resample"
)

// Encoder encodes ico files from a source image.
type Encoder struct {
	Wr        io.Writer
	Algorithm InterpolationFunction
}

// NewEncoder initialises an encoder.
func NewEncoder(wr io.Writer) *Encoder {
	return &Encoder{
		Wr:        wr,
		Algorithm: MitchellNetravali,
	}
}

// WithAlgorithm applies the interpolation function used to resize the image.
func (enc *Encoder) WithAlgorithm(a InterpolationFunction) *Encoder {
	enc.Algorithm = a
	return enc
}

// Encode writes the image at every size the file holds, resizing it for each.
func (enc *Encoder) Encode(img image.Image) error {
	if enc.Wr == nil {
		return errors.New("cannot write to nil writer")
	}
	if img == nil {
		return errors.New("cannot encode nil image")
	}
	return enc.encode(nil, img)
}

// EncodeSizes writes artwork supplied per size, so a drawing made for one
// size is used there rather than reduced from a larger one. Sizes given no
// artwork are filled from the largest image supplied, which also sets the
// largest icon written.
func (enc *Encoder) EncodeSizes(images map[uint]image.Image) error {
	if enc.Wr == nil {
		return errors.New("cannot write to nil writer")
	}
	if len(images) == 0 {
		return errors.New("cannot encode without an image")
	}
	var source image.Image
	for _, size := range sizes {
		img, ok := images[size]
		if !ok {
			continue
		}
		if img == nil {
			return fmt.Errorf("cannot encode nil image for %d", size)
		}
		// sizes runs largest first, so the first match is the biggest slot
		// that was filled.
		if source == nil || resample.BiggestSide(img) > resample.BiggestSide(source) {
			source = img
		}
	}
	if source == nil {
		return errors.New("no image was given for a size this format holds")
	}
	return enc.encode(images, source)
}

// Encode writes img to wr in ico format, at every size the file holds.
func Encode(wr io.Writer, img image.Image) error {
	return NewEncoder(wr).Encode(img)
}

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

// encode encodes an icon at every size up to the source, taking artwork from
// images where a size has its own.
func (enc *Encoder) encode(images map[uint]image.Image, source image.Image) error {
	if resample.BiggestSide(source) < smallest {
		return ErrImageTooSmall{image: source, need: smallest}
	}

	icons := []Icon{}
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
			return fmt.Errorf("encoding: %w", err)
		}

		icons = append(icons, Icon{
			Width:  int(size),
			Height: int(size),
			Planes: 1,
			Bits:   32,
			Data:   data,
		})
	}

	if len(icons) == 0 {
		return ErrImageTooSmall{image: source, need: smallest}
	}

	out, err := Assemble(icons)
	if err != nil {
		return fmt.Errorf("assembling: %w", err)
	}

	if _, err := enc.Wr.Write(out); err != nil {
		return fmt.Errorf("writing: %w", err)
	}

	return nil
}

// encodeIcon stores one icon, as a PNG at the largest size and as a bitmap
// below it.
func encodeIcon(img image.Image, size int) ([]byte, error) {
	if size >= pngAbove {
		buf := bytes.NewBuffer(nil)
		if err := png.Encode(buf, withAlpha{img}); err != nil {
			return nil, fmt.Errorf("png: %w", err)
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
