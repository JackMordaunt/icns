//go:build darwin

package icns

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

// These tests hold the package against iconutil, the tool Apple ships, in
// both directions: a file it wrote has to read back as the artwork that went
// in, and a file this package wrote has to come apart under its hands.

// iconutil runs the tool, failing the test with whatever it printed.
func iconutil(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("iconutil", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("iconutil %v: %v\n%s", args, err, out)
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func readPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	return img
}

// oracleArtwork paints one flat colour per slot, so a colour identifies the
// file it came from wherever it ends up.
func oracleArtwork() (map[Slot]image.Image, map[Slot]color.NRGBA) {
	var (
		images = map[Slot]image.Image{}
		want   = map[Slot]color.NRGBA{}
	)
	for i, slot := range Slots() {
		c := color.NRGBA{R: uint8(10 + i*20), G: uint8(200 - i*15), B: 0x40, A: 0xFF}
		images[slot] = solid(int(slot.Pixels()), c)
		want[slot] = c
	}
	return images, want
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// TestIconutilWrites checks that a file built by Apple's tool reads back as
// the artwork that went into it, element by element.
func TestIconutilWrites(t *testing.T) {
	dir := t.TempDir()
	set := filepath.Join(dir, "Oracle.iconset")
	if err := os.MkdirAll(set, 0o755); err != nil {
		t.Fatal(err)
	}
	images, want := oracleArtwork()
	for slot, img := range images {
		writePNG(t, filepath.Join(set, "icon_"+slot.String()+".png"), img)
	}
	out := filepath.Join(dir, "apple.icns")
	iconutil(t, "-c", "icns", "-o", out, set)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decoding what iconutil wrote: %v", err)
	}
	icons := d.Icons()
	t.Logf("iconutil wrote %d icons this package reads", len(icons))
	if len(icons) == 0 {
		t.Fatal("no icons were read back")
	}
	for _, icon := range icons {
		slot, ok := slotOf[icon.ID]
		if !ok {
			t.Errorf("iconutil wrote %s, which this package does not place in a slot", icon.ID)
			continue
		}
		img, err := icon.Decode()
		if err != nil {
			t.Errorf("%s: %v", icon.ID, err)
			continue
		}
		if got := uint(img.Bounds().Dx()); got != slot.Pixels() {
			t.Errorf("%s is %d pixels, want %d", icon.ID, got, slot.Pixels())
		}
		if got := centre(img); got != want[slot] {
			t.Errorf("%s holds %v, want %v from slot %s", icon.ID, got, want[slot], slot)
		}
	}
}

// TestIconutilReads checks that Apple's tool takes a file this package wrote
// apart into the artwork that went in, which covers the run-length encoded
// elements and the table of contents.
func TestIconutilReads(t *testing.T) {
	dir := t.TempDir()
	images, want := oracleArtwork()
	buf := bytes.NewBuffer(nil)
	if err := NewEncoder(buf).EncodeSlots(images); err != nil {
		t.Fatal(err)
	}
	ours := filepath.Join(dir, "ours.icns")
	if err := os.WriteFile(ours, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	set := filepath.Join(dir, "ours.iconset")
	iconutil(t, "-c", "iconset", "-o", set, ours)

	t.Logf("iconutil extracted %v", listDir(t, set))
	for slot, c := range want {
		path := filepath.Join(set, "icon_"+slot.String()+".png")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("iconutil did not extract slot %s", slot)
			continue
		}
		img := readPNG(t, path)
		if got := uint(img.Bounds().Dx()); got != slot.Pixels() {
			t.Errorf("slot %s came out %d pixels, want %d", slot, got, slot.Pixels())
		}
		if got := centre(img); got != c {
			t.Errorf("slot %s came out %v, want %v", slot, got, c)
		}
	}
}
