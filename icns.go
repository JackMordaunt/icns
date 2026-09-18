package icns

import (
	"errors"
	"fmt"
	"image"
	"io"
	"sync"

	"golang.org/x/image/draw"
)

// Encoder encodes ICNS files from a source image.
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

// Encode icns with the given configuration.
func (enc *Encoder) Encode(img image.Image) error {
	if enc.Wr == nil {
		return errors.New("cannot write to nil writer")
	}
	if img == nil {
		return errors.New("cannot encode nil image")
	}
	iconset, err := NewIconSet(img, enc.Algorithm)
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
	return NewEncoder(wr).Encode(img)
}

// NewIconSet uses the source image to create an IconSet.
// If width != height, the image will be resized using the largest side without
// preserving the aspect ratio.
func NewIconSet(img image.Image, interp InterpolationFunction) (*IconSet, error) {
	biggest := findNearestSize(img)
	if biggest == 0 {
		return nil, ErrImageTooSmall{image: img, need: 16}
	}
	var plan []OsType
	for _, size := range sizesFrom(biggest) {
		types, ok := getTypesFromSize(size)
		if !ok {
			continue
		}
		plan = append(plan, types...)
	}
	icons := make([]*Icon, len(plan))
	work := sync.WaitGroup{}
	for i, osType := range plan {
		work.Add(1)
		go func() {
			defer work.Done()
			icons[i] = &Icon{
				Type:  osType,
				Image: resizeSquare(img, osType.Size, interp),
			}
		}()
	}
	work.Wait()
	iconSet := &IconSet{
		Icons: icons,
	}
	return iconSet, nil
}

// resizeSquare scales img into a size by size square, ignoring the source
// aspect ratio. An image already that size is returned as it is.
//
// Scaling happens in alpha-premultiplied space, so colour does not bleed out
// of fully transparent pixels into the icon's edges.
func resizeSquare(img image.Image, size uint, interp InterpolationFunction) image.Image {
	bounds := img.Bounds()
	if bounds.Dx() == int(size) && bounds.Dy() == int(size) {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, int(size), int(size)))
	interp.scaler().Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	return dst
}

var sizes = []uint{
	1024,
	512,
	256,
	128,
	64,
	32,
	16,
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

// biggestSide returns the larger of img's two dimensions.
func biggestSide(img image.Image) uint {
	b := img.Bounds()
	return uint(max(b.Dx(), b.Dy(), 0))
}

// sizesFrom returns a slice containing the sizes less than and including max.
func sizesFrom(max uint) []uint {
	for ii, s := range sizes {
		if s <= max {
			return sizes[ii:]
		}
	}
	return []uint{}
}

// IconDescription describes an icon.
type IconDescription struct {
	OsType
	ImageFormat
}

func (desc IconDescription) String() string {
	return fmt.Sprintf("%s (%s)", desc.OsType, desc.ImageFormat)
}

// ImageFormat specifies the type of image data associated with an icon.
type ImageFormat int

const (
	ImageFormatPNG ImageFormat = iota
	ImageFormatJPEG2000
	// ImageFormatRGB is 24-bit colour in run-length encoded channel planes,
	// with alpha held in a separate mask element.
	ImageFormatRGB
)

func (f ImageFormat) String() string {
	switch f {
	case ImageFormatPNG:
		return "PNG"
	case ImageFormatJPEG2000:
		return "JPEG 2000"
	case ImageFormatRGB:
		return "24-bit RGB"
	}
	return fmt.Sprintf("unknown format %d", f)
}

// encoding is how an element stores its image data.
type encoding int

const (
	// encodingCompressed holds a whole image file, PNG or JPEG 2000.
	encodingCompressed encoding = iota
	// encodingRGB holds run-length encoded colour planes, with alpha in the
	// separate element named by OsType.mask.
	encodingRGB
)

// OsType is a 4 character identifier used to differentiate icon types.
type OsType struct {
	ID   string
	Size uint

	// enc is how this element stores its image data.
	enc encoding
	// mask is the element holding this type's alpha, for encodingRGB.
	mask string
	// emit marks the types the encoder writes. More types can be read than
	// are written.
	emit bool
}

func (t OsType) String() string {
	return fmt.Sprintf("%s %d", t.ID, t.Size)
}

var osTypes = []OsType{
	{ID: "ic10", Size: 1024, emit: true},
	{ID: "ic14", Size: 512, emit: true},
	{ID: "ic09", Size: 512, emit: true},
	{ID: "ic13", Size: 256, emit: true},
	{ID: "ic08", Size: 256, emit: true},
	{ID: "ic07", Size: 128, emit: true},
	{ID: "ic12", Size: 64, emit: true},
	{ID: "ic11", Size: 32, emit: true},

	{ID: "icp6", Size: 48},
	{ID: "icp5", Size: 32},
	{ID: "icp4", Size: 16},

	{ID: "it32", Size: 128, enc: encodingRGB, mask: "t8mk"},
	{ID: "ih32", Size: 48, enc: encodingRGB, mask: "h8mk"},
	{ID: "il32", Size: 32, enc: encodingRGB, mask: "l8mk"},
	{ID: "is32", Size: 16, enc: encodingRGB, mask: "s8mk"},
}

// getTypesFromSize returns the writable types for the given icon size (in px).
// The boolean indicates whether the types exist.
func getTypesFromSize(size uint) ([]OsType, bool) {
	var retOsTypes []OsType
	for _, t := range osTypes {
		if t.Size == size && t.emit {
			retOsTypes = append(retOsTypes, t)
		}
	}
	return retOsTypes, len(retOsTypes) != 0
}

func getTypeFromID(ID string) (OsType, bool) {
	for _, t := range osTypes {
		if t.ID == ID {
			return t, true
		}
	}
	return OsType{}, false
}

func osTypeFromID(ID string) OsType {
	t, _ := getTypeFromID(ID)
	return t
}
