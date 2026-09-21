package appicon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ManifestName is the file name the bundle's directory holds beside its assets.
const ManifestName = "icon.json"

// AssetsName is the directory within the bundle that holds the layer images.
const AssetsName = "Assets"

// Files renders the bundle as the files its directory holds, keyed by their
// path within it. The manifest is at icon.json and every layer's image is
// under Assets.
func (b Bundle) Files() (map[string][]byte, error) {
	doc, names, err := b.document()
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("writing the manifest: %w", err)
	}
	out := map[string][]byte{ManifestName: append(encoded, '\n')}
	for i, group := range b.Groups {
		for j, layer := range group.Layers {
			buf := bytes.NewBuffer(nil)
			if err := png.Encode(buf, layer.Image); err != nil {
				return nil, fmt.Errorf("writing layer %s: %w", names[i][j], err)
			}
			out[path.Join(AssetsName, names[i][j])] = buf.Bytes()
		}
	}
	return out, nil
}

// Write writes the bundle into dir, which is created along with the assets
// directory beneath it. The directory is the bundle, so its name is what
// carries the .icon extension.
func (b Bundle) Write(dir string) error {
	files, err := b.Files()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, AssetsName), 0o755); err != nil {
		return fmt.Errorf("preparing the bundle: %w", err)
	}
	for name, data := range files {
		at := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.WriteFile(at, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return nil
}

// document builds the manifest and the file name chosen for every layer,
// which the caller needs to write the images where the manifest says.
func (b Bundle) document() (manifest, [][]string, error) {
	var layers int
	for _, group := range b.Groups {
		layers += len(group.Layers)
	}
	if layers == 0 {
		return manifest{}, nil, ErrNoLayers
	}
	doc := manifest{
		Fill:      b.Fill,
		Fills:     b.Fills,
		Platforms: platforms{Squares: b.Platforms},
	}
	// A specialised fill says everything the plain one does, so only one of
	// the two is written.
	if len(doc.Fills) > 0 {
		doc.Fill = ""
	} else if doc.Fill == "" {
		doc.Fill = FillAutomatic
	}
	if len(doc.Platforms.Squares) == 0 {
		doc.Platforms.Squares = []string{PlatformMac}
	}
	var (
		names = make([][]string, len(b.Groups))
		taken = make(map[string]bool, layers)
		n     int
	)
	for i, group := range b.Groups {
		names[i] = make([]string, len(group.Layers))
		out := jsonGroup{
			BlendModes:     group.BlendModes,
			BlurMaterials:  group.BlurMaterials,
			Layers:         make([]jsonLayer, len(group.Layers)),
			Lighting:       group.Lighting,
			Specular:       group.Specular,
			Translucencies: group.Translucencies,
		}
		if len(group.BlurMaterials) == 0 {
			out.BlurMaterial = group.BlurMaterial
		}
		for j, layer := range group.Layers {
			if layer.Image == nil {
				return manifest{}, nil, fmt.Errorf("layer %q has no image", layer.Name)
			}
			n++
			name := layerName(layer.Name, n)
			if taken[name] {
				return manifest{}, nil, fmt.Errorf("%w: %q", ErrDuplicateLayer, name)
			}
			taken[name] = true
			file := name + ".png"
			names[i][j] = file
			out.Layers[j] = jsonLayer{
				BlendModes: layer.BlendModes,
				Fills:      layer.Fills,
				Glass:      layer.Glass,
				Hidden:     layer.Hidden,
				ImageName:  file,
				Name:       name,
				Position:   layer.Position,
			}
		}
		if s := group.Shadow; s != nil {
			kind := s.Kind
			if kind == "" {
				kind = ShadowNeutral
			}
			out.Shadow = &Shadow{Kind: kind, Opacity: s.Opacity}
		}
		if t := group.Translucency; t != nil && len(group.Translucencies) == 0 {
			out.Translucency = t
		}
		doc.Groups = append(doc.Groups, out)
	}
	return doc, names, nil
}

// layerName renders a layer's name as something that can also be a file name,
// falling back to its position when nothing usable is left.
func layerName(name string, position int) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == ' ':
			return r
		}
		return -1
	}, name)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return fmt.Sprintf("Layer %d", position)
	}
	return clean
}

// The manifest's shape. The names are the ones Icon Composer writes, which
// is what actool reads.
type manifest struct {
	Fill      string              `json:"fill,omitempty"`
	Fills     []Specialized[Fill] `json:"fill-specializations,omitempty"`
	Groups    []jsonGroup         `json:"groups"`
	Platforms platforms           `json:"supported-platforms"`
}

type platforms struct {
	Squares []string `json:"squares"`
}

type jsonGroup struct {
	BlendModes     []Specialized[string]       `json:"blend-mode-specializations,omitempty"`
	BlurMaterial   *float64                    `json:"blur-material,omitempty"`
	BlurMaterials  []Specialized[float64]      `json:"blur-material-specializations,omitempty"`
	Layers         []jsonLayer                 `json:"layers"`
	Lighting       string                      `json:"lighting,omitempty"`
	Shadow         *Shadow                     `json:"shadow,omitempty"`
	Specular       bool                        `json:"specular,omitempty"`
	Translucency   *Translucency               `json:"translucency,omitempty"`
	Translucencies []Specialized[Translucency] `json:"translucency-specializations,omitempty"`
}

type jsonLayer struct {
	BlendModes []Specialized[string] `json:"blend-mode-specializations,omitempty"`
	Fills      []Specialized[Fill]   `json:"fill-specializations,omitempty"`
	Glass      bool                  `json:"glass"`
	Hidden     bool                  `json:"hidden,omitempty"`
	ImageName  string                `json:"image-name"`
	Name       string                `json:"name"`
	Position   *Position             `json:"position,omitempty"`
}
