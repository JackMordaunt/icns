package icns

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"io"
)

// Icon encodes an icns icon.
type Icon struct {
	Type  OsType
	Image image.Image

	elements []element
}

// WriteTo encodes the icon into wr.
func (i *Icon) WriteTo(wr io.Writer) (int64, error) {
	if err := i.encode(); err != nil {
		return 0, err
	}
	var written int64
	for _, el := range i.elements {
		n, err := writeElement(wr, el)
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// encode builds the elements this icon occupies in the file.
func (i *Icon) encode() error {
	if len(i.elements) > 0 {
		return nil
	}
	data, err := encodeImage(i.Image)
	if err != nil {
		return err
	}
	i.elements = []element{{id: i.Type.ID, payload: data}}
	return nil
}

func encodeImage(img image.Image) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeElement writes one element: its 4-byte type, its length counting the
// header, then its payload.
func writeElement(wr io.Writer, el element) (int64, error) {
	var header [elementHeaderSize]byte
	copy(header[:4], el.id)
	binary.BigEndian.PutUint32(header[4:], uint32(elementHeaderSize+len(el.payload)))
	n, err := wr.Write(header[:])
	written := int64(n)
	if err != nil {
		return written, err
	}
	n, err = wr.Write(el.payload)
	return written + int64(n), err
}

// IconSet encodes a set of icons into an ICNS file.
type IconSet struct {
	Icons []*Icon

	data []byte
}

// WriteTo writes the ICNS file to wr.
func (s *IconSet) WriteTo(wr io.Writer) (int64, error) {
	if err := s.encodeIcons(); err != nil {
		return 0, err
	}
	// The file is itself an element enclosing all the others.
	return writeElement(wr, element{id: "icns", payload: s.data})
}

func (s *IconSet) encodeIcons() error {
	if len(s.data) > 0 {
		return nil
	}
	buf := bytes.NewBuffer(nil)
	for _, icon := range s.Icons {
		if icon == nil {
			continue
		}
		if _, err := icon.WriteTo(buf); err != nil {
			return err
		}
	}
	s.data = buf.Bytes()
	return nil
}
