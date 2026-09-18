package icns

import (
	"bytes"
	"image"
	"image/color"
	"reflect"
	"testing"
)

// slotOf names the element that fills each slot, which is what per-slot
// artwork has to land in.
var slotOf = map[string]Slot{
	"ic10": {512, 2},
	"ic09": {512, 1},
	"ic14": {256, 2},
	"ic08": {256, 1},
	"ic13": {128, 2},
	"ic07": {128, 1},
	"ic12": {32, 2},
	"ic11": {16, 2},
	"il32": {32, 1},
	"is32": {16, 1},
}

func solid(side int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// centre reports the colour at the middle of img.
func centre(img image.Image) color.NRGBA {
	b := img.Bounds()
	return color.NRGBAModel.Convert(img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)).(color.NRGBA)
}

func TestParseSlot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want Slot
		ok   bool
	}{
		{"icon_16x16.png", Slot{16, 1}, true},
		{"icon_16x16@2x.png", Slot{16, 2}, true},
		{"icon_32x32@2x.png", Slot{32, 2}, true},
		{"icon_512x512.png", Slot{512, 1}, true},
		{"icon_512x512@2x.png", Slot{512, 2}, true},
		{"icon_16x32.png", Slot{}, false},
		{"icon_0x0.png", Slot{}, false},
		{"icon_16x16@0x.png", Slot{}, false},
		{"icon_16x16.jpg", Slot{}, false},
		{"16x16.png", Slot{}, false},
		{"icon_16x16@2x.png.bak", Slot{}, false},
		{"", Slot{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(st *testing.T) {
			got, ok := ParseSlot(tt.name)
			if ok != tt.ok || got != tt.want {
				st.Fatalf("ParseSlot(%q) = %v, %v; want %v, %v", tt.name, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSlotString(t *testing.T) {
	t.Parallel()
	if got := (Slot{16, 1}).String(); got != "16x16" {
		t.Errorf("got %q, want 16x16", got)
	}
	if got := (Slot{512, 2}).String(); got != "512x512@2x" {
		t.Errorf("got %q, want 512x512@2x", got)
	}
	if got := (Slot{16, 2}).Pixels(); got != 32 {
		t.Errorf("16x16@2x is %d pixels, want 32", got)
	}
}

// TestSlots checks that the slots reported are exactly the ten an iconset
// holds, so a caller knows what artwork to supply.
func TestSlots(t *testing.T) {
	t.Parallel()
	got := Slots()
	if len(got) != len(slotOf) {
		t.Fatalf("Slots returned %d entries, want %d", len(got), len(slotOf))
	}
	seen := map[Slot]bool{}
	for _, slot := range got {
		seen[slot] = true
	}
	for id, slot := range slotOf {
		if !seen[slot] {
			t.Errorf("slot %s, filled by %s, is missing", slot, id)
		}
	}
	// Largest artwork first.
	for i := 1; i < len(got); i++ {
		if got[i].Pixels() > got[i-1].Pixels() {
			t.Fatalf("slot %d (%s) is larger than the one before it (%s)", i, got[i], got[i-1])
		}
	}
}

// TestEncodeSlots checks that artwork given for a slot reaches the element
// that fills it, including the three pixel sizes two slots share.
func TestEncodeSlots(t *testing.T) {
	t.Parallel()
	var (
		images = map[Slot]image.Image{}
		want   = map[Slot]color.NRGBA{}
	)
	for i, slot := range Slots() {
		c := color.NRGBA{R: uint8(10 + i*20), G: uint8(200 - i*15), B: 0x40, A: 0xFF}
		// Artwork at exactly the slot's size, so nothing is resampled and
		// the colour has to survive exactly.
		images[slot] = solid(int(slot.Pixels()), c)
		want[slot] = c
	}
	buf := bytes.NewBuffer(nil)
	if err := NewEncoder(buf).EncodeSlots(images); err != nil {
		t.Fatal(err)
	}
	d, err := NewDecoder(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	icons := d.Icons()
	if len(icons) != len(slotOf) {
		t.Fatalf("encoded %d icons, want %d", len(icons), len(slotOf))
	}
	for _, icon := range icons {
		slot, ok := slotOf[icon.ID]
		if !ok {
			t.Fatalf("unexpected element %s", icon.ID)
		}
		img, err := icon.Decode()
		if err != nil {
			t.Fatalf("%s: %v", icon.ID, err)
		}
		if got := uint(img.Bounds().Dx()); got != slot.Pixels() {
			t.Errorf("%s is %d pixels, want %d", icon.ID, got, slot.Pixels())
		}
		if got := centre(img); got != want[slot] {
			t.Errorf("%s fills slot %s with %v, want %v", icon.ID, slot, got, want[slot])
		}
	}
}

func TestEncodeSlotsFallback(t *testing.T) {
	t.Parallel()
	var (
		source = color.NRGBA{R: 0x20, G: 0x40, B: 0x60, A: 0xFF}
		tuned  = color.NRGBA{R: 0xF0, G: 0x10, B: 0x80, A: 0xFF}
	)
	// Only two slots are given artwork, and the 16 pixel one is supplied at
	// the wrong size so it has to be resized into place.
	images := map[Slot]image.Image{
		{Points: 512, Scale: 2}: solid(1024, source),
		{Points: 16, Scale: 1}:  solid(64, tuned),
	}
	buf := bytes.NewBuffer(nil)
	if err := NewEncoder(buf).EncodeSlots(images); err != nil {
		t.Fatal(err)
	}
	d, err := NewDecoder(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(d.Icons()); got != len(slotOf) {
		t.Fatalf("encoded %d icons, want the full set of %d", got, len(slotOf))
	}
	for _, icon := range d.Icons() {
		img, err := icon.Decode()
		if err != nil {
			t.Fatalf("%s: %v", icon.ID, err)
		}
		want := source
		if icon.ID == "is32" {
			want = tuned
			if got := img.Bounds().Dx(); got != 16 {
				t.Errorf("is32 is %d pixels, want 16", got)
			}
		}
		if got := centre(img); got != want {
			t.Errorf("%s = %v, want %v", icon.ID, got, want)
		}
	}
}

func TestEncodeSlotsRejects(t *testing.T) {
	t.Parallel()
	buf := bytes.NewBuffer(nil)
	if err := NewEncoder(buf).EncodeSlots(nil); err == nil {
		t.Error("encoding without artwork was accepted")
	}
	if err := NewEncoder(buf).EncodeSlots(map[Slot]image.Image{{16, 1}: nil}); err == nil {
		t.Error("encoding a nil image was accepted")
	}
	if err := NewEncoder(buf).EncodeSlots(map[Slot]image.Image{{16, 1}: solid(8, color.NRGBA{})}); err == nil {
		t.Error("encoding artwork below the smallest icon was accepted")
	}
}

// TestEncodeSlotsMatchesSingleImage checks the two entry points agree when
// the artwork is the same.
func TestEncodeSlotsMatchesSingleImage(t *testing.T) {
	t.Parallel()
	src := gradient(256)
	var single, slots bytes.Buffer
	if err := Encode(&single, src); err != nil {
		t.Fatal(err)
	}
	if err := NewEncoder(&slots).EncodeSlots(map[Slot]image.Image{{Points: 128, Scale: 2}: src}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(single.Bytes(), slots.Bytes()) {
		t.Fatal("the same artwork through each entry point produced different files")
	}
}
