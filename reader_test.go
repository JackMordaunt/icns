package icns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// element builds one icns element: 4-byte type, 4-byte big-endian length of
// the whole element, then the payload.
func element(id string, payload []byte) []byte {
	out := make([]byte, 0, elementHeaderSize+len(payload))
	out = append(out, id...)
	out = binary.BigEndian.AppendUint32(out, uint32(elementHeaderSize+len(payload)))
	return append(out, payload...)
}

// file wraps elements in an icns header with a correct length.
func file(elements ...[]byte) []byte {
	return element("icns", bytes.Join(elements, nil))
}

func pngBytes(t testing.TB, side int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, side, side))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeMalformed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		data []byte
		want error
	}{
		{"empty", nil, ErrInvalidHeader},
		{"short", []byte("ic"), ErrInvalidHeader},
		{"wrong magic", element("ICNS", nil), ErrInvalidHeader},
		{"header only", file(), ErrNoIcons},
		{"declared size exceeds data", append([]byte("icns"), 0xff, 0xff, 0xff, 0xff), ErrMalformed},
		{"truncated element header", append([]byte("icns\x00\x00\x00\x0c"), 'i', 'c', '0', '7'), ErrMalformed},
		{"element size below header", file(append([]byte("ic07"), 0, 0, 0, 4)), ErrMalformed},
		{"element overruns file", file(append([]byte("ic07"), 0, 0, 1, 0)), ErrMalformed},
		{"zero-length TOC loops forever without a check", file(append([]byte("TOC "), 0, 0, 0, 0)), ErrMalformed},
		{"unknown elements only", file(element("TOC ", []byte{1, 2, 3, 4}), element("icnV", []byte{0, 0, 0, 0})), ErrNoIcons},
		{"empty icon payload", file(element("ic07", nil)), ErrNoIcons},
		{"only jpeg2000", file(element("ic07", jpeg2000header)), ErrUnsupportedFormat},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			_, err := Decode(bytes.NewReader(tt.data))
			if !errors.Is(err, tt.want) {
				st.Fatalf("Decode error = %v, want %v", err, tt.want)
			}
			if _, err := Probe(bytes.NewReader(tt.data)); err == nil && !errors.Is(tt.want, ErrUnsupportedFormat) {
				st.Fatalf("Probe accepted data that Decode rejected with %v", tt.want)
			}
		})
	}
}

func TestDecodeSkipsNonIconElements(t *testing.T) {
	t.Parallel()
	data := file(
		element("TOC ", []byte("ic07\x00\x00\x00\x10")),
		element("icnV", []byte{0x40, 0x00, 0x00, 0x00}),
		element("name", []byte("icon")),
		element("ic07", pngBytes(t, 128)),
		element("ic11", pngBytes(t, 32)),
	)
	desc, err := Probe(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(desc) != 2 || desc[0].ID != "ic07" || desc[1].ID != "ic11" {
		t.Fatalf("Probe = %v, want ic07 and ic11", desc)
	}
	// Trailing bytes beyond the declared file size are tolerated.
	img, err := Decode(bytes.NewReader(append(data, "junk"...)))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != 128 {
		t.Fatalf("Decode returned a %dpx icon, want 128", got)
	}
}

func TestDecodeFallsBackPastJPEG2000(t *testing.T) {
	t.Parallel()
	data := file(
		element("ic10", jpeg2000header), // Largest, but undecodable.
		element("ic07", pngBytes(t, 128)),
	)
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != 128 {
		t.Fatalf("Decode returned a %dpx icon, want the 128px PNG", got)
	}
	all, err := DecodeAll(bytes.NewReader(data))
	if err != nil || len(all) != 1 {
		t.Fatalf("DecodeAll = %d images, %v; want 1, nil", len(all), err)
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
	f.Add(file())
	f.Add(file(append([]byte("TOC "), 0, 0, 0, 0)))
	f.Add(file(append([]byte("ic07"), 0, 0, 0, 4)))
	f.Add(append([]byte("icns"), 0xff, 0xff, 0xff, 0xff))
	f.Fuzz(func(t *testing.T, data []byte) {
		Probe(bytes.NewReader(data))
		Decode(bytes.NewReader(data))
		DecodeAll(bytes.NewReader(data))
	})
}

// gradient returns a side by side image whose colour and alpha vary with
// position, so an encode/decode roundtrip cannot pass by accident.
func gradient(side int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / side),
				G: uint8(y * 255 / side),
				B: uint8((x + y) * 255 / (2 * side)),
				A: uint8(255 - y*255/(2*side)),
			})
		}
	}
	return img
}
