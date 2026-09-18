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
	if i.Type.enc == encodingRGB {
		planes, mask := splitPlanes(i.Image, int(i.Type.Size))
		i.elements = []element{
			{id: i.Type.ID, payload: packRLE(planes)},
			{id: i.Type.mask, payload: mask},
		}
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
}

// WriteTo writes the ICNS file to wr.
func (s *IconSet) WriteTo(wr io.Writer) (int64, error) {
	var elements []element
	for _, icon := range s.Icons {
		if icon == nil {
			continue
		}
		if err := icon.encode(); err != nil {
			return 0, err
		}
		elements = append(elements, icon.elements...)
	}
	body := bytes.NewBuffer(nil)
	// The table of contents comes first and covers everything after it, so a
	// reader can index the file without walking every element.
	if _, err := writeElement(body, tableOfContents(elements)); err != nil {
		return 0, err
	}
	for _, el := range elements {
		if _, err := writeElement(body, el); err != nil {
			return 0, err
		}
	}
	// The file is itself an element enclosing all the others.
	return writeElement(wr, element{id: "icns", payload: body.Bytes()})
}

// tableOfContents lists each element's type and total size, in order.
func tableOfContents(elements []element) element {
	toc := element{
		id:      "TOC ",
		payload: make([]byte, 0, len(elements)*elementHeaderSize),
	}
	for _, el := range elements {
		toc.payload = append(toc.payload, el.id...)
		toc.payload = binary.BigEndian.AppendUint32(toc.payload, uint32(elementHeaderSize+len(el.payload)))
	}
	return toc
}
