package ico

import (
	"bytes"
	"errors"
	"testing"
)

// payload stands in for an encoded icon, which Assemble copies rather than
// reads.
func payload(n int, fill byte) []byte { return bytes.Repeat([]byte{fill}, n) }

func TestAssembleKeepsWhatItWasGiven(t *testing.T) {
	icons := []Icon{
		{Width: 256, Height: 256, Planes: 1, Bits: 32, Data: payload(40, 0xA1)},
		{Width: 32, Height: 32, Colours: 16, Planes: 1, Bits: 4, Data: payload(16, 0xB2)},
		{Width: 16, Height: 16, Planes: 1, Bits: 32, Data: payload(8, 0xC3)},
	}
	file, err := Assemble(icons)
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	d, err := NewDecoder(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	got := d.Icons()
	if len(got) != len(icons) {
		t.Fatalf("read %d icons, want %d", len(got), len(icons))
	}
	// Icons comes back largest first, which is the order they went in.
	for i, want := range icons {
		if got[i].Width != want.Width || got[i].Height != want.Height {
			t.Errorf("icon %d is %dx%d, want %dx%d", i, got[i].Width, got[i].Height, want.Width, want.Height)
		}
		if !bytes.Equal(got[i].Payload(), want.Data) {
			t.Errorf("icon %d holds different pixels from the ones given", i)
		}
	}
}

// TestAssembleWritesTheLargestAsZero covers the one dimension that does not
// fit in the byte the directory keeps it in.
func TestAssembleWritesTheLargestAsZero(t *testing.T) {
	file, err := Assemble([]Icon{{Width: 256, Height: 256, Planes: 1, Bits: 32, Data: payload(8, 0xFF)}})
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	row := file[directorySize:]
	if row[0] != 0 || row[1] != 0 {
		t.Errorf("the row records %dx%d, want zero for the largest", row[0], row[1])
	}
	d, err := NewDecoder(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if got := d.Icons()[0]; got.Width != 256 || got.Height != 256 {
		t.Errorf("read back as %dx%d, want 256x256", got.Width, got.Height)
	}
}

func TestAssembleRefusesWhatItCannotWrite(t *testing.T) {
	for _, tt := range []struct {
		name  string
		icons []Icon
		want  error
	}{
		{"nothing at all", nil, ErrNoIcons},
		{
			name:  "an icon with no pixels",
			icons: []Icon{{Width: 16, Height: 16, Planes: 1, Bits: 32}},
			want:  ErrMalformed,
		},
		{
			name:  "larger than the directory can record",
			icons: []Icon{{Width: 512, Height: 512, Planes: 1, Bits: 32, Data: payload(8, 1)}},
			want:  ErrMalformed,
		},
		{
			name:  "no size at all",
			icons: []Icon{{Width: 0, Height: 16, Planes: 1, Bits: 32, Data: payload(8, 1)}},
			want:  ErrMalformed,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Assemble(tt.icons); !errors.Is(err, tt.want) {
				t.Errorf("error is %v, want %v", err, tt.want)
			}
		})
	}
}

// TestAssembleIsWhatTheEncoderUses keeps the two from drifting: an encoded
// file and one assembled from the same payloads are the same file.
func TestAssembleIsWhatTheEncoderUses(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, gradient(64)); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	d, err := NewDecoder(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	var stored []Icon
	for _, icon := range d.Icons() {
		stored = append(stored, Icon{
			Width:  icon.Width,
			Height: icon.Height,
			Planes: 1,
			Bits:   32,
			Data:   icon.Payload(),
		})
	}
	again, err := Assemble(stored)
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if !bytes.Equal(again, buf.Bytes()) {
		t.Errorf("assembled %d bytes, want the %d the encoder wrote", len(again), buf.Len())
	}
}
