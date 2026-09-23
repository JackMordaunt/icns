package icns

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Severity ranks what a Problem means for the icon macOS ends up drawing.
type Severity int

const (
	// Invisible means the icon cannot be read, so macOS draws another size.
	Invisible Severity = iota
	// Degraded means the icon is drawn, but not at the size, with the
	// transparency, or in the places it was meant to be.
	Degraded
	// Advice means nothing present is wrong, only that something usual is
	// absent.
	Advice
)

func (s Severity) String() string {
	switch s {
	case Invisible:
		return "invisible"
	case Degraded:
		return "degraded"
	case Advice:
		return "advice"
	}
	return fmt.Sprintf("unknown severity %d", s)
}

// Problem is one finding about an icns file.
type Problem struct {
	// Severity is what the finding means for the icon macOS draws.
	Severity Severity
	// Icon names the icon the finding concerns, such as "it32 128", and is
	// empty when the finding concerns the file as a whole.
	Icon string
	// Message describes what was found.
	Message string
}

func (p Problem) String() string {
	if p.Icon == "" {
		return fmt.Sprintf("%s: %s", p.Severity, p.Message)
	}
	return fmt.Sprintf("%s: %s: %s", p.Severity, p.Icon, p.Message)
}

// Validate reads an icns file and reports what macOS will make of it, most
// serious first. Data that cannot be parsed as an icns at all is returned as
// an error rather than as a Problem.
func Validate(r io.Reader) ([]Problem, error) {
	d, err := NewDecoder(r)
	if err != nil {
		return nil, err
	}
	icons := d.Icons()
	var problems []Problem
	for _, icon := range icons {
		problems = append(problems, icon.problems()...)
	}
	problems = append(problems, bundleProblems(icons)...)
	problems = append(problems, gaps(icons)...)
	slices.SortStableFunc(problems, func(a, b Problem) int {
		return cmp.Compare(a.Severity, b.Severity)
	})
	return problems, nil
}

// problems reports what the icon's own bytes say about it.
func (e Entry) problems() []Problem {
	// JPEG 2000 is read by macOS and by nothing else without a codec, so it
	// is reported rather than decoded.
	if e.ImageFormat == ImageFormatJPEG2000 {
		return []Problem{{
			Severity: Advice,
			Icon:     e.OsType.String(),
			Message:  "stored as JPEG 2000, which macOS reads and most other tools cannot",
		}}
	}
	var problems []Problem
	if e.enc == encodingRGB && e.mask == nil {
		problems = append(problems, Problem{
			Severity: Degraded,
			Icon:     e.OsType.String(),
			Message: fmt.Sprintf(
				"the file holds no %s element, so the icon draws fully opaque",
				e.OsType.mask,
			),
		})
	}
	problems = append(problems, e.paddingProblems()...)
	img, err := e.Decode()
	if err != nil {
		return append(problems, Problem{
			Severity: Invisible,
			Icon:     e.OsType.String(),
			Message:  fmt.Sprintf("cannot be decoded: %v", err),
		})
	}
	width, height := int(e.Size), int(e.Size)
	if e.height > 0 {
		height = int(e.height)
	}
	if size := img.Bounds().Size(); size.X != width || size.Y != height {
		problems = append(problems, Problem{
			Severity: Degraded,
			Icon:     e.OsType.String(),
			Message: fmt.Sprintf(
				"holds a %dx%d image, where the type is %dx%d",
				size.X, size.Y, width, height,
			),
		})
	}
	return problems
}

// paddingProblems reports a colour plane element whose stream ends on its
// last run. Apple's reader on Apple silicon drops the last value of such a
// stream, so a byte has to follow it for the icon to survive.
//
// Only the three plane types are read this way. The ARGB elements are left
// alone: actool on macOS 26 writes them with their stream ending exactly on
// the last run, so whatever drops a value does not reach them.
func (e Entry) paddingProblems() []Problem {
	if e.ImageFormat != ImageFormatRGB {
		return nil
	}
	var (
		pixels = int(e.Size) * int(e.Size)
		data   = e.data
		want   = pixels * rgbPlanes
	)
	// it32 is the one colour element that prefixes its planes with four zero
	// bytes.
	if e.ID == "it32" && len(data) >= it32PrefixSize && binary.BigEndian.Uint32(data[:it32PrefixSize]) == 0 {
		data = data[it32PrefixSize:]
	}
	// Data stored at its exact length is not compressed, so there is no run
	// for a reader to drop.
	if len(data) == want {
		return nil
	}
	_, used, err := unpackRLE(data, want)
	if err != nil || used < len(data) {
		return nil
	}
	return []Problem{{
		Severity: Degraded,
		Icon:     e.OsType.String(),
		Message:  "the compressed planes end on their last run, which Apple silicon drops, blanking the tail of the icon",
	}}
}

// bundleProblems reports the small PNG types that an app bundle does not
// render, when the file holds nothing else at their size.
func bundleProblems(icons []Entry) []Problem {
	held := func(id string) bool {
		return slices.ContainsFunc(icons, func(e Entry) bool { return e.ID == id })
	}
	var problems []Problem
	for _, pair := range []struct{ png, colour, mask string }{
		{png: "icp4", colour: "is32", mask: "s8mk"},
		{png: "icp5", colour: "il32", mask: "l8mk"},
	} {
		if !held(pair.png) || held(pair.colour) {
			continue
		}
		problems = append(problems, Problem{
			Severity: Degraded,
			Icon:     osTypeFromID(pair.png).String(),
			Message: fmt.Sprintf(
				"does not render from an app bundle, and the file holds no %s and %s at that size",
				pair.colour, pair.mask,
			),
		})
	}
	return problems
}

// gaps reports the written types the file lacks below the largest icon it
// holds. Sizes above that are absent because the artwork ran out, which is
// not a fault of the file.
func gaps(icons []Entry) []Problem {
	var largest uint
	for _, icon := range icons {
		largest = max(largest, icon.Size)
	}
	var absent []string
	for _, t := range osTypes {
		if !t.emit || t.Size > largest {
			continue
		}
		if slices.ContainsFunc(icons, func(e Entry) bool { return e.ID == t.ID }) {
			continue
		}
		absent = append(absent, t.String())
	}
	if len(absent) == 0 {
		return nil
	}
	return []Problem{{
		Severity: Advice,
		Message: fmt.Sprintf(
			"the file holds no %s, so macOS scales another icon where they are asked for",
			strings.Join(absent, ", "),
		),
	}}
}
