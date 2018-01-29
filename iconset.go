package icns

import (
	"bytes"
	"io"
)

// IconSet encodes a set of icons into an ICNS file.
type IconSet struct {
	Icons []*Icon

	header    [8]byte
	headerSet bool
	data      []byte
}

// WriteTo writes the ICNS file to wr.
func (s *IconSet) WriteTo(wr io.Writer) (int64, error) {
	var written int64
	if err := s.encodeIcons(); err != nil {
		return written, err
	}
	size, err := s.writeHeader(wr)
	written += size
	if err != nil {
		return written, err
	}
	size, err = s.writeData(wr)
	written += size
	if err != nil {
		return written, err
	}
	return written, nil
}

func (s *IconSet) encodeIcons() error {
	if len(s.data) > 0 {
		return nil
	}
	buf := bytes.NewBuffer(nil)
	for _, icon := range s.Icons {
		if _, err := icon.WriteTo(buf); err != nil {
			return err
		}
	}
	s.data = buf.Bytes()
	return nil
}

func (s *IconSet) writeHeader(wr io.Writer) (int64, error) {
	if !s.headerSet {
		defer func() { s.headerSet = true }()
		s.header[0] = 'i'
		s.header[1] = 'c'
		s.header[2] = 'n'
		s.header[3] = 's'
		length := uint32(len(s.data) + 8)
		writeUint32(s.header[4:8], length)
	}
	written, err := wr.Write(s.header[:8])
	return int64(written), err
}

func (s *IconSet) writeData(wr io.Writer) (int64, error) {
	written, err := wr.Write(s.data)
	return int64(written), err
}
