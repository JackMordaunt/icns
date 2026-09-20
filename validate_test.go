package icns

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
)

func validate(t *testing.T, data []byte) []Problem {
	t.Helper()
	problems, err := Validate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	return problems
}

// found returns the first problem whose message contains want.
func found(problems []Problem, want string) (Problem, bool) {
	for _, p := range problems {
		if strings.Contains(p.Message, want) {
			return p, true
		}
	}
	return Problem{}, false
}

// flat returns an image of one colour, which the run-length encoder
// compresses rather than storing at its exact length.
func flat(side int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 0x20, 0x40, 0x60, 0xFF
	}
	return img
}

// pngOf encodes an image of a given side, so an element can be given a
// payload of a size its type does not name.
func pngOf(t testing.TB, side int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, flat(side)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestValidateAcceptsOurOwnOutput(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, gradient(1024)); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if problems := validate(t, buf.Bytes()); len(problems) != 0 {
		for _, p := range problems {
			t.Errorf("unexpected problem: %s", p)
		}
	}
}

// TestValidateReportsUnpaddedPlanes covers the reason padRLE exists: without
// the trailing byte, Apple silicon drops the last run.
func TestValidateReportsUnpaddedPlanes(t *testing.T) {
	planes, mask := splitPlanes(flat(32), 32)
	packed := packRLE(planes)
	if len(packed) == len(planes) {
		t.Fatal("fixture did not compress, so there is no run to drop")
	}
	data := file(
		encodeElement("il32", packed),
		encodeElement("l8mk", mask),
	)
	p, ok := found(validate(t, data), "Apple silicon drops")
	if !ok {
		t.Fatalf("no problem reported, got %v", validate(t, data))
	}
	if p.Severity != Degraded {
		t.Errorf("severity is %s, want %s", p.Severity, Degraded)
	}
}

func TestValidateAcceptsPaddedPlanes(t *testing.T) {
	planes, mask := splitPlanes(flat(32), 32)
	data := file(
		encodeElement("il32", padRLE(packRLE(planes), len(planes))),
		encodeElement("l8mk", mask),
	)
	if _, ok := found(validate(t, data), "Apple silicon drops"); ok {
		t.Errorf("reported padding that is present: %v", validate(t, data))
	}
}

// TestValidateAcceptsUncompressedPlanes covers the other half of the rule:
// planes stored at their exact length hold no run to drop.
func TestValidateAcceptsUncompressedPlanes(t *testing.T) {
	// No three bytes in a row are equal, so every run is a literal and the
	// encoder stores the planes as they are.
	planes := make([]byte, 32*32*3)
	for i := range planes {
		planes[i] = byte(i % 251)
	}
	mask := make([]byte, 32*32)
	packed := padRLE(packRLE(planes), len(planes))
	if len(packed) != len(planes) {
		t.Fatalf("fixture compressed to %d of %d bytes, so it is not stored", len(packed), len(planes))
	}
	data := file(
		encodeElement("il32", packed),
		encodeElement("l8mk", mask),
	)
	if _, ok := found(validate(t, data), "Apple silicon drops"); ok {
		t.Errorf("reported padding for uncompressed planes: %v", validate(t, data))
	}
}

// TestValidateLeavesARGBAlone records why the padding check covers only the
// three plane types. actool on macOS 26 compiled an icon into an icns whose
// ic04 is ARGB with its stream ending exactly on the last run, so warning
// about that shape would be accusing Apple's own output.
func TestValidateLeavesARGBAlone(t *testing.T) {
	const side = 16
	planes := make([]byte, side*side*4)
	for i := range planes {
		planes[i] = byte(0x40 + i/(side*side))
	}
	packed := packRLE(planes)
	if len(packed) == len(planes) {
		t.Fatal("fixture did not compress, so there is no run to end on")
	}
	data := file(encodeElement("ic04", append([]byte("ARGB"), packed...)))
	problems := validate(t, data)
	if _, ok := found(problems, "Apple silicon drops"); ok {
		t.Errorf("reported padding for an ARGB icon: %v", problems)
	}
}

func TestValidateReportsAMissingMask(t *testing.T) {
	planes, _ := splitPlanes(flat(32), 32)
	data := file(encodeElement("il32", padRLE(packRLE(planes), len(planes))))
	p, ok := found(validate(t, data), "draws fully opaque")
	if !ok {
		t.Fatalf("no problem reported, got %v", validate(t, data))
	}
	if p.Severity != Degraded {
		t.Errorf("severity is %s, want %s", p.Severity, Degraded)
	}
	if !strings.Contains(p.Message, "l8mk") {
		t.Errorf("message does not name the mask element: %s", p.Message)
	}
}

func TestValidateReportsIcp4WithoutIs32(t *testing.T) {
	data := file(encodeElement("icp4", pngOf(t, 16)))
	p, ok := found(validate(t, data), "does not render from an app bundle")
	if !ok {
		t.Fatalf("no problem reported, got %v", validate(t, data))
	}
	if p.Severity != Degraded {
		t.Errorf("severity is %s, want %s", p.Severity, Degraded)
	}
	if !strings.Contains(p.Icon, "icp4") {
		t.Errorf("problem names %q, want icp4", p.Icon)
	}
}

func TestValidateAcceptsIcp4BesideIs32(t *testing.T) {
	planes, mask := splitPlanes(flat(16), 16)
	data := file(
		encodeElement("icp4", pngOf(t, 16)),
		encodeElement("is32", padRLE(packRLE(planes), len(planes))),
		encodeElement("s8mk", mask),
	)
	if _, ok := found(validate(t, data), "does not render from an app bundle"); ok {
		t.Errorf("reported a bundle problem for a file that holds is32: %v", validate(t, data))
	}
}

func TestValidateReportsSizeDisagreement(t *testing.T) {
	// ic07 names 128 pixels; the payload holds 64.
	data := file(encodeElement("ic07", pngOf(t, 64)))
	p, ok := found(validate(t, data), "where the type is 128x128")
	if !ok {
		t.Fatalf("no problem reported, got %v", validate(t, data))
	}
	if p.Severity != Degraded {
		t.Errorf("severity is %s, want %s", p.Severity, Degraded)
	}
}

func TestValidateReportsJPEG2000AsAdvice(t *testing.T) {
	data := file(
		encodeElement("ic10", jpeg2000header),
		encodeElement("ic07", pngOf(t, 128)),
	)
	p, ok := found(validate(t, data), "JPEG 2000")
	if !ok {
		t.Fatalf("no problem reported, got %v", validate(t, data))
	}
	if p.Severity != Advice {
		t.Errorf("severity is %s, want %s", p.Severity, Advice)
	}
}

func TestValidateReportsGapsBelowTheLargest(t *testing.T) {
	data := file(
		encodeElement("ic09", pngOf(t, 512)),
		encodeElement("ic07", pngOf(t, 128)),
	)
	p, ok := found(validate(t, data), "holds no")
	if !ok {
		t.Fatalf("no problem reported, got %v", validate(t, data))
	}
	if p.Severity != Advice {
		t.Errorf("severity is %s, want %s", p.Severity, Advice)
	}
	for _, want := range []string{"ic13", "ic08", "ic12", "ic11", "il32", "is32"} {
		if !strings.Contains(p.Message, want) {
			t.Errorf("message does not name %s: %s", want, p.Message)
		}
	}
	// ic10 is 1024, above the largest icon present, so the artwork simply
	// ran out and it is not reported.
	if strings.Contains(p.Message, "ic10") {
		t.Errorf("message names a size above the largest held: %s", p.Message)
	}
}

func TestValidateReportsAnUndecodableIcon(t *testing.T) {
	// An element whose type stores colour planes, holding too little to
	// expand.
	data := file(
		encodeElement("il32", []byte{0xFF, 0x01}),
		encodeElement("l8mk", make([]byte, 32*32)),
	)
	p, ok := found(validate(t, data), "cannot be decoded")
	if !ok {
		t.Fatalf("no problem reported, got %v", validate(t, data))
	}
	if p.Severity != Invisible {
		t.Errorf("severity is %s, want %s", p.Severity, Invisible)
	}
}

func TestValidateOrdersBySeverity(t *testing.T) {
	planes, _ := splitPlanes(flat(32), 32)
	data := file(
		encodeElement("il32", padRLE(packRLE(planes), len(planes))),
		encodeElement("ic10", jpeg2000header),
	)
	problems := validate(t, data)
	if len(problems) < 2 {
		t.Fatalf("want several problems, got %v", problems)
	}
	for i := 1; i < len(problems); i++ {
		if problems[i-1].Severity > problems[i].Severity {
			t.Fatalf("problem %d is less serious than the one after it: %v", i-1, problems)
		}
	}
}

func TestValidateRejectsWhatItCannotParse(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("not an icns"), file()} {
		if _, err := Validate(bytes.NewReader(data)); err == nil {
			t.Errorf("validating %q returned no error", data)
		}
	}
}

// TestValidateIgnoresColourOfMask keeps the mask elements from being read as
// icons in their own right.
func TestValidateIgnoresColourOfMask(t *testing.T) {
	planes, mask := splitPlanes(flat(16), 16)
	data := file(
		encodeElement("is32", padRLE(packRLE(planes), len(planes))),
		encodeElement("s8mk", mask),
	)
	for _, p := range validate(t, data) {
		if strings.Contains(p.Icon, "s8mk") {
			t.Errorf("reported the mask as an icon: %s", p)
		}
	}
}
