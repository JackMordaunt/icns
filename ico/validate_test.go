package ico

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// frame is one icon to place in a file the encoder would not write itself.
type frame struct {
	width, height int
	data          []byte
}

// buildICO assembles a file from payloads already encoded, so a test can
// store something invalid on purpose.
func buildICO(frames ...frame) []byte {
	out := make([]byte, 0, directorySize+entrySize*len(frames))
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, uint16(len(frames)))
	offset := directorySize + entrySize*len(frames)
	for _, f := range frames {
		out = append(out, byte(f.width), byte(f.height), 0, 0)
		out = binary.LittleEndian.AppendUint16(out, 1)
		out = binary.LittleEndian.AppendUint16(out, 32)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(f.data)))
		out = binary.LittleEndian.AppendUint32(out, uint32(offset))
		offset += len(f.data)
	}
	for _, f := range frames {
		out = append(out, f.data...)
	}
	return out
}

// encodePNG encodes img the way a program that does not know what Windows
// requires would: through image/png, which drops the alpha channel from an
// image that is entirely opaque.
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding png: %v", err)
	}
	return buf.Bytes()
}

func validate(t *testing.T, data []byte) []Problem {
	t.Helper()
	problems, err := Validate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	return problems
}

// find returns the first problem whose message contains want.
func find(problems []Problem, want string) (Problem, bool) {
	for _, p := range problems {
		if strings.Contains(p.Message, want) {
			return p, true
		}
	}
	return Problem{}, false
}

func TestValidateAcceptsOurOwnOutput(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, gradient(256)); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if problems := validate(t, buf.Bytes()); len(problems) != 0 {
		for _, p := range problems {
			t.Errorf("unexpected problem: %s", p)
		}
	}
}

// TestValidateAcceptsAnOpaqueSource covers the case the encoder guards
// against: an image with no transparency still has to reach Windows with an
// alpha channel.
func TestValidateAcceptsAnOpaqueSource(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, solid(256, color.NRGBA{R: 1, G: 2, B: 3, A: 255})); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if problems := validate(t, buf.Bytes()); len(problems) != 0 {
		for _, p := range problems {
			t.Errorf("unexpected problem: %s", p)
		}
	}
}

func TestValidateReportsAPNGWithoutAlpha(t *testing.T) {
	// image/png writes truecolour for an image that is entirely opaque,
	// which is the frame Windows passes over.
	opaque := encodePNG(t, solid(256, color.NRGBA{R: 1, G: 2, B: 3, A: 255}))
	if _, _, colour, ok := ihdr(opaque); !ok || colour != 2 {
		t.Fatalf("fixture is colour type %d, want truecolour", colour)
	}
	problems := validate(t, buildICO(frame{width: 0, height: 0, data: opaque}))
	p, ok := find(problems, "no alpha channel")
	if !ok {
		t.Fatalf("no problem reported, got %v", problems)
	}
	if p.Severity != Invisible {
		t.Errorf("severity is %s, want %s", p.Severity, Invisible)
	}
	if !strings.Contains(p.Icon, "256x256") {
		t.Errorf("problem names %q, want the 256 pixel icon", p.Icon)
	}
}

func TestValidateAcceptsAPNGWithAlpha(t *testing.T) {
	withA := encodePNG(t, withAlpha{solid(256, color.NRGBA{R: 1, G: 2, B: 3, A: 255})})
	if _, _, colour, ok := ihdr(withA); !ok || colour != 6 {
		t.Fatalf("fixture is colour type %d, want truecolour with alpha", colour)
	}
	problems := validate(t, buildICO(frame{width: 0, height: 0, data: withA}))
	if _, ok := find(problems, "alpha channel"); ok {
		t.Errorf("reported a problem for a frame that has one: %v", problems)
	}
}

func TestValidateReportsSizeDisagreement(t *testing.T) {
	// The directory says 64, the image is 32.
	small := encodePNG(t, withAlpha{gradient(32)})
	problems := validate(t, buildICO(frame{width: 64, height: 64, data: small}))
	p, ok := find(problems, "the directory lists it as 64x64")
	if !ok {
		t.Fatalf("no problem reported, got %v", problems)
	}
	if p.Severity != Degraded {
		t.Errorf("severity is %s, want %s", p.Severity, Degraded)
	}
}

func TestValidateReportsDuplicateSizes(t *testing.T) {
	one := encodePNG(t, withAlpha{gradient(32)})
	two := encodePNG(t, withAlpha{solid(32, color.NRGBA{A: 255})})
	problems := validate(t, buildICO(
		frame{width: 32, height: 32, data: one},
		frame{width: 32, height: 32, data: two},
	))
	p, ok := find(problems, "2 icons of 32x32")
	if !ok {
		t.Fatalf("no problem reported, got %v", problems)
	}
	if p.Severity != Degraded {
		t.Errorf("severity is %s, want %s", p.Severity, Degraded)
	}
}

func TestValidateReportsMissingSizes(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, gradient(16)); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	problems := validate(t, buf.Bytes())
	p, ok := find(problems, "holds no")
	if !ok {
		t.Fatalf("no problem reported, got %v", problems)
	}
	if p.Severity != Advice {
		t.Errorf("severity is %s, want %s", p.Severity, Advice)
	}
	for _, want := range []string{"32x32", "48x48", "256x256"} {
		if !strings.Contains(p.Message, want) {
			t.Errorf("message does not name %s: %s", want, p.Message)
		}
	}
}

func TestValidateReportsATruncatedBitmap(t *testing.T) {
	problems := validate(t, buildICO(frame{width: 32, height: 32, data: []byte{1, 2, 3}}))
	p, ok := find(problems, "too few for a bitmap header")
	if !ok {
		t.Fatalf("no problem reported, got %v", problems)
	}
	if p.Severity != Invisible {
		t.Errorf("severity is %s, want %s", p.Severity, Invisible)
	}
}

func TestValidateOrdersBySeverity(t *testing.T) {
	// One frame Windows passes over, at a size that leaves others missing.
	opaque := encodePNG(t, solid(256, color.NRGBA{R: 1, A: 255}))
	problems := validate(t, buildICO(frame{width: 0, height: 0, data: opaque}))
	if len(problems) < 2 {
		t.Fatalf("want several problems, got %v", problems)
	}
	for i := 1; i < len(problems); i++ {
		if problems[i-1].Severity > problems[i].Severity {
			t.Fatalf("problem %d is less serious than the one after it: %v", i-1, problems)
		}
	}
	if problems[0].Severity != Invisible {
		t.Errorf("first problem is %s, want %s", problems[0].Severity, Invisible)
	}
}

func TestValidateRejectsWhatItCannotParse(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("not an icon"), {0, 0, 1, 0, 0, 0}} {
		if _, err := Validate(bytes.NewReader(data)); err == nil {
			t.Errorf("validating %q returned no error", data)
		}
	}
}
