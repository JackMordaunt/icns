package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/gpu/headless"
	l "gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	m "gioui.org/widget/material"
	"github.com/jackmordaunt/icns/v4"
)

// iconColour is what the test icon is painted, chosen so it cannot be
// confused with the theme's greys.
var iconColour = color.NRGBA{R: 0xFF, G: 0x20, B: 0x20, A: 0xFF}

// testIcons encodes a flat coloured icns and decodes it back, the same route
// the window takes when it opens a file.
func testIcons(t *testing.T) []image.Image {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i] = iconColour.R
		src.Pix[i+1] = iconColour.G
		src.Pix[i+2] = iconColour.B
		src.Pix[i+3] = iconColour.A
	}
	var buf bytes.Buffer
	if err := icns.Encode(&buf, src); err != nil {
		t.Fatalf("encoding test icns: %v", err)
	}
	imgs, err := icns.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("decoding test icns: %v", err)
	}
	return imgs
}

const (
	frameWidth  = 800
	frameHeight = 600
)

// render lays the window out and rasterises it offscreen, returning how many
// pixels of the icon's colour reached the preview area on the right.
func render(t *testing.T, window *headless.Window, ui *UI) (int, *image.RGBA) {
	t.Helper()
	var ops op.Ops
	gtx := l.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: l.Exact(image.Pt(frameWidth, frameHeight)),
	}
	ui.Update(gtx)
	ui.Layout(gtx)
	if err := window.Frame(gtx.Ops); err != nil {
		t.Fatalf("rendering frame: %v", err)
	}
	shot := image.NewRGBA(image.Rect(0, 0, frameWidth, frameHeight))
	if err := window.Screenshot(shot); err != nil {
		t.Fatalf("reading frame: %v", err)
	}
	var painted int
	for y := 0; y < frameHeight; y++ {
		for x := frameWidth / 2; x < frameWidth; x++ {
			c := color.NRGBAModel.Convert(shot.At(x, y)).(color.NRGBA)
			if c.R > 0xC0 && c.G < 0x60 && c.B < 0x60 && c.A > 0x80 {
				painted++
			}
		}
	}
	return painted, shot
}

// TestRender rasterises the window offscreen, which is the only way to tell
// that the event and input plumbing still produces a frame.
func TestRender(t *testing.T) {
	window, err := headless.NewWindow(frameWidth, frameHeight)
	if err != nil {
		t.Skipf("headless rendering unavailable: %v", err)
	}
	defer window.Release()

	// An empty window shows the open button and none of the icon's colour,
	// which is what makes the count below mean something.
	empty := &UI{Th: m.NewTheme(), ProcessedIcon: make(chan ProcessedIconResult, 1)}
	if painted, shot := render(t, window, empty); painted > 100 {
		t.Errorf("an empty window drew %d pixels of the icon's colour", painted)
		if path := keep(t, shot); path != "" {
			t.Logf("frame written to %s", path)
		}
	}

	ui := &UI{Th: m.NewTheme(), ProcessedIcon: make(chan ProcessedIconResult, 1)}
	// Hand the window a loaded file the way the loader goroutine does, so
	// Update drains it into thumbnails.
	ui.ProcessedIcon <- ProcessedIconResult{File: "test.icns", Imgs: testIcons(t)}
	painted, shot := render(t, window, ui)
	if len(ui.Icons) == 0 {
		t.Fatal("the loaded icons did not reach the window")
	}
	if ui.Preview == nil {
		t.Fatal("no icon was selected for the preview area")
	}
	t.Logf("icon pixels in the preview area: %d", painted)
	if painted < 1000 {
		t.Errorf("only %d pixels of the icon were drawn in the preview area", painted)
		if path := keep(t, shot); path != "" {
			t.Logf("frame written to %s", path)
		}
	}
}

// keep writes the frame out so a failure can be looked at.
func keep(t *testing.T, img image.Image) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frame.png")
	f, err := os.Create(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return ""
	}
	return path
}
