package icns

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"sort"
)

var jpeg2000header = []byte{0x00, 0x00, 0x00, 0x0c, 0x6a, 0x50, 0x20, 0x20}

// Decode returns the largest decodable icon in the icns file, ignoring all
// other sizes. JPEG 2000 icons are skipped due to lack of image decoding
// support, so the result may be smaller than the largest icon present.
func Decode(r io.Reader) (image.Image, error) {
	icons, err := decode(r)
	if err != nil {
		return nil, err
	}
	sort.Slice(icons, func(ii, jj int) bool {
		return icons[ii].OsType.Size > icons[jj].OsType.Size
	})
	for _, icon := range icons {
		if icon.ImageFormat == ImageFormatJPEG2000 {
			continue
		}
		img, _, err := image.Decode(icon.r)
		if err != nil {
			return nil, fmt.Errorf("decoding icon %s %s: %w", icon.OsType, icon.ImageFormat, err)
		}
		return img, nil
	}
	return nil, fmt.Errorf("%w: only %s icons present", ErrUnsupportedFormat, ImageFormatJPEG2000)
}

// DecodeAll extracts all icon resolutions present in the icns data that
// contain PNG data. JPEG 2000 is ignored due to lack of image decoding
// support.
func DecodeAll(r io.Reader) (images []image.Image, err error) {
	icons, err := decode(r)
	if err != nil {
		return nil, err
	}
	for _, icon := range icons {
		if icon.IconDescription.ImageFormat == ImageFormatJPEG2000 {
			continue
		}
		img, _, err := image.Decode(icon.r)
		if err != nil {
			return nil, fmt.Errorf("decoding icon %s %s: %w", icon.OsType, icon.ImageFormat, err)
		}
		images = append(images, img)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("%w: only %s icons present", ErrUnsupportedFormat, ImageFormatJPEG2000)
	}
	sort.Slice(images, func(ii, jj int) bool {
		var (
			left  = images[ii].Bounds().Size()
			right = images[jj].Bounds().Size()
		)
		return (left.X + left.Y) > (right.X + right.Y)
	})
	return images, nil
}

// Probe extracts descriptions of the icons in the icns.
func Probe(r io.Reader) (desc []IconDescription, _ error) {
	icons, err := decode(r)
	if err != nil {
		return nil, err
	}
	for _, icon := range icons {
		desc = append(desc, icon.IconDescription)
	}
	return desc, nil
}

// elementHeaderSize is the size of the type and length fields that begin
// every element, the file header included.
const elementHeaderSize = 8

// decode identifies the icons in the icns without decoding the image data.
//
// An icns file is a sequence of elements, each a 4-byte type followed by a
// 4-byte big-endian length that counts the whole element, header included.
// The file itself is one such element of type "icns" enclosing the rest.
// Every length is validated against the data actually present so malformed
// or truncated input yields an error rather than a panic or an endless loop.
func decode(r io.Reader) (icons []iconReader, err error) {
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
		payload := data[offset+elementHeaderSize : offset+size]
		offset += size
		// Elements other than icons ("TOC ", "icnV", "name", "info", ...)
		// and icons of legacy types carry nothing we can decode; skip them.
		if !isOsType(id) || len(payload) == 0 {
			continue
		}
		ir := iconReader{
			IconDescription: IconDescription{
				OsType: osTypeFromID(id),
			},
			r: bytes.NewReader(payload),
		}
		if bytes.HasPrefix(payload, jpeg2000header) {
			ir.ImageFormat = ImageFormatJPEG2000
		}
		icons = append(icons, ir)
	}
	if len(icons) == 0 {
		return nil, ErrNoIcons
	}
	return icons, nil
}

type iconReader struct {
	IconDescription
	r io.Reader
}

func isOsType(ID string) bool {
	_, ok := getTypeFromID(ID)
	return ok
}

func init() {
	image.RegisterFormat("icns", "icns", Decode, nil)
}
