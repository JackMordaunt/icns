package icns

import (
	"errors"
	"fmt"
	"image"
)

// Errors returned by the decoder. They are wrapped with detail, so compare
// with errors.Is.
var (
	// ErrInvalidHeader means the data does not begin with an icns header.
	ErrInvalidHeader = errors.New("invalid header for icns file")
	// ErrMalformed means an element's declared length disagrees with the
	// data present; the file is truncated or corrupt.
	ErrMalformed = errors.New("malformed icns file")
	// ErrNoIcons means the file is well formed but contains no icons of a
	// type this package understands.
	ErrNoIcons = errors.New("no icons found")
	// ErrUnsupportedFormat means every icon present uses an image format
	// this package cannot decode (JPEG 2000).
	ErrUnsupportedFormat = errors.New("unsupported image format")
)

// ErrImageTooSmall is returned when the image is too small to process.
type ErrImageTooSmall struct {
	need  int
	image image.Image
}

func (err ErrImageTooSmall) Error() string {
	b := err.image.Bounds()
	format := "image is too small: %dx%d, need at least %dx%d"
	return fmt.Sprintf(format, b.Dx(), b.Dy(), err.need, err.need)
}
