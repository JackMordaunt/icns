// Previwer GUI for `.icns` icons.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	l "gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	m "gioui.org/widget/material"
	c "gioui.org/x/component"
	"github.com/jackmordaunt/icns"
	"github.com/ncruces/zenity"
)

func main() {
	go func() {
		ui := UI{
			Window: app.NewWindow(app.Title("icnsify"), app.MinSize(unit.Dp(700), unit.Dp(250))),
			Th:     m.NewTheme(gofont.Collection()),
		}
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

// UI contains all state for the UI.
type UI struct {
	*app.Window
	Th *m.Theme

	// Preview points to the currently selected icon to render in the preview area.
	Preview *widget.Image
	// Icons contains all the different resolutions found in the icns file.
	Icons []widget.Image
	// FileName is the name of the source icon file on disk.
	FileName string

	OpenBtn widget.Clickable
	SideBar layout.List
}

// Loop initializes UI state and starts the render loop.
func (ui *UI) Loop() error {
	var (
		ops    op.Ops
		events = ui.Window.Events()
	)
	for event := range events {
		switch event := (event).(type) {
		case system.DestroyEvent:
			return event.Err
		case system.FrameEvent:
			gtx := l.NewContext(&ops, event)
			ui.Update(gtx)
			ui.Layout(gtx)
			event.Frame(gtx.Ops)
		}
	}
	return nil
}

// Update the UI state.
func (ui *UI) Update(gtx C) {
	ui.SideBar.Axis = l.Vertical
	if ui.OpenBtn.Clicked() {
		if err := func() error {
			file, err := zenity.SelectFile()
			if err != nil {
				return fmt.Errorf("selecting file: %w", err)
			}
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			imgs, err := icns.DecodeAll(f)
			if err != nil {
				return fmt.Errorf("decoding: %w", err)
			}
			ui.Icons = ui.Icons[:]
			for _, img := range imgs {
				ui.Icons = append(ui.Icons, widget.Image{
					Src:      paint.NewImageOp(img),
					Fit:      widget.Contain,
					Position: l.Center,
				})
			}
			if len(ui.Icons) > 0 {
				ui.Preview = &ui.Icons[0]
			}
			ui.FileName = filepath.Base(file)
			return nil
		}(); err != nil {
			// TODO(jfm): Push to dismissable error stack.
			log.Printf("opening icns: %v", err)
		}
	}
	for ii := range ui.Icons {
		for _, event := range gtx.Events(ui.Icons[ii]) {
			if c, ok := event.(pointer.Event); ok && c.Type == pointer.Release {
				ui.Preview = &ui.Icons[ii]
			}
		}
	}
}

// Layout the UI.
func (ui *UI) Layout(gtx C) D {
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
				return m.Label(ui.Th, unit.Dp(15), ui.FileName).Layout(gtx)
			})
		}),
		l.Flexed(1, func(gtx C) D {
			return ui.SideBar.Layout(gtx, len(ui.Icons), func(gtx C, ii int) D {
				return l.UniformInset(unit.Dp(15)).Layout(gtx, func(gtx C) D {
					cs := &gtx.Constraints
					cs.Max.X = gtx.Px(ThumbnailWidth)
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
			btn.TextSize = unit.Dp(25)
			return btn.Layout(gtx)
		}
		return ui.Preview.Layout(gtx)
	})
}

// LayoutThumbnail displays a specific icon thumbnail.
func (ui *UI) LayoutThumbnail(gtx C, ii int) D {
	return l.Stack{}.Layout(
		gtx,
		l.Stacked(func(gtx C) D {
			return l.Flex{
				Axis:      l.Vertical,
				Alignment: l.Middle,
			}.Layout(
				gtx,
				l.Rigid(func(gtx C) D {
					return ui.Icons[ii].Layout(gtx)
				}),
				l.Rigid(func(gtx C) D {
					return m.Label(ui.Th, unit.Dp(15), strconv.Itoa(ii+1)).
						Layout(gtx)
				}),
			)
		}),
		l.Expanded(func(gtx C) D {
			if ui.Icons[ii] == *ui.Preview {
				return c.Rect{
					Size:  gtx.Constraints.Min,
					Color: SelectedHighlight,
					Radii: 4,
				}.Layout(gtx)
			}
			return D{}
		}),
		l.Expanded(func(gtx C) D {
			pointer.Rect(image.Rectangle{Max: gtx.Constraints.Min}).Add(gtx.Ops)
			pointer.InputOp{
				Tag:   ui.Icons[ii],
				Types: pointer.Release,
			}.Add(gtx.Ops)
			return D{}
		}),
	)
}
