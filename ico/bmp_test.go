package ico

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/color"
	"testing"
)

// colourTable builds a table of n entries where index 1 is c.
func colourTable(n int, c color.NRGBA) []byte {
	out := make([]byte, n*4)
	out[4], out[5], out[6] = c.B, c.G, c.R
	return out
}

// bmpIcon builds one bitmap icon of the given depth, every pixel the same
// colour, with an all opaque mask.
func bmpIcon(size, bits int, c color.NRGBA) []byte {
	var (
		stride     = ((size*bits + 31) / 32) * 4
		maskStride = ((size + 31) / 32) * 4
		palette    []byte
		fill       byte
	)
	switch bits {
	case 1:
		palette, fill = colourTable(2, c), 0xFF
	case 4:
		palette, fill = colourTable(16, c), 0x11
	case 8:
		palette, fill = colourTable(256, c), 0x01
	}
	out := make([]byte, 0, headerSize+len(palette)+stride*size+maskStride*size)
	out = binary.LittleEndian.AppendUint32(out, headerSize)
	out = binary.LittleEndian.AppendUint32(out, uint32(size))
	out = binary.LittleEndian.AppendUint32(out, uint32(size*2))
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, uint16(bits))
	out = binary.LittleEndian.AppendUint32(out, 0)
	out = binary.LittleEndian.AppendUint32(out, 0)
	out = binary.LittleEndian.AppendUint32(out, 0)
	out = binary.LittleEndian.AppendUint32(out, 0)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(palette)/4))
	out = binary.LittleEndian.AppendUint32(out, 0)
	out = append(out, palette...)

	rows := make([]byte, stride*size)
	switch bits {
	case 32:
		for y := range size {
			row := rows[y*stride:]
			for x := range size {
				row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = c.B, c.G, c.R, c.A
			}
		}
	case 24:
		for y := range size {
			row := rows[y*stride:]
			for x := range size {
				row[x*3], row[x*3+1], row[x*3+2] = c.B, c.G, c.R
			}
		}
	default:
		for i := range rows {
			rows[i] = fill
		}
	}
	out = append(out, rows...)
	return append(out, make([]byte, maskStride*size)...)
}

// icoFile wraps one icon in a directory.
func icoFile(size int, data []byte) []byte {
	out := make([]byte, 0, directorySize+entrySize+len(data))
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = append(out, byte(size), byte(size), 0, 0)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, 32)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(data)))
	out = binary.LittleEndian.AppendUint32(out, uint32(directorySize+entrySize))
	return append(out, data...)
}

// TestDecodeDepths covers the bit depths older icons are written at, which
// carry their own colour table and take their alpha from the mask.
func TestDecodeDepths(t *testing.T) {
	t.Parallel()
	for _, bits := range []int{1, 4, 8, 24, 32} {
		t.Run(map[int]string{1: "one bit", 4: "four bit", 8: "eight bit", 24: "twenty four bit", 32: "thirty two bit"}[bits], func(st *testing.T) {
			want := color.NRGBA{R: 0x30, G: 0x90, B: 0xC0, A: 0xFF}
			img, err := Decode(bytes.NewReader(icoFile(32, bmpIcon(32, bits, want))))
			if err != nil {
				st.Fatal(err)
			}
			if got := img.Bounds().Dx(); got != 32 {
				st.Fatalf("decoded a %dpx icon, want 32", got)
			}
			if got := centre(img); got != want {
				st.Fatalf("centre = %v, want %v", got, want)
			}
		})
	}
}

// TestDecodeMaskWhenAlphaIsEmpty covers the writers that left the alpha
// channel of a 32 bit icon at zero and meant the mask to be read.
func TestDecodeMaskWhenAlphaIsEmpty(t *testing.T) {
	t.Parallel()
	const size = 16
	data := bmpIcon(size, 32, color.NRGBA{R: 0x80, G: 0x40, B: 0x20, A: 0x00})
	img, err := Decode(bytes.NewReader(icoFile(size, data)))
	if err != nil {
		t.Fatal(err)
	}
	// The mask is all opaque, so the icon has to come back opaque rather
	// than invisible.
	if got := centre(img); got.A != 0xFF {
		t.Fatalf("centre = %v, want it opaque from the mask", got)
	}
}

func TestDecodeMalformed(t *testing.T) {
	t.Parallel()
	valid := bmpIcon(16, 32, color.NRGBA{A: 0xFF})
	tests := []struct {
		desc string
		data []byte
		want error
	}{
		{"empty", nil, ErrInvalidHeader},
		{"short", []byte{0, 0}, ErrInvalidHeader},
		{"reserved is not zero", []byte{1, 0, 1, 0, 1, 0}, ErrInvalidHeader},
		{"a cursor, not an icon", []byte{0, 0, 2, 0, 1, 0}, ErrInvalidHeader},
		{"no entries", []byte{0, 0, 1, 0, 0, 0}, ErrNoIcons},
		{"truncated directory", []byte{0, 0, 1, 0, 2, 0, 1, 2, 3}, ErrMalformed},
		{"icon lies outside the file", icoFile(16, valid)[:directorySize+entrySize+4], ErrMalformed},
		{"bitmap header is short", icoFile(16, []byte{1, 2, 3}), ErrMalformed},
		{"pixels do not fit", icoFile(16, bmpIcon(16, 32, color.NRGBA{})[:headerSize+16]), ErrMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			if _, err := Decode(bytes.NewReader(tt.data)); !errors.Is(err, tt.want) {
				st.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDecodeUnsupported(t *testing.T) {
	t.Parallel()
	// Sixteen bits per pixel, which this package does not read.
	odd := bmpIcon(16, 32, color.NRGBA{A: 0xFF})
	binary.LittleEndian.PutUint16(odd[14:16], 16)
	if _, err := Decode(bytes.NewReader(icoFile(16, odd))); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("error = %v, want ErrUnsupportedFormat", err)
	}
	// A compressed bitmap, which this package does not read either.
	compressed := bmpIcon(16, 32, color.NRGBA{A: 0xFF})
	binary.LittleEndian.PutUint32(compressed[16:20], 1)
	if _, err := Decode(bytes.NewReader(icoFile(16, compressed))); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("error = %v, want ErrUnsupportedFormat", err)
	}
}

// FuzzDecode checks that arbitrary input never panics or hangs the decoder.
func FuzzDecode(f *testing.F) {
	var valid bytes.Buffer
	if err := Encode(&valid, gradient(64)); err != nil {
		f.Fatal(err)
	}
	f.Add(valid.Bytes())
	f.Add([]byte{})
	f.Add([]byte{0, 0, 1, 0, 0, 0})
	for _, bits := range []int{1, 4, 8, 24, 32} {
		f.Add(icoFile(16, bmpIcon(16, bits, color.NRGBA{A: 0xFF})))
	}
	f.Add(icoFile(16, []byte{1, 2, 3}))
	f.Fuzz(func(t *testing.T, data []byte) {
		if d, err := NewDecoder(bytes.NewReader(data)); err == nil {
			for _, icon := range d.Icons() {
				icon.Decode()
			}
		}
		Decode(bytes.NewReader(data))
		DecodeAll(bytes.NewReader(data))
	})
}
