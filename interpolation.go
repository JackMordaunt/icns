package icns

import (
	"math"

	"golang.org/x/image/draw"
)

// InterpolationFunction is the algorithm used to resize the image.
//
// It is an enumeration of this package's own rather than an alias for the
// resampling library's type, so the resampler can be replaced without
// breaking callers.
type InterpolationFunction int

// InterpolationFunction constants, ordered from fastest to highest quality.
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

// scaler returns the resampler implementing f. Values outside the
// enumeration fall back to MitchellNetravali, the package default.
func (f InterpolationFunction) scaler() draw.Interpolator {
	switch f {
	case NearestNeighbor:
		return draw.NearestNeighbor
	case Bilinear:
		return draw.BiLinear
	case Bicubic:
		// Catmull-Rom is the cubic hermite spline interpolant.
		return draw.CatmullRom
	case Lanczos2:
		return lanczos2
	case Lanczos3:
		return lanczos3
	default:
		return mitchellNetravali
	}
}

// Kernels receive the distance from the sample as a non-negative value, and
// return the weight to give the pixel at that distance.

// mitchellNetravali is the Mitchell-Netravali filter with B = C = 1/3, the
// parameters its authors found to be the best compromise between blurring
// and ringing.
var mitchellNetravali = &draw.Kernel{Support: 2, At: func(t float64) float64 {
	const b, c = 1.0 / 3.0, 1.0 / 3.0
	switch {
	case t < 1:
		return ((12-9*b-6*c)*t*t*t + (-18+12*b+6*c)*t*t + (6 - 2*b)) / 6
	case t < 2:
		return ((-b-6*c)*t*t*t + (6*b+30*c)*t*t + (-12*b-48*c)*t + (8*b + 24*c)) / 6
	default:
		return 0
	}
}}

var (
	lanczos2 = lanczos(2)
	lanczos3 = lanczos(3)
)

// lanczos returns the Lanczos filter of order a: a sinc windowed by another
// sinc stretched across the kernel's whole support.
func lanczos(a float64) *draw.Kernel {
	return &draw.Kernel{Support: a, At: func(t float64) float64 {
		switch {
		case t == 0:
			return 1
		case t < a:
			return a * math.Sin(math.Pi*t) * math.Sin(math.Pi*t/a) / (math.Pi * math.Pi * t * t)
		default:
			return 0
		}
	}}
}
