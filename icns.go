package icns

import (
	"cmp"
	"errors"
	"fmt"
	"image"
	"io"
	"slices"
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
	iconset, err := NewIconSet(img, enc.Algorithm)
	if err != nil {
		return err
	}
	_, err = iconset.WriteTo(enc.Wr)
	return err
}

// EncodeSlots icns from artwork supplied per slot, so hand tuned art is used
// where it is given rather than resized from a single source.
func (enc *Encoder) EncodeSlots(images map[Slot]image.Image) error {
	if enc.Wr == nil {
		return errors.New("cannot write to nil writer")
	}
	iconset, err := NewIconSetFrom(images, enc.Algorithm)
	if err != nil {
		return err
	}
	_, err = iconset.WriteTo(enc.Wr)
	return err
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
	if img == nil {
		return nil, errors.New("cannot encode nil image")
	}
	return newIconSet(nil, img, interp)
}

// NewIconSetFrom uses artwork supplied per slot to create an IconSet, which is
// what an iconset directory holds. A slot given artwork of the wrong size has
// it resized; a slot given none is filled from the largest image supplied,
// which also sets the largest icon written.
func NewIconSetFrom(images map[Slot]image.Image, interp InterpolationFunction) (*IconSet, error) {
	if len(images) == 0 {
		return nil, errors.New("cannot encode without an image")
	}
	slots := make([]Slot, 0, len(images))
	for slot, img := range images {
		if img == nil {
			return nil, fmt.Errorf("cannot encode nil image for %s", slot)
		}
		slots = append(slots, slot)
	}
	// The largest artwork stands in for the slots left empty. Ties are broken
	// by slot so the choice does not depend on map ordering.
	slices.SortFunc(slots, func(a, b Slot) int {
		if order := cmp.Compare(biggestSide(images[b]), biggestSide(images[a])); order != 0 {
			return order
		}
		if order := cmp.Compare(b.Points, a.Points); order != 0 {
			return order
		}
		return cmp.Compare(b.Scale, a.Scale)
	})
	return newIconSet(images, images[slots[0]], interp)
}

func newIconSet(images map[Slot]image.Image, source image.Image, interp InterpolationFunction) (*IconSet, error) {
	biggest := findNearestSize(source)
	if biggest == 0 {
		return nil, ErrImageTooSmall{image: source, need: 16}
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
			art := source
			if supplied, ok := images[osType.slot]; ok {
				art = supplied
			}
			icons[i] = &Icon{
				Type:  osType,
				Image: resizeSquare(art, osType.Size, interp),
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
	// ImageFormatARGB is run-length encoded channel planes that carry their
	// own alpha, behind an "ARGB" header.
	ImageFormatARGB
	// ImageFormatBitmap is one bit per pixel with a one bit mask.
	ImageFormatBitmap
	// ImageFormatIndexed is an index per pixel into a fixed colour table.
	ImageFormatIndexed
)

func (f ImageFormat) String() string {
	switch f {
	case ImageFormatPNG:
		return "PNG"
	case ImageFormatJPEG2000:
		return "JPEG 2000"
	case ImageFormatRGB:
		return "24-bit RGB"
	case ImageFormatARGB:
		return "ARGB"
	case ImageFormatBitmap:
		return "1-bit"
	case ImageFormatIndexed:
		return "indexed colour"
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
	// encodingBitmap holds one bit per pixel followed by its own mask.
	encodingBitmap
	// encodingIndexed4 and encodingIndexed8 hold an index per pixel into a
	// fixed colour table, with alpha in the mask half of OsType.mask.
	encodingIndexed4
	encodingIndexed8
)

// OsType is a 4 character identifier used to differentiate icon types.
type OsType struct {
	ID   string
	Size uint

	// enc is how this element stores its image data.
	enc encoding
	// mask is the element holding this type's alpha, for the encodings that
	// keep it apart from the colour.
	mask string
	// height is the pixel height, when the icon is not square.
	height uint
	// slot is the iconset slot this type fills, for the written types.
	slot Slot
	// emit marks the types the encoder writes. More types can be read than
	// are written.
	emit bool
}

func (t OsType) String() string {
	return fmt.Sprintf("%s %d", t.ID, t.Size)
}

var osTypes = []OsType{
	{ID: "ic10", Size: 1024, slot: Slot{512, 2}, emit: true},
	{ID: "ic14", Size: 512, slot: Slot{256, 2}, emit: true},
	{ID: "ic09", Size: 512, slot: Slot{512, 1}, emit: true},
	{ID: "ic13", Size: 256, slot: Slot{128, 2}, emit: true},
	{ID: "ic08", Size: 256, slot: Slot{256, 1}, emit: true},
	{ID: "ic07", Size: 128, slot: Slot{128, 1}, emit: true},
	{ID: "ic12", Size: 64, slot: Slot{32, 2}, emit: true},
	{ID: "ic11", Size: 32, slot: Slot{16, 2}, emit: true},

	{ID: "icp6", Size: 48},
	{ID: "icp5", Size: 32},
	{ID: "icp4", Size: 16},

	// Toolbar and sidebar icons, which hold ARGB or PNG.
	{ID: "SB24", Size: 48},
	{ID: "icsB", Size: 36},
	{ID: "ic05", Size: 32},
	{ID: "sb24", Size: 24},
	{ID: "icsb", Size: 18},
	{ID: "ic04", Size: 16},

	// The small sizes are written as colour and mask rather than PNG, which
	// is what Apple still emits for them: icp4 and icp5 hold PNG but do not
	// render from an app bundle.
	{ID: "it32", Size: 128, enc: encodingRGB, mask: "t8mk"},
	{ID: "ih32", Size: 48, enc: encodingRGB, mask: "h8mk"},

	// Icons from System 7 through Mac OS 8, an index per pixel against a
	// fixed table, with alpha in the mask half of the "#" element.
	{ID: "ich8", Size: 48, enc: encodingIndexed8, mask: "ich#"},
	{ID: "ich4", Size: 48, enc: encodingIndexed4, mask: "ich#"},
	{ID: "ich#", Size: 48, enc: encodingBitmap},
	{ID: "icl8", Size: 32, enc: encodingIndexed8, mask: "ICN#"},
	{ID: "icl4", Size: 32, enc: encodingIndexed4, mask: "ICN#"},
	{ID: "ICN#", Size: 32, enc: encodingBitmap},
	{ID: "ICON", Size: 32, enc: encodingBitmap},
	{ID: "ics8", Size: 16, enc: encodingIndexed8, mask: "ics#"},
	{ID: "ics4", Size: 16, enc: encodingIndexed4, mask: "ics#"},
	{ID: "ics#", Size: 16, enc: encodingBitmap},
	{ID: "icm8", Size: 16, height: 12, enc: encodingIndexed8, mask: "icm#"},
	{ID: "icm4", Size: 16, height: 12, enc: encodingIndexed4, mask: "icm#"},
	{ID: "icm#", Size: 16, height: 12, enc: encodingBitmap},
	{ID: "il32", Size: 32, enc: encodingRGB, mask: "l8mk", slot: Slot{32, 1}, emit: true},
	{ID: "is32", Size: 16, enc: encodingRGB, mask: "s8mk", slot: Slot{16, 1}, emit: true},
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
