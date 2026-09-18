// Package ico implements an encoder and decoder for the Windows icon format.
//
// An ico file holds several sizes of the same icon, the way an icns does, so
// a program that ships both writes one source image to each and lets the
// platform pick. The sizes written are the ones Windows draws, from 16 up to
// 256 pixels.
//
// Each icon is stored either as a device independent bitmap, which every
// version of Windows reads, or as a PNG, which Vista introduced and which
// keeps the largest size from dominating the file.
package ico

import (
	"errors"
	"fmt"
	"image"
	"io"

	"github.com/jackmordaunt/icns/v4/internal/resample"
)

// InterpolationFunction is the algorithm used to resize the image.
type InterpolationFunction = resample.Function

// InterpolationFunction constants, ordered from fastest to highest quality.
const (
	// Nearest-neighbor interpolation
	NearestNeighbor = resample.NearestNeighbor
	// Bilinear interpolation
	Bilinear = resample.Bilinear
	// Bicubic interpolation (with cubic hermite spline)
	Bicubic = resample.Bicubic
	// Mitchell-Netravali interpolation
	MitchellNetravali = resample.MitchellNetravali
	// Lanczos interpolation (a=2)
	Lanczos2 = resample.Lanczos2
	// Lanczos interpolation (a=3)
	Lanczos3 = resample.Lanczos3
)

// Errors returned by the decoder. They are wrapped with detail, so compare
// with errors.Is.
var (
	// ErrInvalidHeader means the data does not begin with an icon directory.
	ErrInvalidHeader = errors.New("invalid header for ico file")
	// ErrMalformed means an offset or length disagrees with the data
	// present; the file is truncated or corrupt.
	ErrMalformed = errors.New("malformed ico file")
	// ErrNoIcons means the directory holds no entries.
	ErrNoIcons = errors.New("no icons found")
	// ErrUnsupportedFormat means an icon is stored in a way this package
	// cannot read, and no registered decoder reads it either.
	ErrUnsupportedFormat = errors.New("unsupported image format")
)

// ErrImageTooSmall is returned when the image is too small to process.
type ErrImageTooSmall struct {
	need  int
	image image.Image
}

func (err ErrImageTooSmall) Error() string {
	b := err.image.Bounds()
	return fmt.Sprintf("image is too small: %dx%d, need at least %dx%d", b.Dx(), b.Dy(), err.need, err.need)
}

// Format is how an icon's pixels are stored inside the file.
type Format int

const (
	// FormatBMP is a device independent bitmap with a one bit mask.
	FormatBMP Format = iota
	// FormatPNG is a whole PNG file.
	FormatPNG
)

func (f Format) String() string {
	switch f {
	case FormatBMP:
		return "BMP"
	case FormatPNG:
		return "PNG"
	}
	return fmt.Sprintf("unknown format %d", f)
}

// sizes are the icon sizes written, largest first. Windows draws icons at
// each of them, and 256 is the largest the format addresses.
var sizes = []uint{256, 128, 64, 48, 32, 24, 16}

// Sizes returns the icon sizes an encoded file holds, largest first.
func Sizes() []uint {
	out := make([]uint, len(sizes))
	copy(out, sizes)
	return out
}

// smallest is the size below which there is nothing worth writing.
const smallest = 16

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
	return enc.write(nil, img)
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
	return enc.write(images, source)
}

// Encode writes img to wr in ico format, at every size the file holds.
func Encode(wr io.Writer, img image.Image) error {
	return NewEncoder(wr).Encode(img)
}
