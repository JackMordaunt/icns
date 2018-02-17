package icns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"io/ioutil"
)

// Decode finds the largest icon listed in the icns file and returns it,
// ignoring all other sizes. The format returned will be whatever the icon data
// is, typically jpeg or png.
func Decode(r io.Reader) (image.Image, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	icnsHeader := data[0:4]
	if string(icnsHeader) != "icns" {
		return nil, errors.New("invalid header for icns file")
	}
	fileSize := binary.BigEndian.Uint32(data[4:8])
	icons := []iconReader{}
	read := uint32(8)
	for read < fileSize {
		next := data[read : read+4]
		read += 4
		switch string(next) {
		case "TOC ":
			read += 4
			continue
		case "icnV":
			read += 4
			continue
		}
		if isOsType(string(next)) {
			iconSize := binary.BigEndian.Uint32(data[read : read+4])
			read += 4
			iconData := data[read : read+iconSize]
			read += iconSize
			icons = append(icons, iconReader{
				OsType: osTypeFromID(string(next)),
				r:      bytes.NewBuffer(iconData),
			})
		}
	}
	var biggest iconReader
	for _, icon := range icons {
		if icon.Size > biggest.Size {
			biggest = icon
		}
	}
	img, _, err := image.Decode(biggest.r)
	if err != nil {
		return nil, err
	}
	return img, nil
}

type iconReader struct {
	OsType
	r io.Reader
}

func isOsType(ID string) bool {
	_, ok := getTypeFromID(ID)
	return ok
}

func init() {
	image.RegisterFormat("icns", "icns", Decode, nil)
}
