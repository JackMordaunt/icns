package ico

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"slices"
)

var pngHeader = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// Decoder reads an ico file and decodes its icons on demand, so a caller
// after one size does not pay for the rest.
type Decoder struct {
	entries []Entry
}

// NewDecoder reads r and identifies the icons it holds without decoding any
// of their pixels.
func NewDecoder(r io.Reader) (*Decoder, error) {
	entries, err := directory(r)
	if err != nil {
		return nil, err
	}
	// Largest first, keeping file order between icons of equal size.
	slices.SortStableFunc(entries, func(a, b Entry) int {
		return cmp.Compare(b.Width*b.Height, a.Width*a.Height)
	})
	return &Decoder{entries: entries}, nil
}

// Icons returns the icons in the file, largest first.
func (d *Decoder) Icons() []Entry {
	return slices.Clone(d.entries)
}

// Entry is one icon in an ico file, before its pixels are decoded.
type Entry struct {
	// Width and Height are the dimensions the directory gives.
	Width, Height int
	// Format is how the icon's pixels are stored.
	Format Format

	data []byte
}

func (e Entry) String() string {
	return fmt.Sprintf("%dx%d (%s)", e.Width, e.Height, e.Format)
}

// Payload returns the bytes the file stores for the icon. For a PNG icon it
// is a whole image file; for a bitmap it is the header, pixels and mask.
//
// The bytes are not copied, and must not be modified.
func (e Entry) Payload() []byte {
	return e.data
}

// Decode decodes the icon's pixels. A PNG icon is passed to image.Decode, so
// it is read by whatever the program has registered.
func (e Entry) Decode() (image.Image, error) {
	if e.Format == FormatPNG {
		img, _, err := image.Decode(bytes.NewReader(e.data))
		if errors.Is(err, image.ErrFormat) {
			return nil, fmt.Errorf("%w: a %s icon, which no registered decoder reads", ErrUnsupportedFormat, e.Format)
		}
		if err != nil {
			return nil, fmt.Errorf("decoding %s icon: %w", e, err)
		}
		return img, nil
	}
	img, err := decodeBMP(e.data)
	if err != nil {
		return nil, fmt.Errorf("decoding %s icon: %w", e, err)
	}
	return img, nil
}

// directory splits an ico file into the icons it lists.
func directory(r io.Reader) ([]Entry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) < directorySize {
		return nil, ErrInvalidHeader
	}
	var (
		reserved = binary.LittleEndian.Uint16(data[0:2])
		kind     = binary.LittleEndian.Uint16(data[2:4])
		count    = int(binary.LittleEndian.Uint16(data[4:6]))
	)
	if reserved != 0 || kind != 1 {
		return nil, ErrInvalidHeader
	}
	if count == 0 {
		return nil, ErrNoIcons
	}
	if len(data) < directorySize+entrySize*count {
		return nil, fmt.Errorf("%w: the directory lists %d icons but is truncated", ErrMalformed, count)
	}
	entries := make([]Entry, 0, count)
	for i := range count {
		row := data[directorySize+entrySize*i:]
		var (
			width  = int(row[0])
			height = int(row[1])
			size   = int(binary.LittleEndian.Uint32(row[8:12]))
			offset = int(binary.LittleEndian.Uint32(row[12:16]))
		)
		// Zero stands for 256, which does not fit in a byte.
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		if size < 0 || offset < 0 || offset+size > len(data) || offset < directorySize {
			return nil, fmt.Errorf("%w: icon %d lies at %d for %d bytes, outside the file", ErrMalformed, i, offset, size)
		}
		payload := data[offset : offset+size]
		entry := Entry{Width: width, Height: height, data: payload}
		if bytes.HasPrefix(payload, pngHeader) {
			entry.Format = FormatPNG
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Decode returns the largest icon in the ico file that can be decoded. An
// icon in a format no registered decoder reads is passed over, so the result
// may be smaller than the largest present.
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

// DecodeAll extracts every icon in the file that can be decoded, largest
// first.
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
	return images, nil
}

// DecodeConfig returns the dimensions of the largest icon in the file.
func DecodeConfig(r io.Reader) (image.Config, error) {
	d, err := NewDecoder(r)
	if err != nil {
		return image.Config{}, err
	}
	largest := d.entries[0]
	return image.Config{
		Width:      largest.Width,
		Height:     largest.Height,
		ColorModel: color.NRGBAModel,
	}, nil
}

func init() {
	image.RegisterFormat("ico", Magic, Decode, DecodeConfig)
}
