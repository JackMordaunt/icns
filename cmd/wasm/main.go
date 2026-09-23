//go:build js && wasm

// Command wasm exposes the icon encoders and decoders to a web page as the
// global "icns", so a conversion runs in the browser rather than on a
// server. See web/ for a page that uses it.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"syscall/js"

	"github.com/jackmordaunt/icns/v4"
	"github.com/jackmordaunt/icns/v4/ico"
)

// jpegQuality is the quality JPEG is written at: the maximum, so a lossy
// format costs the artwork as little as it can.
const jpegQuality = 100

func main() {
	api := js.Global().Get("Object").New()
	api.Set("convert", js.FuncOf(convert))
	api.Set("inspect", js.FuncOf(inspect))
	api.Set("validate", js.FuncOf(validate))
	js.Global().Set("icns", api)
	// The page calls into this, so the program has to stay resident.
	select {}
}

// convert takes the bytes of an image and the name of a format, and returns
// the same artwork written in that format.
func convert(_ js.Value, args []js.Value) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = failure(fmt.Errorf("converting: %v", r))
		}
	}()
	if len(args) < 2 {
		return failure(errors.New("convert wants an image and a format"))
	}
	img, _, err := image.Decode(bytes.NewReader(toGo(args[0])))
	if err != nil {
		return failure(fmt.Errorf("reading the image: %w", err))
	}
	buf := bytes.NewBuffer(nil)
	switch format := strings.ToLower(args[1].String()); format {
	case "icns":
		err = icns.Encode(buf, img)
	case "ico":
		err = ico.Encode(buf, img)
	case "png":
		err = png.Encode(buf, img)
	case "jpg", "jpeg":
		err = jpeg.Encode(buf, img, &jpeg.Options{Quality: jpegQuality})
	default:
		err = fmt.Errorf("cannot write %s", format)
	}
	if err != nil {
		return failure(err)
	}
	return success(buf.Bytes())
}

// inspect lists the icons an icns or ico file holds, largest first. Anything
// else holds one image and lists as empty.
func inspect(_ js.Value, args []js.Value) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = failure(fmt.Errorf("inspecting: %v", r))
		}
	}()
	if len(args) < 1 {
		return failure(errors.New("inspect wants an image"))
	}
	var (
		src   = toGo(args[0])
		icons []string
	)
	if d, err := icns.NewDecoder(bytes.NewReader(src)); err == nil {
		for _, icon := range d.Icons() {
			icons = append(icons, icon.String())
		}
	} else if d, err := ico.NewDecoder(bytes.NewReader(src)); err == nil {
		for _, icon := range d.Icons() {
			icons = append(icons, icon.String())
		}
	}
	list := js.Global().Get("Array").New(len(icons))
	for i, icon := range icons {
		list.SetIndex(i, icon)
	}
	result := js.Global().Get("Object").New()
	result.Set("ok", true)
	result.Set("icons", list)
	return result
}

// validate reports what the platform that owns the format will make of an
// icon file, as an array of findings ordered by severity.
func validate(_ js.Value, args []js.Value) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = failure(fmt.Errorf("validating: %v", r))
		}
	}()
	if len(args) < 1 {
		return failure(errors.New("validate wants an icon file"))
	}
	type finding struct{ severity, icon, message string }
	var (
		src      = toGo(args[0])
		findings []finding
	)
	switch {
	case bytes.HasPrefix(src, []byte(icns.Magic)):
		problems, err := icns.Validate(bytes.NewReader(src))
		if err != nil {
			return failure(err)
		}
		for _, p := range problems {
			findings = append(findings, finding{p.Severity.String(), p.Icon, p.Message})
		}
	case bytes.HasPrefix(src, []byte(ico.Magic)):
		problems, err := ico.Validate(bytes.NewReader(src))
		if err != nil {
			return failure(err)
		}
		for _, p := range problems {
			findings = append(findings, finding{p.Severity.String(), p.Icon, p.Message})
		}
	default:
		return failure(errors.New("not an icns or ico file"))
	}
	list := js.Global().Get("Array").New(len(findings))
	for i, f := range findings {
		item := js.Global().Get("Object").New()
		item.Set("severity", f.severity)
		item.Set("icon", f.icon)
		item.Set("message", f.message)
		list.SetIndex(i, item)
	}
	result := js.Global().Get("Object").New()
	result.Set("ok", true)
	result.Set("problems", list)
	return result
}

// toGo copies a Uint8Array into Go memory, which the wasm boundary requires.
func toGo(v js.Value) []byte {
	out := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(out, v)
	return out
}

// success and failure build the object every entry point returns, so a
// failure reaches the page as a value rather than as a panic crossing the
// boundary.
func success(data []byte) js.Value {
	buf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(buf, data)
	out := js.Global().Get("Object").New()
	out.Set("ok", true)
	out.Set("data", buf)
	return out
}

func failure(err error) js.Value {
	out := js.Global().Get("Object").New()
	out.Set("ok", false)
	out.Set("error", err.Error())
	return out
}
