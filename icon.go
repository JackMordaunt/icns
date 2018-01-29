package icns

import (
	"bytes"
	"image"
	"image/png"
	"io"
)

// OsType is a 4 character identifier used to differentiate icon types.
type OsType string

// Icon encodes an icns icon.
type Icon struct {
	Type  OsType
	Image image.Image

	header    [8]byte
	headerSet bool
	data      []byte
}

// WriteTo encodes the icon into wr.
func (i *Icon) WriteTo(wr io.Writer) (int64, error) {
	var written int64
	if err := i.encodePng(); err != nil {
		return written, err
	}
	size, err := i.writeHeader(wr)
	written += size
	if err != nil {
		return written, err
	}
	size, err = i.writeData(wr)
	written += size
	if err != nil {
		return written, err
	}
	return written, nil
}

func (i *Icon) encodePng() error {
	if len(i.data) > 0 {
		return nil
	}
	buf := bytes.NewBuffer(nil)
	if err := png.Encode(buf, i.Image); err != nil {
		return err
	}
	i.data = buf.Bytes()
	return nil
}

func (i *Icon) writeHeader(wr io.Writer) (int64, error) {
	if !i.headerSet {
		defer func() { i.headerSet = true }()
		i.header[0] = i.Type[0]
		i.header[1] = i.Type[1]
		i.header[2] = i.Type[2]
		i.header[3] = i.Type[3]
		length := uint32(len(i.data) + 8)
		writeUint32(i.header[4:8], length)
	}
	written, err := wr.Write(i.header[:8])
	return int64(written), err
}

func (i *Icon) writeData(wr io.Writer) (int64, error) {
	written, err := wr.Write(i.data)
	return int64(written), err
}
