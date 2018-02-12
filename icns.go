package icns

import (
	"errors"
	"image"
	"io"

	"github.com/nfnt/resize"
)

// Encoder encodes ICNS files from a source image.
type Encoder struct {
	Wr          io.Writer
	Image       image.Image
	Algorithm   InterpolationFunction
	ImageFormat string
}

// NewEncoder initialises an encoder.
func NewEncoder(wr io.Writer, img image.Image) *Encoder {
	return &Encoder{
		Wr:    wr,
		Image: img,
	}
}

// WithAlgorithm applies the interpolation function used to resize the image.
func (enc *Encoder) WithAlgorithm(a InterpolationFunction) *Encoder {
	enc.Algorithm = a
	return enc
}

// WithFormat applies the image format identifier used during registration by
// image/png and image/jpeg packages.
func (enc *Encoder) WithFormat(format string) *Encoder {
	enc.ImageFormat = format
	return enc
}

// Encode icns with the given configuration.
func (enc *Encoder) Encode() error {
	if enc.Wr == nil {
		return errors.New("cannot write to nil writer")
	}
	if enc.Image == nil {
		return errors.New("cannot process nil image")
	}
	iconset, err := NewIconSet(enc.Image, enc.Algorithm, enc.ImageFormat)
	if err != nil {
		return err
	}
	if _, err := iconset.WriteTo(enc.Wr); err != nil {
		return err
	}
	return nil
}

// Encode writes img to wr in ICNS format.
// img is assumed to be a rectangle; non-square dimensions will be squared
// without preserving the aspect ratio.
// Uses nearest neighbor as interpolation algorithm.
func Encode(wr io.Writer, img image.Image) error {
	return NewEncoder(wr, img).Encode()
}

// NewIconSet uses the source image to create an IconSet.
// If width != height, the image will be resized using the largest side without
// preserving the aspect ratio.
func NewIconSet(img image.Image, interp InterpolationFunction, format string) (*IconSet, error) {
	biggest := findNearestSize(img)
	if biggest == 0 {
		return nil, ErrImageTooSmall{image: img, need: 16}
	}
	icons := []*Icon{}
	for _, size := range sizesFrom(biggest) {
		t, ok := getType(size)
		if !ok {
			continue
		}
		iconImg := resize.Resize(size, size, img, interp)
		icon := &Icon{
			Type:   t,
			Image:  iconImg,
			format: format,
		}
		icons = append(icons, icon)
	}
	iconset := &IconSet{
		Icons: icons,
	}
	return iconset, nil
}

// Big-endian.
// https://golang.org/src/image/png/writer.go
func writeUint32(b []uint8, u uint32) {
	b[0] = uint8(u >> 24)
	b[1] = uint8(u >> 16)
	b[2] = uint8(u >> 8)
	b[3] = uint8(u >> 0)
}

var sizes = []uint{
	1024,
	512,
	256,
	64,
	32,
}

// findNearestSize finds the biggest icon size we can use for this image.
func findNearestSize(img image.Image) uint {
	size := biggestSide(img)
	for _, s := range sizes {
		if size >= s {
			return s
		}
	}
	return 0
}

func biggestSide(img image.Image) uint {
	var size uint
	b := img.Bounds()
	w, h := uint(b.Max.X), uint(b.Max.Y)
	size = w
	if h > size {
		size = h
	}
	return size
}

// sizesFrom returns a slice containing the sizes less than and including max.
func sizesFrom(max uint) []uint {
	for ii, s := range sizes {
		if s <= max {
			return sizes[ii:len(sizes)]
		}
	}
	return nil
}

// OsType is a 4 character identifier used to differentiate icon types.
type OsType string

// getType returns the type for the given icon size (in px).
// The boolean indicates whether the type exists.
func getType(size uint) (OsType, bool) {
	// 'types' is a map of the OSTypes we care about.
	// All dimensions are considered as retina.
	//
	// Todo(jackmordaunt): Not sure if only retina is sufficient. Should all
	// types be handled? `iconutil` uses file names to determine whether a
	// retina image is desired eg: "icon_256x256@2.png", without such a hint
	// how can you disambiguate 256x256 standard vs 256x256 retina?
	// Do we even need to consider standard sizes over retina?
	// For now, just retina types are considered.
	types := map[uint]OsType{
		1024: "ic10",
		512:  "ic14",
		256:  "ic13",
		128:  "ic07",
		64:   "ic12",
		32:   "ic11",
	}
	v, ok := types[size]
	return v, ok
}
