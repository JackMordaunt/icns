package ico

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Severity ranks what a Problem means for the icon Windows ends up drawing.
type Severity int

const (
	// Invisible means Windows passes the icon over and draws another size.
	Invisible Severity = iota
	// Degraded means Windows draws the icon, but not at the size or with the
	// transparency it was given.
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

// Problem is one finding about an ico file.
type Problem struct {
	// Severity is what the finding means for the icon Windows draws.
	Severity Severity
	// Icon names the icon the finding concerns, such as "256x256 (PNG)", and
	// is empty when the finding concerns the file as a whole.
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

// drawn are the sizes Windows asks for often enough that a file without them
// leaves it scaling another icon.
var drawn = []int{16, 32, 48, 256}

// Validate reads an ico file and reports what Windows will make of it, most
// serious first. Data that cannot be parsed as an ico at all is returned as
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
	problems = append(problems, missing(icons)...)
	problems = append(problems, duplicated(icons)...)
	slices.SortStableFunc(problems, func(a, b Problem) int {
		return cmp.Compare(a.Severity, b.Severity)
	})
	return problems, nil
}

// problems reports what the icon's own bytes say about it.
func (e IconDecoder) problems() []Problem {
	if e.Format == FormatPNG {
		return e.pngProblems()
	}
	return e.bmpProblems()
}

// pngProblems reads the image header of a PNG icon and compares what it
// declares against what Windows reads and what the directory promised.
func (e IconDecoder) pngProblems() []Problem {
	width, height, colour, ok := ihdr(e.data)
	if !ok {
		return []Problem{{
			Severity: Invisible,
			Icon:     e.String(),
			Message:  "begins with a PNG signature but holds no image header",
		}}
	}
	var problems []Problem
	// Windows reads a PNG icon only when it is stored as 32 bit RGBA. The
	// colour types without an alpha channel are greyscale, truecolour and
	// indexed.
	switch colour {
	case pngGreyscale, pngTruecolour, pngIndexed:
		problems = append(problems, Problem{
			Severity: Invisible,
			Icon:     e.String(),
			Message: fmt.Sprintf(
				"stored as %s, which carries no alpha channel; Windows draws a PNG icon only when it is 32 bit RGBA",
				colourType(colour),
			),
		})
	}
	if width != e.Width || height != e.Height {
		problems = append(problems, Problem{
			Severity: Degraded,
			Icon:     e.String(),
			Message: fmt.Sprintf(
				"holds a %dx%d image, but the directory lists it as %dx%d",
				width, height, e.Width, e.Height,
			),
		})
	}
	return problems
}

// bmpProblems reads the header of a bitmap icon and compares what it declares
// against the directory. The stored height covers the pixels and the mask
// together, so it is twice the height of the icon.
func (e IconDecoder) bmpProblems() []Problem {
	if len(e.data) < headerSize {
		return []Problem{{
			Severity: Invisible,
			Icon:     e.String(),
			Message:  fmt.Sprintf("holds %d bytes, too few for a bitmap header", len(e.data)),
		}}
	}
	var (
		width       = int(int32(binary.LittleEndian.Uint32(e.data[4:8])))
		storedH     = int(int32(binary.LittleEndian.Uint32(e.data[8:12])))
		compression = binary.LittleEndian.Uint32(e.data[16:20])
		problems    []Problem
	)
	if compression != 0 {
		problems = append(problems, Problem{
			Severity: Invisible,
			Icon:     e.String(),
			Message:  "the bitmap is compressed",
		})
	}
	if storedH <= 0 || storedH%2 != 0 {
		problems = append(problems, Problem{
			Severity: Invisible,
			Icon:     e.String(),
			Message: fmt.Sprintf(
				"the bitmap declares a height of %d, which is not the pixels and the mask together",
				storedH,
			),
		})
	} else if width != e.Width || storedH/2 != e.Height {
		problems = append(problems, Problem{
			Severity: Degraded,
			Icon:     e.String(),
			Message: fmt.Sprintf(
				"the bitmap is %dx%d, but the directory lists it as %dx%d",
				width, storedH/2, e.Width, e.Height,
			),
		})
	}
	if e.Width >= pngAbove {
		problems = append(problems, Problem{
			Severity: Advice,
			Icon:     e.String(),
			Message: fmt.Sprintf(
				"a bitmap this size costs %d bytes, where a PNG would cost a fraction of it",
				len(e.data),
			),
		})
	}
	return problems
}

// missing reports the sizes Windows draws that the file does not hold.
func missing(icons []IconDecoder) []Problem {
	var absent []string
	for _, size := range drawn {
		if slices.ContainsFunc(icons, func(e IconDecoder) bool {
			return e.Width == size && e.Height == size
		}) {
			continue
		}
		absent = append(absent, fmt.Sprintf("%dx%d", size, size))
	}
	if len(absent) == 0 {
		return nil
	}
	return []Problem{{
		Severity: Advice,
		Message: fmt.Sprintf(
			"the file holds no %s, so Windows scales another icon to draw them",
			strings.Join(absent, ", "),
		),
	}}
}

// duplicated reports sizes the file holds more than once.
func duplicated(icons []IconDecoder) []Problem {
	seen := make(map[string]int, len(icons))
	var order []string
	for _, icon := range icons {
		size := fmt.Sprintf("%dx%d", icon.Width, icon.Height)
		if seen[size] == 0 {
			order = append(order, size)
		}
		seen[size]++
	}
	var problems []Problem
	for _, size := range order {
		if seen[size] < 2 {
			continue
		}
		problems = append(problems, Problem{
			Severity: Degraded,
			Message: fmt.Sprintf(
				"the file holds %d icons of %s, and which one Windows draws is not defined",
				seen[size], size,
			),
		})
	}
	return problems
}

// ihdr reads the width, height and colour type a PNG declares. The boolean
// reports whether the image header is present.
func ihdr(data []byte) (width, height int, colour byte, ok bool) {
	// The signature, then the length and type of the first chunk, then the
	// thirteen bytes the image header holds.
	const at = 16
	if len(data) < at+10 || string(data[12:16]) != typeIHDR {
		return 0, 0, 0, false
	}
	width = int(binary.BigEndian.Uint32(data[at : at+4]))
	height = int(binary.BigEndian.Uint32(data[at+4 : at+8]))
	return width, height, data[at+9], true
}

// PNG chunk and colour types, as the image header of an icon numbers them.
const (
	// typeIHDR is the chunk type a PNG stores its image header under.
	typeIHDR = "IHDR"
	// pngGreyscale is the colour type of one grey channel.
	pngGreyscale = 0
	// pngTruecolour is the colour type of red, green and blue channels.
	pngTruecolour = 2
	// pngIndexed is the colour type of indices into a colour table.
	pngIndexed = 3
	// pngGreyscaleAlpha is the colour type of a grey channel and its alpha.
	pngGreyscaleAlpha = 4
	// pngTruecolourAlpha is the colour type of red, green, blue and alpha.
	pngTruecolourAlpha = 6
)

// colourType names how a PNG stores its pixels.
func colourType(c byte) string {
	switch c {
	case pngGreyscale:
		return "greyscale"
	case pngTruecolour:
		return "truecolour"
	case pngIndexed:
		return "indexed colour"
	case pngGreyscaleAlpha:
		return "greyscale with alpha"
	case pngTruecolourAlpha:
		return "truecolour with alpha"
	}
	return fmt.Sprintf("colour type %d", c)
}
