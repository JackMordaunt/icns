package icns

import (
	"image"
	"io"

	"github.com/nfnt/resize"
)

// InterpolationFunction is the algorithm used to resize the image.
type InterpolationFunction = resize.InterpolationFunction

// InterpolationFunction constants.
const (
	// Nearest-neighbor interpolation
	NearestNeighbor InterpolationFunction = iota
	// Bilinear interpolation
	Bilinear
	// Bicubic interpolation (with cubic hermite spline)
	Bicubic
	// Mitchell-Netravali interpolation
	MitchellNetravali
	// Lanczos interpolation (a=2)
	Lanczos2
	// Lanczos interpolation (a=3)
	Lanczos3
)

// EncodeWithInterpolationFunction uses the given interpolation function resize
// the image before writing out to wr.
func EncodeWithInterpolationFunction(
	wr io.Writer,
	img image.Image,
	interp InterpolationFunction,
) error {
	iconset, err := NewIconSet(img, interp)
	if err != nil {
		return err
	}
	if _, err := iconset.WriteTo(wr); err != nil {
		return err
	}
	return nil
}
