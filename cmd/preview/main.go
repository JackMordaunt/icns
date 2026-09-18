// Previwer GUI for `.icns` icons.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	l "gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	m "gioui.org/widget/material"
	c "gioui.org/x/component"
	"github.com/jackmordaunt/icns/v4"
	"github.com/ncruces/zenity"
)

// BUG(jfm): macOS file dialog returns "no such file or directory". Could be permissions issue.

func main() {
	ui := UI{
		Window:        new(app.Window),
		Th:            m.NewTheme(),
		ProcessedIcon: make(chan ProcessedIconResult, 1),
	}
	ui.Window.Option(app.Title("icnsify"), app.MinSize(unit.Dp(700), unit.Dp(250)))
	if len(os.Args) > 1 {
		if file := os.Args[1]; filepath.Ext(file) == ".icns" {
			ui.Load(func() (string, []image.Image, error) {
				imgs, err := LoadImage(file)
				return file, imgs, err
			})
		}
	}
	go func() {
		if err := ui.Loop(); err != nil {
			log.Fatalf("error: %v", err)
		}
		os.Exit(0)
	}()
	app.Main()
}

type (
	C = l.Context
	D = l.Dimensions
)

// thumbnail is one icon resolution in the sidebar.
type thumbnail struct {
	widget.Image
	Click widget.Clickable
}

// UI contains all state for the UI.
type UI struct {
	*app.Window
	Th *m.Theme

	// Preview points to the currently selected icon to render in the preview area.
	Preview *thumbnail
	// Icons contains all the different resolutions found in the icns file.
	Icons []*thumbnail
	// FileName is the name of the source icon file on disk.
	FileName string
	// Source is the original image data.
	Source image.Image

	OpenBtn widget.Clickable
	SideBar l.List

	ProcessedIcon chan ProcessedIconResult
}

type ProcessedIconResult struct {
	File string
	Imgs []image.Image
	Err  error
}

// Load runs work off the UI goroutine and wakes the window once it has a
// result to collect.
func (ui *UI) Load(work func() (string, []image.Image, error)) {
	go func() {
		file, imgs, err := work()
		ui.ProcessedIcon <- ProcessedIconResult{
			File: filepath.Base(file),
			Imgs: imgs,
			Err:  err,
		}
		ui.Window.Invalidate()
	}()
}

// Loop initializes UI state and starts the render loop.
func (ui *UI) Loop() error {
	var ops op.Ops
	for {
		switch event := ui.Window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, event)
			ui.Update(gtx)
			ui.Layout(gtx)
			event.Frame(gtx.Ops)
		}
	}
}

// Update the UI state.
func (ui *UI) Update(gtx C) {
	for {
		e, ok := gtx.Event(key.Filter{
			Focus:    ui,
			Name:     "S",
			Required: key.ModShortcut,
		})
		if !ok {
			break
		}
		k, ok := e.(key.Event)
		if !ok || k.State != key.Press || ui.Source == nil {
			continue
		}
		if err := ui.SaveAsPrompt(); err != nil {
			log.Printf("saving png as icns: %v", err)
		}
	}
	for _, icon := range ui.Icons {
		if icon.Click.Clicked(gtx) {
			ui.Preview = icon
		}
	}
	if ui.OpenBtn.Clicked(gtx) {
		ui.Load(func() (string, []image.Image, error) {
			file, err := zenity.SelectFile(zenity.Title("Select .icns file"))
			if err != nil {
				return "", nil, fmt.Errorf("selecting file: %w", err)
			}
			imgs, err := LoadImage(file)
			if err != nil {
				return "", nil, err
			}
			return file, imgs, nil
		})
	}
	select {
	case r := <-ui.ProcessedIcon:
		if r.Err != nil {
			// TODO(jfm): push to dismissable error stack.
			log.Printf("loading icns file: %v", r.Err)
			break
		}
		ui.Icons = ui.Icons[:0]
		for _, img := range r.Imgs {
			ui.Icons = append(ui.Icons, &thumbnail{
				Image: widget.Image{
					Src:      paint.NewImageOp(img),
					Fit:      widget.Contain,
					Position: l.Center,
				},
			})
		}
		ui.Preview = nil
		if len(ui.Icons) > 0 {
			ui.Source = r.Imgs[0]
			ui.Preview = ui.Icons[0]
		}
		ui.FileName = r.File
	default:
	}
}

// SaveAsPrompt asks for a destination and writes the previewed icon to it.
func (ui *UI) SaveAsPrompt() error {
	file, err := zenity.SelectFileSave(
		zenity.Title("Save as icns"),
		zenity.Filename(UseExt(ui.FileName, ".icns")))
	if err != nil {
		return fmt.Errorf("selecting file: %w", err)
	}
	if err := ui.SaveAs(file); err != nil {
		return fmt.Errorf("saving to icns: %w", err)
	}
	return nil
}

// Layout the UI.
func (ui *UI) Layout(gtx C) D {
	ui.SideBar.Axis = l.Vertical
	// The window itself takes the keyboard, for the save shortcut.
	event.Op(gtx.Ops, ui)
	gtx.Execute(key.FocusCmd{Tag: ui})
	return l.Flex{
		Axis: l.Horizontal,
	}.Layout(
		gtx,
		l.Rigid(func(gtx C) D { return ui.LayoutSideBar(gtx) }),
		l.Flexed(1, func(gtx C) D { return ui.LayoutPreviewArea(gtx) }),
	)
}

var (
	// ThumbnailWidth specifies how wide the sidebar thumbnails should be.
	ThumbnailWidth = unit.Dp(125)
	// SelectedHighlight specifies the color to render behind the selected thumbnail.
	SelectedHighlight = color.NRGBA{A: 50}
)

// LayoutSideBar displays a sidebar which contains a list of thumbnails for the various icns
// resolutions.
func (ui *UI) LayoutSideBar(gtx C) D {
	return l.Flex{
		Axis:      l.Vertical,
		Alignment: l.Middle,
	}.Layout(
		gtx,
		l.Rigid(func(gtx C) D {
			return l.UniformInset(unit.Dp(5)).Layout(gtx, func(gtx C) D {
				return m.Label(ui.Th, unit.Sp(15), ui.FileName).Layout(gtx)
			})
		}),
		l.Flexed(1, func(gtx C) D {
			return ui.SideBar.Layout(gtx, len(ui.Icons), func(gtx C, ii int) D {
				return l.UniformInset(unit.Dp(15)).Layout(gtx, func(gtx C) D {
					cs := &gtx.Constraints
					cs.Max.X = gtx.Dp(ThumbnailWidth)
					return ui.LayoutThumbnail(gtx, ii)
				})
			})
		}),
	)
}

// LayoutPreviewArea displays the selected icon resultion scaled to the size of the area.
func (ui *UI) LayoutPreviewArea(gtx C) D {
	return l.Center.Layout(gtx, func(gtx C) D {
		if ui.Preview == nil {
			btn := m.Button(ui.Th, &ui.OpenBtn, "Open")
			btn.TextSize = unit.Sp(25)
			return btn.Layout(gtx)
		}
		return ui.Preview.Image.Layout(gtx)
	})
}

// LayoutThumbnail displays a specific icon thumbnail.
func (ui *UI) LayoutThumbnail(gtx C, ii int) D {
	icon := ui.Icons[ii]
	return icon.Click.Layout(gtx, func(gtx C) D {
		return l.Stack{}.Layout(
			gtx,
			l.Stacked(func(gtx C) D {
				return l.Flex{
					Axis:      l.Vertical,
					Alignment: l.Middle,
				}.Layout(
					gtx,
					l.Rigid(func(gtx C) D {
						return icon.Image.Layout(gtx)
					}),
					l.Rigid(func(gtx C) D {
						return m.Label(ui.Th, unit.Sp(15), strconv.Itoa(ii+1)).
							Layout(gtx)
					}),
				)
			}),
			l.Expanded(func(gtx C) D {
				if ui.Preview != icon {
					return D{}
				}
				return c.Rect{
					Size:  gtx.Constraints.Min,
					Color: SelectedHighlight,
					Radii: 4,
				}.Layout(gtx)
			}),
		)
	})
}

// SaveAs saves the previewed image as an icns icon at the path specified.
func (ui *UI) SaveAs(path string) error {
	if ui.Source == nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()
	if err := icns.Encode(f, ui.Source); err != nil {
		return fmt.Errorf("encoding icns: %w", err)
	}
	return nil
}

// LoadImage will load all icons from an icns file, or generate them from a png file.
func LoadImage(path string) ([]image.Image, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving file path: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	switch filepath.Ext(path) {
	case ".icns":
		imgs, err := icns.DecodeAll(f)
		if err != nil {
			return nil, fmt.Errorf("decoding icns: %w", err)
		}
		return imgs, nil
	case ".png":
		img, err := png.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decoding png: %w", err)
		}
		return []image.Image{img}, nil
	}
	return nil, nil
}

// UseExt replaces any existing file extension with the provided one.
func UseExt(s, ext string) string {
	return strings.TrimSuffix(s, filepath.Ext(s)) + ext
}
