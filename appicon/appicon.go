// Package appicon writes the icon bundle Icon Composer authors, which macOS
// 26 compiles into an app's icon.
//
// A bundle is a directory named with a .icon extension holding icon.json and
// an Assets directory of layer images. The manifest names the layers and says
// how they are lit, shadowed and filled; the compiler decides what the icon
// looks like at each size, so a bundle carries artwork rather than
// renditions.
//
// Nothing here compiles a bundle. That is actool's work, and it runs only on
// macOS; a bundle written by this package is the input it takes.
package appicon

import (
	"errors"
	"image"
)

// Errors returned when a bundle cannot be written. They are wrapped with
// detail, so compare with errors.Is.
var (
	// ErrNoLayers means the bundle names no artwork to draw.
	ErrNoLayers = errors.New("no layers to write")
	// ErrDuplicateLayer means two layers share a name, so one image would
	// overwrite the other.
	ErrDuplicateLayer = errors.New("two layers share a name")
	// ErrEmptyFill means a fill names neither a gradient, a colour nor a
	// fill the system provides.
	ErrEmptyFill = errors.New("fill names no colour")
)

// Bundle is an icon: a manifest naming layers, and the images they hold.
type Bundle struct {
	// Fill is how the space behind the layers is filled. Empty is written as
	// "automatic", which leaves the choice to the compiler, and is ignored
	// when Fills is given.
	Fill string
	// Fills is the fill specialised by appearance, for an icon whose
	// background differs in dark mode or when tinted.
	Fills []Specialized[Fill]
	// Groups are composited back to front.
	Groups []Group
	// Platforms are the platforms the icon offers square artwork for. Empty
	// is written as macOS alone.
	Platforms []string
}

// Group is a set of layers that share lighting, shadow and translucency.
type Group struct {
	// Layers are composited back to front within the group.
	Layers []Layer
	// Shadow is the shadow cast beneath the group. Nil leaves it out of the
	// manifest.
	Shadow *Shadow
	// Translucency is how far the group lets the material behind it through.
	// Nil leaves it out of the manifest, and it is ignored when
	// Translucencies is given.
	Translucency *Translucency
	// Translucencies is the translucency specialised by appearance.
	Translucencies []Specialized[Translucency]
	// Lighting is how the group is lit, such as "individual" or "combined".
	Lighting string
	// Specular asks for a specular highlight across the group.
	Specular bool
	// BlurMaterial is the material the group blurs what is behind it with.
	// Nil leaves it out, and it is ignored when BlurMaterials is given.
	BlurMaterial *float64
	// BlurMaterials is the blur material specialised by appearance.
	BlurMaterials []Specialized[float64]
	// BlendModes is how the group composites, specialised by appearance,
	// such as "normal" or "lighten".
	BlendModes []Specialized[string]
}

// Layer is one image in the stack.
type Layer struct {
	// Name identifies the layer in the manifest and names its file.
	Name string
	// Image is the artwork. It is written as a PNG at the size it is given.
	Image image.Image
	// Glass asks for the layer to be treated as glass, which the compiler
	// lights and refracts rather than drawing flat.
	Glass bool
	// Hidden keeps the layer in the manifest without drawing it.
	Hidden bool
	// Position is where the layer sits. Nil leaves it where it was drawn.
	Position *Position
	// Fills is the layer's own fill, specialised by appearance. A layer
	// given one is filled with it rather than with its image's colour.
	Fills []Specialized[Fill]
	// BlendModes is how the layer composites, specialised by appearance.
	BlendModes []Specialized[string]
}

// Shadow is the shadow a group casts.
type Shadow struct {
	// Kind is how the shadow takes its colour, such as "neutral" or
	// "layer-color".
	Kind string
	// Opacity is how dark it is, from 0 to 1.
	Opacity float64
}

// Translucency is how far a group lets what is behind it through.
type Translucency struct {
	Enabled bool
	// Value is the amount, from 0 to 1.
	Value float64
}

// Defaults written where a bundle leaves a choice open.
const (
	// FillAutomatic leaves the fill to the compiler.
	FillAutomatic = "automatic"
	// ShadowNeutral takes the shadow's colour from neither the layer nor the
	// background.
	ShadowNeutral = "neutral"
	// PlatformMac is the platform macOS icons declare.
	PlatformMac = "macOS"
)

// New returns a bundle holding one layer, which is what a single image makes.
// The shadow and translucency match what Icon Composer writes for an icon
// composed the same way.
func New(img image.Image, name string) Bundle {
	return Bundle{
		Fill: FillAutomatic,
		Groups: []Group{{
			Layers:       []Layer{{Name: name, Image: img}},
			Shadow:       &Shadow{Kind: ShadowNeutral, Opacity: 0.5},
			Translucency: &Translucency{Enabled: true, Value: 0.5},
		}},
		Platforms: []string{PlatformMac},
	}
}
