package icns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"reflect"
	"testing"
)

// encodeElement builds one icns element: 4-byte type, 4-byte big-endian
// length of the whole element, then the payload.
func encodeElement(id string, payload []byte) []byte {
	out := make([]byte, 0, elementHeaderSize+len(payload))
	out = append(out, id...)
	out = binary.BigEndian.AppendUint32(out, uint32(elementHeaderSize+len(payload)))
	return append(out, payload...)
}

// file wraps elements in an icns header with a correct length.
func file(elements ...[]byte) []byte {
	return encodeElement("icns", bytes.Join(elements, nil))
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
		{"wrong magic", encodeElement("ICNS", nil), ErrInvalidHeader},
		{"header only", file(), ErrNoIcons},
		{"declared size exceeds data", append([]byte("icns"), 0xff, 0xff, 0xff, 0xff), ErrMalformed},
		{"truncated element header", append([]byte("icns\x00\x00\x00\x0c"), 'i', 'c', '0', '7'), ErrMalformed},
		{"element size below header", file(append([]byte("ic07"), 0, 0, 0, 4)), ErrMalformed},
		{"element overruns file", file(append([]byte("ic07"), 0, 0, 1, 0)), ErrMalformed},
		{"zero-length TOC loops forever without a check", file(append([]byte("TOC "), 0, 0, 0, 0)), ErrMalformed},
		{"unknown elements only", file(encodeElement("TOC ", []byte{1, 2, 3, 4}), encodeElement("icnV", []byte{0, 0, 0, 0})), ErrNoIcons},
		{"empty icon payload", file(encodeElement("ic07", nil)), ErrNoIcons},
		{"only jpeg2000", file(encodeElement("ic07", jpeg2000header)), ErrUnsupportedFormat},
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
		encodeElement("TOC ", []byte("ic07\x00\x00\x00\x10")),
		encodeElement("icnV", []byte{0x40, 0x00, 0x00, 0x00}),
		encodeElement("name", []byte("icon")),
		encodeElement("ic07", pngBytes(t, 128)),
		encodeElement("ic11", pngBytes(t, 32)),
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
		encodeElement("ic10", jpeg2000header), // Largest, but undecodable.
		encodeElement("ic07", pngBytes(t, 128)),
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

func TestDecoder(t *testing.T) {
	t.Parallel()
	png128 := pngBytes(t, 128)
	data := file(
		encodeElement("ic11", pngBytes(t, 32)),
		encodeElement("ic10", jpeg2000header),
		encodeElement("ic07", png128),
	)
	d, err := NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	icons := d.Icons()
	var got []string
	for _, icon := range icons {
		got = append(got, icon.ID)
	}
	if want := []string{"ic10", "ic07", "ic11"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("icons = %v, want %v largest first", got, want)
	}

	// An icon this package cannot decode still hands over its bytes, so a
	// caller can bring its own decoder.
	if _, err := icons[0].Decode(); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("decoding the JPEG 2000 icon = %v, want ErrUnsupportedFormat", err)
	}
	if !bytes.Equal(icons[0].Payload(), jpeg2000header) {
		t.Error("the JPEG 2000 icon's payload is not the stored bytes")
	}

	img, err := icons[1].Decode()
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != 128 {
		t.Errorf("decoded a %dpx icon, want 128", got)
	}
	if !bytes.Equal(icons[1].Payload(), png128) {
		t.Error("the PNG icon's payload is not the stored file")
	}

	// The returned slice is the caller's to reorder.
	icons[0] = Entry{}
	if again := d.Icons(); again[0].ID != "ic10" {
		t.Errorf("Icons was affected by a change to an earlier result: %v", again[0].ID)
	}
}

// TestDecodeUsesRegisteredFormats checks that an element holding a whole
// image file is handed to whatever decoder the program registered. That is
// how a JPEG 2000 icon is read without this package depending on a codec for
// it: the caller imports one, and these elements start decoding.
func TestDecodeUsesRegisteredFormats(t *testing.T) {
	// Registration is global and cannot be undone, so this uses a magic
	// nothing else does and does not run in parallel.
	const magic = "notarealformat"
	want := color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xFF}
	image.RegisterFormat("notareal", magic,
		func(r io.Reader) (image.Image, error) { return solid(64, want), nil },
		func(r io.Reader) (image.Config, error) {
			return image.Config{Width: 64, Height: 64, ColorModel: color.NRGBAModel}, nil
		})

	data := file(encodeElement("ic12", []byte(magic+" and then the pixels")))
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("an element in a registered format did not decode: %v", err)
	}
	if got := centre(img); got != want {
		t.Fatalf("decoded %v, want %v from the registered decoder", got, want)
	}
}

// TestDecodeWithoutARegisteredFormat checks the other side of that: an icon
// nothing can read is reported as unsupported rather than as corrupt, which
// is what lets the callers above skip past it to a size they can read.
func TestDecodeWithoutARegisteredFormat(t *testing.T) {
	t.Parallel()
	data := file(encodeElement("ic07", append(jpeg2000header, 1, 2, 3, 4)))
	_, err := Decode(bytes.NewReader(data))
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("error = %v, want ErrUnsupportedFormat", err)
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
	// Legacy colour and mask elements, which run the RLE decoder.
	rgb, mask, _ := legacyIcon(16)
	f.Add(file(encodeElement("is32", rgb), encodeElement("s8mk", mask)))
	f.Add(file(encodeElement("is32", rgb)))
	f.Add(file(encodeElement("it32", []byte{0, 0, 0, 0, 0xFF, 0x01})))
	f.Add(file(encodeElement("il32", []byte{0xFF}), encodeElement("l8mk", mask)))
	// ARGB, which runs the same decoder over four planes.
	argb, _ := argbElement(16)
	f.Add(file(encodeElement("ic04", argb)))
	f.Add(file(encodeElement("ic04", []byte("ARGB"))))
	// Indexed icons, whose mask lives in a companion element.
	f.Add(file(encodeElement("icl8", make([]byte, 32*32)), encodeElement("ICN#", bitmapMask(32, 32))))
	f.Add(file(encodeElement("icl4", make([]byte, 32*32/2))))
	f.Add(file(encodeElement("ICN#", bitmapMask(32, 32))))
	f.Add(file(encodeElement("icm8", make([]byte, 16*12))))
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
