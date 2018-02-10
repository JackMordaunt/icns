package icns

import (
	"errors"
	"image"
	"io"

	"github.com/nfnt/resize"
)

// Encode writes img to wr in ICNS format.
// img is assumed to be a rectangle; non-square dimensions will be squared
// without preserving the aspect ratio.
// Uses nearest neighbor as interpolation algorithm.
func Encode(wr io.Writer, img image.Image) error {
	return EncodeWithInterpolationFunction(wr, img, NearestNeighbor)
}

// EncodeWithInterpolationFunction uses the given interpolation function resize
// the image before writing out to wr.
func EncodeWithInterpolationFunction(
	wr io.Writer,
	img image.Image,
	interp InterpolationFunction,
) error {
	if wr == nil {
		return errors.New("cannot write to nil writer")
	}
	if img == nil {
		return errors.New("cannot process nil image")
	}
	iconset, err := NewIconSet(img, interp)
	if err != nil {
		return err
	}
	if _, err := iconset.WriteTo(wr); err != nil {
		return err
	}
	return nil
}

// NewIconSet uses the source image to create an IconSet.
// If width != height, the image will be resized using the largest side without
// preserving the aspect ratio.
func NewIconSet(img image.Image, interp InterpolationFunction) (*IconSet, error) {
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
			Type:  t,
			Image: iconImg,
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
