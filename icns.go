package icns

import (
	"fmt"
)

// Magic is the four bytes an icns file begins with, which are also the type
// of the element enclosing every other.
const Magic = "icns"

var sizes = []uint{
	1024,
	512,
	256,
	128,
	64,
	32,
	16,
}

// smallest is the last entry of sizes: the size below which the encoder
// writes nothing, and the smallest artwork it accepts.
const smallest = 16

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
	// encodingCompressed holds a whole image file, PNG or JPEG 2000. Nothing
	// names it, because it is the zero value every type that stores a whole
	// file takes by leaving the field out; removing it would renumber the
	// rest and make those types something else.
	//lint:ignore U1000 the zero value, taken by omission
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
