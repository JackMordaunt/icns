package icns

import "github.com/jackmordaunt/icns/v4/internal/resample"

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
