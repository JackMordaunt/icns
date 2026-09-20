package appicon

import (
	"encoding/json"
	"fmt"
)

// Appearance names a variant a value is specialised for. The zero value is
// the appearance the others vary from.
type Appearance string

// The appearances an icon is drawn in.
const (
	// AppearanceDefault is the appearance the others vary from.
	AppearanceDefault Appearance = ""
	// AppearanceDark is the icon as drawn in dark mode.
	AppearanceDark Appearance = "dark"
	// AppearanceTinted is the icon as drawn when the system tints it.
	AppearanceTinted Appearance = "tinted"
)

// Specialized is a value that differs by appearance. An entry with the
// default appearance is the one the others vary from, and is written first.
type Specialized[T any] struct {
	Appearance Appearance `json:"appearance,omitempty"`
	Value      T          `json:"value"`
}

// For returns the value specialised for an appearance, falling back to the
// default entry. The boolean reports whether either was found.
func For[T any](values []Specialized[T], a Appearance) (T, bool) {
	var (
		out   T
		found bool
	)
	for _, v := range values {
		if v.Appearance == a {
			return v.Value, true
		}
		if v.Appearance == AppearanceDefault {
			out, found = v.Value, true
		}
	}
	return out, found
}

// Fill is how a surface is filled. Exactly one of the three is written, in
// the order they are listed here.
type Fill struct {
	// Gradient is a list of colour stops, first to last, each a colour space
	// and its components, such as "srgb:1.00000,0.25279,1.00000,1.00000".
	Gradient []string
	// Solid is a single colour, written the same way as a stop.
	Solid string
	// Name is a fill the system provides, such as "automatic",
	// "system-light" or "system-dark".
	Name string
}

// NamedFill returns a fill the system provides.
func NamedFill(name string) Fill { return Fill{Name: name} }

// SolidFill returns a fill of one colour, such as
// SolidFill("srgb:1.00000,0.25279,1.00000,1.00000").
func SolidFill(colour string) Fill { return Fill{Solid: colour} }

// GradientFill returns a linear gradient through the colours given.
func GradientFill(stops ...string) Fill { return Fill{Gradient: stops} }

// MarshalJSON writes the fill the way the manifest holds it: a bare string
// for a named fill, and an object naming the kind for the others.
func (f Fill) MarshalJSON() ([]byte, error) {
	switch {
	case len(f.Gradient) > 0:
		return json.Marshal(map[string][]string{"linear-gradient": f.Gradient})
	case f.Solid != "":
		return json.Marshal(map[string]string{"solid": f.Solid})
	case f.Name != "":
		return json.Marshal(f.Name)
	}
	return nil, fmt.Errorf("%w: a fill names no colour", ErrEmptyFill)
}

// UnmarshalJSON reads a fill written either way.
func (f *Fill) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		*f = Fill{Name: name}
		return nil
	}
	var object struct {
		Gradient []string `json:"linear-gradient"`
		Solid    string   `json:"solid"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("reading a fill: %w", err)
	}
	*f = Fill{Gradient: object.Gradient, Solid: object.Solid}
	return nil
}

// Position is where a layer sits relative to the space it is drawn in.
type Position struct {
	// Scale multiplies the layer's size, 1 leaving it as it is.
	Scale float64 `json:"scale"`
	// Translation moves it, across and down, in points.
	Translation [2]float64 `json:"translation-in-points"`
}
