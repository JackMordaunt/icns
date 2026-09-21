package ico

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"reflect"
	"testing"
)

// gradient returns a side by side image whose colour and alpha vary with
// position, so a round trip cannot pass by accident.
func gradient(side int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
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

func solid(side int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func centre(img image.Image) color.NRGBA {
	b := img.Bounds()
	return color.NRGBAModel.Convert(img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)).(color.NRGBA)
}

func same(a, b image.Image) bool {
	if a.Bounds().Size() != b.Bounds().Size() {
		return false
	}
	ab, bb := a.Bounds(), b.Bounds()
	for y := 0; y < ab.Dy(); y++ {
		for x := 0; x < ab.Dx(); x++ {
			if a.At(ab.Min.X+x, ab.Min.Y+y) != b.At(bb.Min.X+x, bb.Min.Y+y) {
				ar, ag, al, aa := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
				br, bg, bl, ba := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
				if ar != br || ag != bg || al != bl || aa != ba {
					return false
				}
			}
		}
	}
	return true
}

func TestEncode(t *testing.T) {
	t.Parallel()
	buf := bytes.NewBuffer(nil)
	if err := Encode(buf, gradient(256)); err != nil {
		t.Fatal(err)
	}
	d, err := NewDecoder(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var (
		got     []int
		formats []Format
	)
	for _, icon := range d.Icons() {
		got = append(got, icon.Width)
		formats = append(formats, icon.Format)
		if icon.Width != icon.Height {
			t.Errorf("icon %s is not square", icon)
		}
	}
	if want := []int{256, 128, 64, 48, 32, 24, 16}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sizes = %v, want %v", got, want)
	}
	// The largest is a PNG, since a bitmap at 256 is a quarter of a megabyte.
	want := []Format{FormatPNG, FormatBMP, FormatBMP, FormatBMP, FormatBMP, FormatBMP, FormatBMP}
	if !reflect.DeepEqual(formats, want) {
		t.Fatalf("formats = %v, want %v", formats, want)
	}
}

// TestDirectory reads the directory by hand, since every reader of the file
// finds its icons through those offsets.
func TestDirectory(t *testing.T) {
	t.Parallel()
	buf := bytes.NewBuffer(nil)
	if err := Encode(buf, gradient(64)); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if got := binary.LittleEndian.Uint16(data[0:2]); got != 0 {
		t.Errorf("reserved = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(data[2:4]); got != 1 {
		t.Errorf("type = %d, want 1 for an icon", got)
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count != 5 { // 64, 48, 32, 24 and 16.
		t.Fatalf("count = %d, want 5", count)
	}
	offset := directorySize + entrySize*count
	for i := range count {
		row := data[directorySize+entrySize*i:]
		size := int(binary.LittleEndian.Uint32(row[8:12]))
		at := int(binary.LittleEndian.Uint32(row[12:16]))
		if at != offset {
			t.Errorf("icon %d starts at %d, want %d, so the images are not packed in order", i, at, offset)
		}
		if at+size > len(data) {
			t.Fatalf("icon %d runs past the end of the file", i)
		}
		offset += size
	}
	if offset != len(data) {
		t.Errorf("the icons end at %d but the file is %d bytes", offset, len(data))
	}
}

// TestRoundTrip checks that an image at an exact icon size comes back
// unchanged: nothing is resampled and a 32 bit bitmap is lossless.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	src := gradient(64)
	buf := bytes.NewBuffer(nil)
	if err := Encode(buf, src); err != nil {
		t.Fatal(err)
	}
	img, err := Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !same(img, src) {
		t.Fatal("the decoded icon differs from the source")
	}
}

func TestEncodeSizes(t *testing.T) {
	t.Parallel()
	var (
		images = map[uint]image.Image{}
		want   = map[uint]color.NRGBA{}
	)
	for i, size := range Sizes() {
		c := color.NRGBA{R: uint8(20 + i*30), G: uint8(200 - i*20), B: 0x40, A: 0xFF}
		images[size] = solid(int(size), c)
		want[size] = c
	}
	buf := bytes.NewBuffer(nil)
	if err := NewEncoder(buf).EncodeSizes(images); err != nil {
		t.Fatal(err)
	}
	d, err := NewDecoder(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	icons := d.Icons()
	if len(icons) != len(want) {
		t.Fatalf("encoded %d icons, want %d", len(icons), len(want))
	}
	for _, icon := range icons {
		img, err := icon.Decode()
		if err != nil {
			t.Fatalf("%s: %v", icon, err)
		}
		if got := centre(img); got != want[uint(icon.Width)] {
			t.Errorf("the %d pixel icon is %v, want %v", icon.Width, got, want[uint(icon.Width)])
		}
	}
}

func TestEncodeRejects(t *testing.T) {
	t.Parallel()
	buf := bytes.NewBuffer(nil)
	if err := Encode(buf, nil); err == nil {
		t.Error("encoding a nil image was accepted")
	}
	if err := Encode(nil, gradient(64)); err == nil {
		t.Error("encoding to a nil writer was accepted")
	}
	if err := Encode(buf, gradient(8)); err == nil {
		t.Error("encoding an image below the smallest icon was accepted")
	}
	if err := NewEncoder(buf).EncodeSizes(nil); err == nil {
		t.Error("encoding without artwork was accepted")
	}
}
