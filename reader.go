package icns

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"slices"
)

var (
	jpeg2000header = []byte{0x00, 0x00, 0x00, 0x0c, 0x6a, 0x50, 0x20, 0x20}
	argbHeader     = []byte("ARGB")
)

// Decoder reads an icns file and decodes its icons on demand, so a caller
// after one size does not pay for the rest.
type Decoder struct {
	entries []Entry
}

// NewDecoder reads r and identifies the icons it holds without decoding any
// of their pixels.
func NewDecoder(r io.Reader) (*Decoder, error) {
	entries, err := decode(r)
	if err != nil {
		return nil, err
	}
	// Largest first, and at a given size the one carrying the most colour,
	// since the older files hold several depths of the same icon.
	slices.SortStableFunc(entries, func(a, b Entry) int {
		if order := cmp.Compare(b.Size, a.Size); order != 0 {
			return order
		}
		return cmp.Compare(b.colours(), a.colours())
	})
	return &Decoder{entries: entries}, nil
}

// Icons returns the icons in the file, largest first.
func (d *Decoder) Icons() []Entry {
	return slices.Clone(d.entries)
}

// Entry is one icon in an icns file, before its pixels are decoded.
type Entry struct {
	IconDescription

	data []byte
	// mask holds the alpha channel for ImageFormatRGB icons, when the file
	// carries the matching mask element.
	mask []byte
}

// Decode decodes the icon's pixels. The formats icns defines itself are
// decoded here; an element holding a whole image file is passed to
// image.Decode, so it is read by whatever the program has registered.
func (e Entry) Decode() (image.Image, error) {
	switch e.ImageFormat {
	case ImageFormatRGB:
		data := e.data
		// it32 is the one colour element that prefixes its planes with four
		// zero bytes.
		if e.ID == "it32" && len(data) >= 4 && binary.BigEndian.Uint32(data[:4]) == 0 {
			data = data[4:]
		}
		img, err := decodeRGB(data, e.mask, int(e.Size))
		if err != nil {
			return nil, fmt.Errorf("decoding icon %s %s: %w", e.OsType, e.ImageFormat, err)
		}
		return img, nil
	case ImageFormatARGB:
		img, err := decodeARGB(e.data[len(argbHeader):], int(e.Size))
		if err != nil {
			return nil, fmt.Errorf("decoding icon %s %s: %w", e.OsType, e.ImageFormat, err)
		}
		return img, nil
	case ImageFormatBitmap, ImageFormatIndexed:
		img, err := e.indexed()
		if err != nil {
			return nil, fmt.Errorf("decoding icon %s %s: %w", e.OsType, e.ImageFormat, err)
		}
		return img, nil
	default:
		img, _, err := image.Decode(bytes.NewReader(e.data))
		if errors.Is(err, image.ErrFormat) {
			return nil, fmt.Errorf("%w: icon %s is %s, which no registered decoder reads", ErrUnsupportedFormat, e.OsType, e.ImageFormat)
		}
		if err != nil {
			return nil, fmt.Errorf("decoding icon %s %s: %w", e.OsType, e.ImageFormat, err)
		}
		return img, nil
	}
}

// indexed decodes the icon types that hold an index per pixel, finding their
// alpha in the mask half of a companion element or of their own payload.
func (e Entry) indexed() (image.Image, error) {
	var (
		width  = int(e.Size)
		height = int(e.Size)
		bits   = 1
	)
	if e.height > 0 {
		height = int(e.height)
	}
	switch e.enc {
	case encodingIndexed4:
		bits = 4
	case encodingIndexed8:
		bits = 8
	}
	// A "#" element holds its bitmap first and its mask second, whether it
	// is the icon itself or the companion an indexed icon points at.
	var (
		plane = width * height / 8
		mask  = e.mask
	)
	if e.enc == encodingBitmap {
		mask = e.data
	}
	if len(mask) >= plane*2 {
		mask = mask[plane : plane*2]
	} else {
		mask = nil
	}
	return decodeIndexed(e.data, mask, width, height, bits)
}

// colours ranks how much colour an icon carries, so the richest at a size
// comes first.
func (e Entry) colours() int {
	switch e.enc {
	case encodingBitmap:
		return 0
	case encodingIndexed4:
		return 1
	case encodingIndexed8:
		return 2
	default:
		return 3
	}
}

// Payload returns the bytes the file stores for the icon. For PNG and JPEG
// 2000 icons it is a complete image file; for the colour and mask types it is
// the run-length encoded colour planes, without the mask that holds their
// alpha.
//
// The bytes are not copied, and must not be modified.
func (e Entry) Payload() []byte {
	return e.data
}

// Decode returns the largest icon in the icns file that can be decoded,
// ignoring all other sizes. An icon in a format no registered decoder reads
// is passed over, so the result may be smaller than the largest present.
func Decode(r io.Reader) (image.Image, error) {
	d, err := NewDecoder(r)
	if err != nil {
		return nil, err
	}
	for _, icon := range d.entries {
		img, err := icon.Decode()
		if errors.Is(err, ErrUnsupportedFormat) {
			continue
		}
		return img, err
	}
	return nil, fmt.Errorf("%w: no icon is in a format a registered decoder reads", ErrUnsupportedFormat)
}

// DecodeAll extracts every icon resolution present in the icns data that can
// be decoded. An icon in a format no registered decoder reads is ignored.
func DecodeAll(r io.Reader) (images []image.Image, err error) {
	d, err := NewDecoder(r)
	if err != nil {
		return nil, err
	}
	for _, icon := range d.entries {
		img, err := icon.Decode()
		if errors.Is(err, ErrUnsupportedFormat) {
			continue
		}
		if err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("%w: no icon is in a format a registered decoder reads", ErrUnsupportedFormat)
	}
	// An element may hold an image of a size other than the one its type
	// names, so order by what was actually decoded.
	slices.SortStableFunc(images, func(a, b image.Image) int {
		left, right := a.Bounds().Size(), b.Bounds().Size()
		return cmp.Compare(right.X+right.Y, left.X+left.Y)
	})
	return images, nil
}

// Probe extracts descriptions of the icons in the icns, largest first.
func Probe(r io.Reader) (desc []IconDescription, _ error) {
	d, err := NewDecoder(r)
	if err != nil {
		return nil, err
	}
	for _, icon := range d.entries {
		desc = append(desc, icon.IconDescription)
	}
	return desc, nil
}

// elementHeaderSize is the size of the type and length fields that begin
// every element, the file header included.
const elementHeaderSize = 8

// element is one type and payload pair from the file.
type element struct {
	id      string
	payload []byte
}

// decode identifies the icons in the icns without decoding the image data.
//
// An icns file is a sequence of elements, each a 4-byte type followed by a
// 4-byte big-endian length that counts the whole element, header included.
// The file itself is one such element of type "icns" enclosing the rest.
// Every length is checked against the data present, and input that disagrees
// is reported as ErrMalformed.
func decode(r io.Reader) (icons []Entry, err error) {
	elements, err := elementsOf(r)
	if err != nil {
		return nil, err
	}
	// Masks are separate elements that may appear either side of the icon
	// they belong to, so the payloads are indexed before they are paired up.
	payloads := make(map[string][]byte, len(elements))
	for _, el := range elements {
		payloads[el.id] = el.payload
	}
	for _, el := range elements {
		osType, ok := getTypeFromID(el.id)
		if !ok || len(el.payload) == 0 {
			// Elements this package does not read: the table of contents,
			// version and name records, masks, and icon types it cannot
			// decode.
			continue
		}
		icon := Entry{
			IconDescription: IconDescription{OsType: osType},
			data:            el.payload,
		}
		// Several types carry more than one format, so the payload decides
		// wherever it says what it holds.
		switch {
		case osType.enc == encodingRGB:
			icon.ImageFormat = ImageFormatRGB
			icon.mask = payloads[osType.mask]
		case osType.enc == encodingBitmap:
			icon.ImageFormat = ImageFormatBitmap
		case osType.enc == encodingIndexed4, osType.enc == encodingIndexed8:
			icon.ImageFormat = ImageFormatIndexed
			icon.mask = payloads[osType.mask]
		case bytes.HasPrefix(el.payload, argbHeader):
			icon.ImageFormat = ImageFormatARGB
		case bytes.HasPrefix(el.payload, jpeg2000header):
			icon.ImageFormat = ImageFormatJPEG2000
		}
		icons = append(icons, icon)
	}
	if len(icons) == 0 {
		return nil, ErrNoIcons
	}
	return icons, nil
}

// elementsOf splits an icns file into its elements.
func elementsOf(r io.Reader) ([]element, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) < elementHeaderSize || string(data[0:4]) != "icns" {
		return nil, ErrInvalidHeader
	}
	fileSize := int(binary.BigEndian.Uint32(data[4:8]))
	if fileSize > len(data) {
		return nil, fmt.Errorf("%w: header declares %d bytes but only %d are present", ErrMalformed, fileSize, len(data))
	}
	data = data[:fileSize]
	var elements []element
	for offset := elementHeaderSize; offset < len(data); {
		if len(data)-offset < elementHeaderSize {
			return nil, fmt.Errorf("%w: truncated element header at offset %d", ErrMalformed, offset)
		}
		var (
			id   = string(data[offset : offset+4])
			size = int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		)
		if size < elementHeaderSize || size > len(data)-offset {
			return nil, fmt.Errorf("%w: element %q at offset %d declares %d bytes", ErrMalformed, id, offset, size)
		}
		elements = append(elements, element{
			id:      id,
			payload: data[offset+elementHeaderSize : offset+size],
		})
		offset += size
	}
	return elements, nil
}

func isOsType(ID string) bool {
	_, ok := getTypeFromID(ID)
	return ok
}

func init() {
	image.RegisterFormat("icns", "icns", Decode, nil)
}
