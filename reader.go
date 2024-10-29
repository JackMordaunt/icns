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

// Decode finds the largest icon listed in the icns file and returns it,
// ignoring all other sizes. The format returned will be PNG. JPEG 2000
// icons are ignored due to lack of image decoding support.
func Decode(r io.Reader) (image.Image, error) {
	icons, err := decode(r)
	if err != nil {
		return nil, err
	}
	sort.Slice(icons, func(ii, jj int) bool {
		return icons[ii].OsType.Size > icons[jj].OsType.Size
	})
	icon := icons[0]
	if icon.IconDescription.ImageFormat == ImageFormatJPEG2000 {
		return nil, fmt.Errorf("decoding largest image (icon %s %s): unsupported format", icon.OsType, icon.ImageFormat)
	}
	img, _, err := image.Decode(icon.r)
	if err != nil {
		return nil, fmt.Errorf("decoding largest image (icon %s %s): %w", icon.OsType, icon.ImageFormat, err)
	}
	return img, nil
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
		return nil, fmt.Errorf("no supported icons found")
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

// decode identifies the icons in the icns (without decoding the image data).
func decode(r io.Reader) (icons []iconReader, err error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var (
		header   = data[0:4]
		fileSize = binary.BigEndian.Uint32(data[4:8])
		read     = uint32(8)
	)
	if string(header) != "icns" {
		return nil, fmt.Errorf("invalid header for icns file")
	}
	for read < fileSize {
		next := data[read : read+4]
		read += 4
		switch string(next) {
		case "TOC ":
			tocSize := binary.BigEndian.Uint32(data[read : read+4])
			read += tocSize - 4 // size includes header and size fields
			continue
		case "icnV":
			read += 4
			continue
		}
		dataSize := binary.BigEndian.Uint32(data[read : read+4])
		read += 4
		if dataSize == 0 {
			continue // no content, we're not interested
		}
		iconData := data[read : read+dataSize-8]
		read += dataSize - 8 // size includes header and size fields
		if isOsType(string(next)) {
			ir := iconReader{
				IconDescription: IconDescription{
					OsType: osTypeFromID(string(next)),
				},
				r: bytes.NewBuffer(iconData),
			}
			if bytes.Equal(iconData[:8], jpeg2000header) {
				ir.ImageFormat = ImageFormatJPEG2000
			}
			icons = append(icons, ir)
		}
	}
	if len(icons) == 0 {
		return nil, fmt.Errorf("no icons found")
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
