package icns

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"
)

// TestPalettes pins the anchors of the two fixed colour tables. The cube is
// indexed 36R + 6G + B over the six levels, descending from white.
func TestPalettes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc  string
		index int
		want  color.NRGBA
	}{
		{"white leads the table", 0, color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}},
		{"blue steps down first", 1, color.NRGBA{0xFF, 0xFF, 0xCC, 0xFF}},
		{"blue bottoms out", 5, color.NRGBA{0xFF, 0xFF, 0x00, 0xFF}},
		{"green steps after blue wraps", 6, color.NRGBA{0xFF, 0xCC, 0xFF, 0xFF}},
		{"red steps after green wraps", 36, color.NRGBA{0xCC, 0xFF, 0xFF, 0xFF}},
		{"the cube ends before black", 214, color.NRGBA{0x00, 0x00, 0x33, 0xFF}},
		{"the red ramp follows the cube", 215, color.NRGBA{0xEE, 0x00, 0x00, 0xFF}},
		{"the red ramp ends", 224, color.NRGBA{0x11, 0x00, 0x00, 0xFF}},
		{"then green", 225, color.NRGBA{0x00, 0xEE, 0x00, 0xFF}},
		{"then blue", 235, color.NRGBA{0x00, 0x00, 0xEE, 0xFF}},
		{"then grey", 245, color.NRGBA{0xEE, 0xEE, 0xEE, 0xFF}},
		{"the grey ramp ends", 254, color.NRGBA{0x11, 0x11, 0x11, 0xFF}},
		{"black is last", 255, color.NRGBA{0x00, 0x00, 0x00, 0xFF}},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			if got := macPalette8[tt.index]; got != tt.want {
				st.Fatalf("index %d = %v, want %v", tt.index, got, tt.want)
			}
		})
	}
	if got := macPalette4[0]; got != (color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Errorf("4-bit index 0 = %v, want white", got)
	}
	if got := macPalette4[15]; got != (color.NRGBA{0x00, 0x00, 0x00, 0xFF}) {
		t.Errorf("4-bit index 15 = %v, want black", got)
	}
	// Every entry of the 256 table must be distinct, which the construction
	// only manages because black is held back from the cube.
	seen := map[color.NRGBA]int{}
	for i, c := range macPalette8 {
		if first, ok := seen[c]; ok {
			t.Errorf("index %d repeats index %d (%v)", i, first, c)
		}
		seen[c] = i
	}
}

// bitmapMask builds a "#" element: a one bit icon followed by its one bit
// mask, with the left half of the icon opaque.
func bitmapMask(w, h int) []byte {
	plane := w * h / 8
	out := make([]byte, plane*2)
	for i := 0; i < w*h; i++ {
		x := i % w
		if x < w/4 {
			out[i/8] |= 0x80 >> (i % 8) // Ink.
		}
		if x < w/2 {
			out[plane+i/8] |= 0x80 >> (i % 8) // Opaque.
		}
	}
	return out
}

func TestDecodeIndexed(t *testing.T) {
	t.Parallel()

	t.Run("one bit icon carries its own mask", func(st *testing.T) {
		data := file(encodeElement("ICN#", bitmapMask(32, 32)))
		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if got := img.Bounds().Size(); got != image.Pt(32, 32) {
			st.Fatalf("decoded %v, want 32x32", got)
		}
		at := func(x, y int) color.NRGBA {
			return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
		}
		if got := at(2, 5); got != (color.NRGBA{0, 0, 0, 0xFF}) {
			st.Errorf("ink pixel = %v, want opaque black", got)
		}
		if got := at(12, 5); got != (color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
			st.Errorf("paper pixel = %v, want opaque white", got)
		}
		if got := at(24, 5); got.A != 0 {
			st.Errorf("pixel outside the mask = %v, want transparent", got)
		}
	})

	t.Run("eight bit icon takes the companion mask", func(st *testing.T) {
		// Index 3 of the 256 table, well inside the colour cube.
		pixels := make([]byte, 32*32)
		for i := range pixels {
			pixels[i] = 3
		}
		data := file(
			encodeElement("icl8", pixels),
			encodeElement("ICN#", bitmapMask(32, 32)),
		)
		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		at := func(x, y int) color.NRGBA {
			return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
		}
		want := macPalette8[3]
		if got := at(4, 4); got != want {
			st.Errorf("inside the mask = %v, want %v", got, want)
		}
		if got := at(24, 4); got.A != 0 {
			st.Errorf("outside the mask = %v, want transparent", got)
		}
	})

	t.Run("four bit icon packs two pixels per byte", func(st *testing.T) {
		pixels := make([]byte, 16*16/2)
		for i := range pixels {
			pixels[i] = 0x3A // Index 3 then index 10.
		}
		data := file(
			encodeElement("ics4", pixels),
			encodeElement("ics#", bitmapMask(16, 16)),
		)
		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		at := func(x, y int) color.NRGBA {
			return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
		}
		if got := at(0, 0); got != macPalette4[3] {
			st.Errorf("first pixel = %v, want %v", got, macPalette4[3])
		}
		if got := at(1, 0); got != macPalette4[10] {
			st.Errorf("second pixel = %v, want %v", got, macPalette4[10])
		}
	})

	t.Run("the mini icon is not square", func(st *testing.T) {
		data := file(
			encodeElement("icm8", make([]byte, 16*12)),
			encodeElement("icm#", bitmapMask(16, 12)),
		)
		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if got := img.Bounds().Size(); got != image.Pt(16, 12) {
			st.Fatalf("decoded %v, want 16x12", got)
		}
	})

	t.Run("richer depths come first", func(st *testing.T) {
		data := file(
			encodeElement("ICN#", bitmapMask(32, 32)),
			encodeElement("icl4", make([]byte, 32*32/2)),
			encodeElement("icl8", make([]byte, 32*32)),
		)
		d, err := NewDecoder(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		var got []string
		for _, icon := range d.Icons() {
			got = append(got, icon.ID)
		}
		want := []string{"icl8", "icl4", "ICN#"}
		for i := range want {
			if got[i] != want[i] {
				st.Fatalf("icons = %v, want %v", got, want)
			}
		}
	})

	t.Run("short data", func(st *testing.T) {
		data := file(encodeElement("icl8", make([]byte, 10)))
		if _, err := Decode(bytes.NewReader(data)); !errors.Is(err, ErrMalformed) {
			st.Fatalf("error = %v, want ErrMalformed", err)
		}
	})
}
