package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/jackmordaunt/icns/v4"
	"github.com/jackmordaunt/icns/v4/ico"
)

func art(side int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / side),
				G: uint8(y * 255 / side),
				B: 0x40,
				A: uint8(255 - y*128/side),
			})
		}
	}
	return img
}

func encoded(t *testing.T, write func(*bytes.Buffer) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	return buf.Bytes()
}

// opaqueICO holds one PNG frame with no alpha channel, which Windows passes
// over.
func opaqueICO(t *testing.T) []byte {
	t.Helper()
	solid := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for i := 0; i < len(solid.Pix); i += 4 {
		solid.Pix[i], solid.Pix[i+1], solid.Pix[i+2], solid.Pix[i+3] = 9, 9, 9, 255
	}
	frame := encoded(t, func(b *bytes.Buffer) error { return png.Encode(b, solid) })
	out := make([]byte, 0, 22+len(frame))
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = append(out, 0, 0, 0, 0)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, 32)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(frame)))
	out = binary.LittleEndian.AppendUint32(out, 22)
	return append(out, frame...)
}

// bundleICNS holds only the small PNG type that an app bundle will not
// render.
func bundleICNS(t *testing.T) []byte {
	t.Helper()
	frame := encoded(t, func(b *bytes.Buffer) error { return png.Encode(b, art(16)) })
	element := func(id string, payload []byte) []byte {
		out := append([]byte{}, id...)
		out = binary.BigEndian.AppendUint32(out, uint32(8+len(payload)))
		return append(out, payload...)
	}
	return element("icns", element("icp4", frame))
}

func TestCheckPassesOurOwnOutput(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"icns", encoded(t, func(b *bytes.Buffer) error { return icns.Encode(b, art(1024)) })},
		{"ico", encoded(t, func(b *bytes.Buffer) error { return ico.Encode(b, art(256)) })},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := check("icon."+tt.name, bytes.NewReader(tt.data)); err != nil {
				t.Errorf("check reported %v", err)
			}
		})
	}
}

func TestCheckFailsOnFindingsThatShow(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"icon.ico", opaqueICO(t)},
		{"icon.icns", bundleICNS(t)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := check(tt.name, bytes.NewReader(tt.data))
			if err == nil {
				t.Fatal("check reported nothing")
			}
			if !strings.Contains(err.Error(), "change what is drawn") {
				t.Errorf("error is %v, want it to count the findings", err)
			}
			if !strings.Contains(err.Error(), tt.name) {
				t.Errorf("error does not name the file: %v", err)
			}
		})
	}
}

func TestCheckRejectsWhatIsNotAnIcon(t *testing.T) {
	plain := encoded(t, func(b *bytes.Buffer) error { return png.Encode(b, art(64)) })
	err := check("art.png", bytes.NewReader(plain))
	if err == nil || !strings.Contains(err.Error(), "not an icns, ico or Windows binary") {
		t.Errorf("check returned %v, want it to refuse a plain image", err)
	}
}

// TestContainerReadsTheBytesFirst keeps a misnamed file from being validated
// against the wrong format.
func TestContainerReadsTheBytesFirst(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
		ext  string
		want string
	}{
		{"icns named ico", encoded(t, func(b *bytes.Buffer) error { return icns.Encode(b, art(64)) }), ".ico", ".icns"},
		{"ico named icns", encoded(t, func(b *bytes.Buffer) error { return ico.Encode(b, art(64)) }), ".icns", ".ico"},
		{"unknown bytes, known extension", []byte("rubbish"), ".icns", ".icns"},
		{"unknown bytes, plain extension", []byte("rubbish"), ".png", ""},
		{"a binary named exe", []byte("MZ\x90\x00"), ".exe", ".exe"},
		{"a binary named dll", []byte("MZ\x90\x00"), ".dll", ".dll"},
		{"a binary named nothing", []byte("MZ\x90\x00"), "", ".exe"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := container(tt.data, tt.ext); got != tt.want {
				t.Errorf("container = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCheckReadsABinary uses the exe package's fixture, a DLL whose resource
// section mingw laid out, rather than building another one here.
func TestCheckReadsABinary(t *testing.T) {
	const fixture = "../../exe/testdata/icon.dll"
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	// The fixture holds 32, 24 and 16, so the only finding is the advice
	// that larger sizes are absent.
	if err := check("icon.dll", bytes.NewReader(data)); err != nil {
		t.Errorf("check reported %v", err)
	}
}

func TestCheckRefusesABinaryWithoutIcons(t *testing.T) {
	data, err := os.ReadFile("../../exe/testdata/plain.dll")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	err = check("plain.dll", bytes.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "no icons found") {
		t.Errorf("check returned %v, want it to report no icons", err)
	}
}

func TestNameLabelsAPipe(t *testing.T) {
	if got := name(""); got != "stdin" {
		t.Errorf("name(\"\") = %q, want stdin", got)
	}
	if got := name("/tmp/art/icon.icns"); got != "icon.icns" {
		t.Errorf("name = %q, want the base name", got)
	}
}
