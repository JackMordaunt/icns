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

// Magic is the four bytes an ico file begins with: two reserved zeroes, then
// the kind, which is 1 for an icon rather than a cursor.
const Magic = "\x00\x00\x01\x00"

// kindIcon is the value of the directory's kind field for an icon, which is
// the only kind this package reads and writes; the same layout with a kind
// of 2 is a cursor.
const kindIcon = 1

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
