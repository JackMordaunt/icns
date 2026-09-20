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

// manifest is the file name the bundle's directory holds beside its assets.
const manifest = "icon.json"

// assets is the directory within the bundle that holds the layer images.
const assets = "Assets"

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
	out := map[string][]byte{manifest: append(encoded, '\n')}
	for i, group := range b.Groups {
		for j, layer := range group.Layers {
			buf := bytes.NewBuffer(nil)
			if err := png.Encode(buf, layer.Image); err != nil {
				return nil, fmt.Errorf("writing layer %s: %w", names[i][j], err)
			}
			out[path.Join(assets, names[i][j])] = buf.Bytes()
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
	if err := os.MkdirAll(filepath.Join(dir, assets), 0o755); err != nil {
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
func (b Bundle) document() (icon, [][]string, error) {
	var layers int
	for _, group := range b.Groups {
		layers += len(group.Layers)
	}
	if layers == 0 {
		return icon{}, nil, ErrNoLayers
	}
	doc := icon{
		Fill:      b.Fill,
		Platforms: platforms{Squares: b.Platforms},
	}
	if doc.Fill == "" {
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
		out := jsonGroup{Layers: make([]jsonLayer, len(group.Layers))}
		for j, layer := range group.Layers {
			if layer.Image == nil {
				return icon{}, nil, fmt.Errorf("layer %q has no image", layer.Name)
			}
			n++
			name := layerName(layer.Name, n)
			if taken[name] {
				return icon{}, nil, fmt.Errorf("%w: %q", ErrDuplicateLayer, name)
			}
			taken[name] = true
			file := name + ".png"
			names[i][j] = file
			out.Layers[j] = jsonLayer{
				Glass:     layer.Glass,
				ImageName: file,
				Name:      name,
			}
		}
		if s := group.Shadow; s != nil {
			kind := s.Kind
			if kind == "" {
				kind = ShadowNeutral
			}
			out.Shadow = &jsonShadow{Kind: kind, Opacity: s.Opacity}
		}
		if t := group.Translucency; t != nil {
			out.Translucency = &jsonTranslucency{Enabled: t.Enabled, Value: t.Value}
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
type icon struct {
	Fill      string      `json:"fill"`
	Groups    []jsonGroup `json:"groups"`
	Platforms platforms   `json:"supported-platforms"`
}

type platforms struct {
	Squares []string `json:"squares"`
}

type jsonGroup struct {
	Layers       []jsonLayer       `json:"layers"`
	Shadow       *jsonShadow       `json:"shadow,omitempty"`
	Translucency *jsonTranslucency `json:"translucency,omitempty"`
}

type jsonLayer struct {
	Glass     bool   `json:"glass"`
	ImageName string `json:"image-name"`
	Name      string `json:"name"`
}

type jsonShadow struct {
	Kind    string  `json:"kind"`
	Opacity float64 `json:"opacity"`
}

type jsonTranslucency struct {
	Enabled bool    `json:"enabled"`
	Value   float64 `json:"value"`
}
