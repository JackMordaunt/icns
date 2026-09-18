package icns

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"
)

// rleLiterals encodes data as literal runs only, which is the simplest valid
// encoding and always longer than the input.
func rleLiterals(data []byte) []byte {
	var out []byte
	for len(data) > 0 {
		n := min(len(data), 128)
		out = append(out, byte(n-1))
		out = append(out, data[:n]...)
		data = data[n:]
	}
	return out
}

func TestUnpackRLE(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		data []byte
		// want is the number of bytes asked for, which the caller knows from
		// the icon's dimensions. It is stated rather than derived, because a
		// stream whose length equals it is read as uncompressed.
		want int
		out  []byte
		err  error
	}{
		{
			desc: "literal run",
			data: []byte{0x02, 0x01, 0x02, 0x03},
			want: 3,
			out:  []byte{0x01, 0x02, 0x03},
		},
		{
			desc: "repeat run",
			data: []byte{0x80, 0x07},
			want: 3,
			out:  []byte{0x07, 0x07, 0x07},
		},
		{
			desc: "literal then repeat",
			data: []byte{0x02, 0x01, 0x02, 0x02, 0x82, 0x03},
			want: 8,
			out:  []byte{0x01, 0x02, 0x02, 0x03, 0x03, 0x03, 0x03, 0x03},
		},
		{
			desc: "longest repeat",
			data: []byte{0xFF, 0x09},
			want: 130,
			out:  bytes.Repeat([]byte{0x09}, 130),
		},
		{
			desc: "stored uncompressed",
			data: []byte{0x01, 0x02, 0x03},
			want: 3,
			out:  []byte{0x01, 0x02, 0x03},
		},
		{
			desc: "literal run overruns the element",
			data: []byte{0x7F, 0x01, 0x02},
			want: 200,
			err:  ErrMalformed,
		},
		{
			desc: "repeat run with no byte to repeat",
			data: []byte{0x04, 0x01, 0x02, 0x03, 0x04, 0x05, 0x80},
			want: 200,
			err:  ErrMalformed,
		},
		{
			desc: "expands short",
			data: []byte{0x00, 0x01},
			want: 3,
			err:  ErrMalformed,
		},
		{
			desc: "a single run expands past what was asked for",
			data: []byte{0xFF, 0x09},
			want: 3,
			err:  ErrMalformed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			got, err := unpackRLE(tt.data, tt.want)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					st.Fatalf("error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				st.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.out) {
				st.Fatalf("got %v, want %v", got, tt.out)
			}
		})
	}
}

func TestPadRLE(t *testing.T) {
	t.Parallel()
	// Compressed data gets a byte so a reader that drops the last value of
	// the stream loses the padding instead of a pixel.
	if got := padRLE([]byte{0x80, 0x07}, 3); !bytes.Equal(got, []byte{0x80, 0x07, 0x00}) {
		t.Errorf("compressed data = %v, want a trailing zero", got)
	}
	// Uncompressed data has to keep its exact length, which is how a reader
	// tells that it was never compressed.
	raw := []byte{1, 2, 3}
	if got := padRLE(raw, len(raw)); !bytes.Equal(got, raw) {
		t.Errorf("uncompressed data = %v, want it unchanged", got)
	}
	// Whatever the padding, the data still reads back.
	planes := bytes.Repeat([]byte{0x40}, 768)
	out, err := unpackRLE(padRLE(packRLE(planes), len(planes)), len(planes))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, planes) {
		t.Fatal("padded data did not survive the round trip")
	}
}

// legacyIcon builds an is32 colour element and its s8mk mask for a gradient,
// returning the elements and the image they describe.
func legacyIcon(side int) (rgb, mask []byte, want *image.NRGBA) {
	pixels := side * side
	planes := make([]byte, pixels*3)
	mask = make([]byte, pixels)
	want = image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			i := y*side + x
			c := color.NRGBA{
				R: uint8(x * 255 / side),
				G: uint8(y * 255 / side),
				B: 0x40,
				A: uint8((x + y) * 255 / (2 * side)),
			}
			planes[i] = c.R
			planes[pixels+i] = c.G
			planes[pixels*2+i] = c.B
			mask[i] = c.A
			want.SetNRGBA(x, y, c)
		}
	}
	return rleLiterals(planes), mask, want
}

func TestDecodeLegacyElements(t *testing.T) {
	t.Parallel()
	const side = 16
	rgb, mask, want := legacyIcon(side)

	t.Run("colour and mask", func(st *testing.T) {
		data := file(encodeElement("is32", rgb), encodeElement("s8mk", mask))
		imgs, err := DecodeAll(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if len(imgs) != 1 {
			st.Fatalf("decoded %d icons, want 1", len(imgs))
		}
		if !imageCompare(imgs[0], want) {
			st.Fatal("decoded icon differs from the source")
		}
	})

	t.Run("mask ahead of the colour", func(st *testing.T) {
		// Apple writes the mask after its element, but nothing requires it.
		data := file(encodeElement("s8mk", mask), encodeElement("is32", rgb))
		imgs, err := DecodeAll(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if !imageCompare(imgs[0], want) {
			st.Fatal("decoded icon differs from the source")
		}
	})

	t.Run("no mask leaves the icon opaque", func(st *testing.T) {
		data := file(encodeElement("is32", rgb))
		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		for y := 0; y < side; y++ {
			for x := 0; x < side; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a != 0xFFFF {
					st.Fatalf("pixel (%d,%d) alpha = %d, want opaque", x, y, a)
				}
			}
		}
	})

	t.Run("uncompressed colour", func(st *testing.T) {
		pixels := side * side
		planes := make([]byte, pixels*3)
		for i := range planes {
			planes[i] = byte(i)
		}
		data := file(encodeElement("is32", planes), encodeElement("s8mk", mask))
		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if got := img.Bounds().Dx(); got != side {
			st.Fatalf("decoded a %dpx icon, want %d", got, side)
		}
	})

	t.Run("mask of the wrong length", func(st *testing.T) {
		data := file(encodeElement("is32", rgb), encodeElement("s8mk", mask[:10]))
		if _, err := Decode(bytes.NewReader(data)); !errors.Is(err, ErrMalformed) {
			st.Fatalf("error = %v, want ErrMalformed", err)
		}
	})
}

// TestDecodeIT32Header covers the one colour element that prefixes its planes
// with four zero bytes.
func TestDecodeIT32Header(t *testing.T) {
	t.Parallel()
	const side = 128
	pixels := side * side
	planes := make([]byte, pixels*3)
	for i := range planes {
		planes[i] = byte(i / side)
	}
	rgb := append([]byte{0, 0, 0, 0}, rleLiterals(planes)...)
	mask := bytes.Repeat([]byte{0xFF}, pixels)
	data := file(encodeElement("it32", rgb), encodeElement("t8mk", mask))
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got != image.Pt(side, side) {
		t.Fatalf("decoded %v, want %dx%d", got, side, side)
	}
}
