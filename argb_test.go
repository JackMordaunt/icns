package icns

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"
)

// argbElement builds an ARGB payload for a gradient, returning it and the
// image it describes.
func argbElement(side int) (payload []byte, want *image.NRGBA) {
	pixels := side * side
	planes := make([]byte, pixels*4)
	want = image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			i := y*side + x
			c := color.NRGBA{
				R: uint8(x * 255 / side),
				G: uint8(y * 255 / side),
				B: 0x80,
				A: uint8(255 - x*128/side),
			}
			// Alpha leads, then the colour channels.
			planes[i] = c.A
			planes[pixels+i] = c.R
			planes[pixels*2+i] = c.G
			planes[pixels*3+i] = c.B
			want.SetNRGBA(x, y, c)
		}
	}
	return append([]byte("ARGB"), rleLiterals(planes)...), want
}

func TestDecodeARGB(t *testing.T) {
	t.Parallel()
	const side = 16
	payload, want := argbElement(side)

	t.Run("sidebar icon", func(st *testing.T) {
		data := file(encodeElement("ic04", payload))
		desc, err := Probe(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if len(desc) != 1 || desc[0].ImageFormat != ImageFormatARGB {
			st.Fatalf("Probe = %v, want one ARGB icon", desc)
		}
		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if !imageCompare(img, want) {
			st.Fatal("the decoded icon differs from the source")
		}
	})

	t.Run("compressed run", func(st *testing.T) {
		// A flat image compresses to repeats, which exercises the other half
		// of the run-length decoder. icsb is 18 pixels, not 16.
		const icsbSide = 18
		planes := make([]byte, icsbSide*icsbSide*4)
		for i := range planes {
			planes[i] = 0xFF
		}
		data := file(encodeElement("icsb", append([]byte("ARGB"), packRLE(planes)...)))
		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if got := img.Bounds().Dx(); got != 18 {
			st.Fatalf("decoded a %dpx icon, want 18", got)
		}
		if got := color.NRGBAModel.Convert(img.At(9, 9)).(color.NRGBA); got != (color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
			st.Fatalf("centre = %v, want opaque white", got)
		}
	})

	t.Run("png in an ARGB capable type", func(st *testing.T) {
		// These types hold either format, so the payload has to decide.
		data := file(encodeElement("ic05", pngBytes(t, 32)))
		desc, err := Probe(bytes.NewReader(data))
		if err != nil {
			st.Fatal(err)
		}
		if desc[0].ImageFormat != ImageFormatPNG {
			st.Fatalf("format = %s, want PNG", desc[0].ImageFormat)
		}
		if _, err := Decode(bytes.NewReader(data)); err != nil {
			st.Fatal(err)
		}
	})

	t.Run("truncated planes", func(st *testing.T) {
		data := file(encodeElement("ic04", []byte("ARGB\x01\x02\x03")))
		if _, err := Decode(bytes.NewReader(data)); !errors.Is(err, ErrMalformed) {
			st.Fatalf("error = %v, want ErrMalformed", err)
		}
	})
}
