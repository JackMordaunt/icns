package icns

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"reflect"
	"testing"
)

// TestDecode relies on Encode being correct.
// We are testing that an ICNS with a series of icons will only yield the
// largest icon in the series.
func TestDecode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc  string
		input image.Image
		want  int // Side of the decoded icon.
	}{
		{"valid square icon, exact size", gradient(256), 256},
		{"non exact size", gradient(50), 32},
		// 32px wide but with Max at 72: measuring Max instead of Dx would
		// pick the 64px tier and upscale.
		{"not at origin", gradient(128).SubImage(image.Rect(40, 40, 72, 72)), 32},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			buf := bytes.NewBuffer(nil)
			if err := Encode(buf, tt.input); err != nil {
				st.Fatalf("unexpected error while encoding: %v", err)
			}
			img, err := Decode(buf)
			if err != nil {
				st.Fatalf("unexpected error: %v", err)
			}
			if got := img.Bounds().Size(); got != image.Pt(tt.want, tt.want) {
				st.Fatalf("decoded icon is %v, want %dx%d", got, tt.want, tt.want)
			}
		})
	}
}

// TestRoundTrip checks that an image of an exact icon size survives
// encode(decode(img)) pixel for pixel: no resampling happens for that size
// and PNG is lossless.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	src := gradient(128)
	buf := bytes.NewBuffer(nil)
	if err := Encode(buf, src); err != nil {
		t.Fatal(err)
	}
	imgs, err := DecodeAll(buf)
	if err != nil {
		t.Fatal(err)
	}
	// ic07, ic12, ic11, il32 and is32.
	if len(imgs) != 5 {
		t.Fatalf("DecodeAll returned %d icons, want 5", len(imgs))
	}
	if !imageCompare(imgs[0], src) {
		t.Fatal("largest decoded icon differs from the source image")
	}
}

// TestInterpolationFunctions checks that every algorithm, and any value
// outside the enumeration, produces the full icon set at the right sizes.
func TestInterpolationFunctions(t *testing.T) {
	t.Parallel()
	src := gradient(128)
	tests := []struct {
		desc   string
		interp InterpolationFunction
	}{
		{"nearest neighbor", NearestNeighbor},
		{"bilinear", Bilinear},
		{"bicubic", Bicubic},
		{"mitchell-netravali", MitchellNetravali},
		{"lanczos2", Lanczos2},
		{"lanczos3", Lanczos3},
		{"out of range falls back to the default", InterpolationFunction(99)},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			buf := bytes.NewBuffer(nil)
			if err := NewEncoder(buf).WithAlgorithm(tt.interp).Encode(src); err != nil {
				st.Fatalf("encoding: %v", err)
			}
			imgs, err := DecodeAll(buf)
			if err != nil {
				st.Fatalf("decoding: %v", err)
			}
			var sides []int
			for _, img := range imgs {
				b := img.Bounds()
				if b.Dx() != b.Dy() {
					st.Fatalf("icon is not square: %v", b)
				}
				sides = append(sides, b.Dx())
			}
			// 32 twice: ic11 carries 16@2x and il32 carries a true 32.
			if want := []int{128, 64, 32, 32, 16}; !reflect.DeepEqual(sides, want) {
				st.Fatalf("icon sides = %v, want %v", sides, want)
			}
			// A resampled icon must carry the source's colour, not a blank
			// or transparent frame.
			if _, _, _, a := imgs[1].At(32, 32).RGBA(); a == 0 {
				st.Error("the resampled 64px icon is transparent at its centre")
			}
		})
	}
}

// TestEncodedElements pins the set of elements the encoder writes, which
// matches what iconutil produces for a full iconset.
func TestEncodedElements(t *testing.T) {
	t.Parallel()
	buf := bytes.NewBuffer(nil)
	if err := Encode(buf, gradient(1024)); err != nil {
		t.Fatal(err)
	}
	els, err := elementsOf(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, el := range els {
		got = append(got, el.id)
	}
	want := []string{
		"TOC ", "ic10", "ic14", "ic09", "ic13", "ic08", "ic07", "ic12",
		"ic11", "il32", "l8mk", "is32", "s8mk",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("elements = %v, want %v", got, want)
	}
	// The table of contents covers everything after it: type and total size.
	toc := els[0].payload
	if len(toc) != (len(els)-1)*elementHeaderSize {
		t.Fatalf("table of contents is %d bytes, want %d", len(toc), (len(els)-1)*elementHeaderSize)
	}
	for i, el := range els[1:] {
		entry := toc[i*elementHeaderSize:]
		if id := string(entry[:4]); id != el.id {
			t.Errorf("entry %d names %q, want %q", i, id, el.id)
		}
		size := binary.BigEndian.Uint32(entry[4:8])
		if want := uint32(elementHeaderSize + len(el.payload)); size != want {
			t.Errorf("entry %d for %s gives size %d, want %d", i, el.id, size, want)
		}
	}
}

// TestLegacyRoundTrip checks that a 16px source survives the colour and mask
// encoding unchanged: it is written at its own size, so nothing is resampled
// and the planes are lossless.
func TestLegacyRoundTrip(t *testing.T) {
	t.Parallel()
	src := gradient(16)
	buf := bytes.NewBuffer(nil)
	if err := Encode(buf, src); err != nil {
		t.Fatal(err)
	}
	img, err := Decode(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !imageCompare(img, src) {
		t.Fatal("the decoded icon differs from the source")
	}
}

// TestResizeKeepsColorOutOfTransparentPixels guards the reason resampling
// happens in premultiplied space. Filtering an opaque edge against
// transparent pixels in straight space drags their colour into the edge,
// the familiar dark halo around a downscaled icon.
func TestResizeKeepsColorOutOfTransparentPixels(t *testing.T) {
	t.Parallel()
	// Left half opaque white, right half transparent black.
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 32; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	// Bilinear has no negative lobes, so every output pixel is a plain
	// average of its neighbours and the expected values are exact.
	got := resizeSquare(src, 32, Bilinear)
	var blended int
	for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
		for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
			c := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				continue // Fully transparent: colour is unobservable.
			}
			if c.A < 255 {
				blended++
			}
			if c.R != 255 || c.G != 255 || c.B != 255 {
				t.Fatalf("pixel (%d,%d) is %v, want white at any alpha", x, y, c)
			}
		}
	}
	// Without pixels that actually mix the two halves there is nothing to
	// bleed, and the check above would pass for the wrong reason.
	if blended == 0 {
		t.Fatal("no partially transparent pixels: the edge never blended")
	}
}

// imageCompare reports whether two images have identical bounds and pixels.
func imageCompare(left, right image.Image) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	lb := left.Bounds()
	if lb.Size() != right.Bounds().Size() {
		return false
	}
	offset := right.Bounds().Min.Sub(lb.Min)
	for x := lb.Min.X; x < lb.Max.X; x++ {
		for y := lb.Min.Y; y < lb.Max.Y; y++ {
			lr, lg, lbl, la := left.At(x, y).RGBA()
			rr, rg, rb, ra := right.At(x+offset.X, y+offset.Y).RGBA()
			if lr != rr || lg != rg || lbl != rb || la != ra {
				return false
			}
		}
	}
	return true
}

// TestEncode tests for input validation, sanity checks and errors.
// The validity of the encoding is not tested here.
// Super large images are not tested because the resizing takes too
// long for unit testing.
func TestEncode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		wr   io.Writer
		img  image.Image

		wantErr bool
	}{
		{
			"nil image",
			io.Discard,
			nil,
			true,
		},
		{
			"nil writer",
			nil,
			rect(0, 0, 50, 50),
			true,
		},
		{
			"valid sqaure",
			io.Discard,
			rect(0, 0, 50, 50),
			false,
		},
		{
			"valid non-square",
			io.Discard,
			rect(0, 0, 10, 50),
			false,
		},
		{
			"valid non-square, weird dimensions",
			io.Discard,
			rect(0, 0, 17, 77),
			false,
		},
		{
			"invalid zero img",
			io.Discard,
			rect(0, 0, 0, 0),
			true,
		},
		{
			"invalid small img",
			io.Discard,
			rect(0, 0, 1, 1),
			true,
		},
		{
			"valid square not at origin point",
			io.Discard,
			rect(10, 10, 50, 50),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			err := Encode(tt.wr, tt.img)
			if tt.wantErr && err == nil {
				st.Fatal("expected an error")
			}
			if !tt.wantErr && err != nil {
				st.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSizesFromMax(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		from uint
		want []uint
	}{
		{
			"small",
			100,
			[]uint{64, 32, 16},
		},
		{
			"large",
			99999,
			[]uint{1024, 512, 256, 128, 64, 32, 16},
		},
		{
			"smallest",
			0,
			[]uint{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			got := sizesFrom(tt.from)
			if !reflect.DeepEqual(got, tt.want) {
				st.Errorf("want=%d, got=%d", tt.want, got)
			}
		})
	}
}

func TestBiggestSide(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		img  image.Image
		want uint
	}{
		{
			"equal",
			rect(0, 0, 100, 100),
			100,
		},
		{
			"right larger",
			rect(0, 0, 50, 100),
			100,
		},
		{
			"left larger",
			rect(0, 0, 100, 50),
			100,
		},
		{
			"off by one",
			rect(0, 0, 100, 99),
			100,
		},
		{
			"empty",
			rect(0, 0, 0, 0),
			0,
		},
		{
			"left empty",
			rect(0, 0, 0, 10),
			10,
		},
		{
			"right empty",
			rect(0, 0, 10, 0),
			10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			got := biggestSide(tt.img)
			if got != tt.want {
				st.Errorf("want=%d, got=%d", tt.want, got)
			}
		})
	}
}

func TestFindNearestSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		img  image.Image
		want uint
	}{
		{
			"small",
			rect(0, 0, 100, 100),
			64,
		},
		{
			"very large",
			rect(0, 0, 123456789, 123456789),
			1024,
		},
		{
			"too small",
			rect(0, 0, 15, 15),
			0,
		},
		{
			"off by one",
			rect(0, 0, 33, 33),
			32,
		},
		{
			"exact",
			rect(0, 0, 256, 256),
			256,
		},
		{
			"exact",
			rect(0, 0, 1024, 1024),
			1024,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			got := findNearestSize(tt.img)
			if tt.want != got {
				st.Errorf("want=%d, got=%d", tt.want, got)
			}
		})
	}
}

func TestEncodeImage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string

		img    image.Image
		format string

		want string
	}{
		{
			"png - png",
			_decode(_png(rect(0, 0, 50, 50))),
			"png",
			"png",
		},
		{
			"default png - png",
			_decode(_png(rect(0, 0, 50, 50))),
			"",
			"png",
		},
		{
			"jpg - jpg",
			_decode(_jpg(rect(0, 0, 50, 50))),
			"jpeg",
			"png",
		},
		{
			"default jpg - png",
			_decode(_jpg(rect(0, 0, 50, 50))),
			"",
			"png",
		},
		{
			"invalid format identifier",
			_decode(_jpg(rect(0, 0, 50, 50))),
			"asdf",
			"png",
		},
		{
			"not actually a jpeg",
			_decode(_png(rect(0, 0, 50, 50))),
			"jpeg",
			"png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			data, err := encodeImage(tt.img)
			if err != nil {
				st.Fatalf("encoding image: %v", err)
			}
			_, f, err := image.Decode(bytes.NewBuffer(data))
			if err != nil {
				st.Fatalf("decoding iamge: %v", err)
			}
			if f != tt.want {
				st.Fatalf("formats: want=%s, got=%s", tt.want, f)
			}
		})
	}
}

func rect(x0, y0, x1, y1 int) image.Image {
	return image.Rect(x0, y0, x1, y1)
}

func _png(img image.Image) io.Reader {
	buf := bytes.NewBuffer(nil)
	if err := png.Encode(buf, img); err != nil {
		panic(fmt.Errorf("encoding png: %w", err))
	}
	return buf
}

func _jpg(img image.Image) io.Reader {
	buf := bytes.NewBuffer(nil)
	if err := jpeg.Encode(buf, img, nil); err != nil {
		panic(fmt.Errorf("encoding jpeg: %w", err))
	}
	return buf
}

func _decode(r io.Reader) image.Image {
	m, _, err := image.Decode(r)
	if err != nil {
		panic(fmt.Errorf("decoding image: %w", err))
	}
	return m
}
