package appicon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func art(side int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xFF})
		}
	}
	return img
}

func paths(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// decode reads the manifest back as loosely typed JSON, so the test asserts
// the shape actool reads rather than the Go types that produced it.
func decode(t *testing.T, files map[string][]byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(files["icon.json"], &doc); err != nil {
		t.Fatalf("manifest does not parse: %v", err)
	}
	return doc
}

func TestFilesHoldsAManifestAndItsLayers(t *testing.T) {
	files, err := New(art(64), "Probe").Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	want := []string{"Assets/Probe.png", "icon.json"}
	if got := paths(files); !reflect.DeepEqual(got, want) {
		t.Errorf("files are %v, want %v", got, want)
	}
	img, err := png.Decode(bytes.NewReader(files["Assets/Probe.png"]))
	if err != nil {
		t.Fatalf("the layer is not a png: %v", err)
	}
	if got := img.Bounds().Size(); got.X != 64 || got.Y != 64 {
		t.Errorf("layer is %v, want the artwork it was given", got)
	}
}

// TestManifestMatchesTheShapeIconComposerWrites pins the key names against a
// manifest taken from a shipping app's bundle.
func TestManifestMatchesTheShapeIconComposerWrites(t *testing.T) {
	files, err := New(art(32), "Mist").Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	doc := decode(t, files)
	if doc["fill"] != FillAutomatic {
		t.Errorf("fill is %v, want %q", doc["fill"], FillAutomatic)
	}
	platforms, ok := doc["supported-platforms"].(map[string]any)
	if !ok {
		t.Fatalf("supported-platforms is %T, want an object", doc["supported-platforms"])
	}
	if squares, _ := platforms["squares"].([]any); len(squares) != 1 || squares[0] != PlatformMac {
		t.Errorf("squares are %v, want [%s]", platforms["squares"], PlatformMac)
	}
	groups, ok := doc["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("groups are %v, want one", doc["groups"])
	}
	group := groups[0].(map[string]any)
	for key, want := range map[string]any{
		"shadow":       map[string]any{"kind": ShadowNeutral, "opacity": 0.5},
		"translucency": map[string]any{"enabled": true, "value": 0.5},
	} {
		if got := group[key]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s is %v, want %v", key, got, want)
		}
	}
	layers := group["layers"].([]any)
	if len(layers) != 1 {
		t.Fatalf("layers are %v, want one", layers)
	}
	want := map[string]any{"glass": false, "image-name": "Mist.png", "name": "Mist"}
	if got := layers[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("layer is %v, want %v", got, want)
	}
}

func TestGlassAndSeveralGroups(t *testing.T) {
	b := Bundle{
		Groups: []Group{
			{Layers: []Layer{{Name: "Back", Image: art(16)}}},
			{
				Layers:       []Layer{{Name: "Front", Image: art(16), Glass: true}},
				Shadow:       &Shadow{Kind: "layer-color", Opacity: 0.25},
				Translucency: &Translucency{Enabled: false, Value: 0.8},
			},
		},
	}
	files, err := b.Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	want := []string{"Assets/Back.png", "Assets/Front.png", "icon.json"}
	if got := paths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("files are %v, want %v", got, want)
	}
	groups := decode(t, files)["groups"].([]any)
	if len(groups) != 2 {
		t.Fatalf("groups are %v, want two", groups)
	}
	// A group given neither is written without them rather than with zeroes.
	first := groups[0].(map[string]any)
	if _, ok := first["shadow"]; ok {
		t.Errorf("a group with no shadow wrote one: %v", first)
	}
	if _, ok := first["translucency"]; ok {
		t.Errorf("a group with no translucency wrote one: %v", first)
	}
	second := groups[1].(map[string]any)
	if got := second["shadow"]; !reflect.DeepEqual(got, map[string]any{"kind": "layer-color", "opacity": 0.25}) {
		t.Errorf("shadow is %v", got)
	}
	if got := second["layers"].([]any)[0].(map[string]any)["glass"]; got != true {
		t.Errorf("glass is %v, want true", got)
	}
}

// TestSpecializedValuesReplaceThePlainOnes covers the pairs where the
// manifest holds either a value or a list of them specialised by appearance,
// never both.
func TestSpecializedValuesReplaceThePlainOnes(t *testing.T) {
	material := 1.0
	b := Bundle{
		Fill:  "automatic",
		Fills: []Specialized[Fill]{{Value: NamedFill("system-light")}},
		Groups: []Group{{
			Layers:         []Layer{{Name: "One", Image: art(16)}},
			Translucency:   &Translucency{Enabled: true, Value: 0.5},
			Translucencies: []Specialized[Translucency]{{Value: Translucency{Enabled: false, Value: 0.8}}},
			BlurMaterial:   &material,
			BlurMaterials:  []Specialized[float64]{{Value: 2}},
		}},
	}
	files, err := b.Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	doc := decode(t, files)
	if _, ok := doc["fill"]; ok {
		t.Errorf("wrote a plain fill beside the specialised one: %v", doc["fill"])
	}
	if _, ok := doc["fill-specializations"]; !ok {
		t.Error("did not write the specialised fill")
	}
	group := doc["groups"].([]any)[0].(map[string]any)
	for _, plain := range []string{"translucency", "blur-material"} {
		if _, ok := group[plain]; ok {
			t.Errorf("wrote a plain %s beside the specialised one: %v", plain, group[plain])
		}
	}
	for _, special := range []string{"translucency-specializations", "blur-material-specializations"} {
		if _, ok := group[special]; !ok {
			t.Errorf("did not write %s", special)
		}
	}
}

// TestRichManifestMatchesTheShippingShape builds the manifest a composed icon
// needs and holds its keys against the ones a shipping app's bundle uses.
func TestRichManifestMatchesTheShippingShape(t *testing.T) {
	b := Bundle{
		Fills: []Specialized[Fill]{
			{Value: NamedFill("system-light")},
			{Appearance: AppearanceDark, Value: NamedFill("system-dark")},
		},
		Groups: []Group{{
			BlendModes: []Specialized[string]{{Appearance: AppearanceTinted, Value: "normal"}},
			Lighting:   "individual",
			Specular:   true,
			Shadow:     &Shadow{Kind: "layer-color", Opacity: 0.5},
			Translucencies: []Specialized[Translucency]{
				{Value: Translucency{Enabled: true, Value: 0.84}},
				{Appearance: AppearanceTinted, Value: Translucency{Enabled: false, Value: 0.84}},
			},
			Layers: []Layer{{
				Name:     "Cube",
				Image:    art(32),
				Glass:    true,
				Position: &Position{Scale: 1.24, Translation: [2]float64{0, 0}},
				Fills: []Specialized[Fill]{
					{Appearance: AppearanceDark, Value: NamedFill("automatic")},
					{Appearance: AppearanceTinted, Value: GradientFill(
						"display-p3:0.90000,0.90000,0.90000,0.83000",
						"srgb:1.00000,1.00000,1.00000,0.41987",
					)},
				},
				BlendModes: []Specialized[string]{{Appearance: AppearanceDark, Value: "lighten"}},
			}},
		}},
	}
	files, err := b.Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	doc := decode(t, files)
	group := doc["groups"].([]any)[0].(map[string]any)
	for _, key := range []string{
		"blend-mode-specializations", "lighting", "shadow", "specular",
		"translucency-specializations", "layers",
	} {
		if _, ok := group[key]; !ok {
			t.Errorf("group is missing %s", key)
		}
	}
	layer := group["layers"].([]any)[0].(map[string]any)
	for _, key := range []string{
		"blend-mode-specializations", "fill-specializations", "glass",
		"image-name", "name", "position",
	} {
		if _, ok := layer[key]; !ok {
			t.Errorf("layer is missing %s", key)
		}
	}
	// A gradient is the one fill written as an object naming its kind.
	tinted := layer["fill-specializations"].([]any)[1].(map[string]any)
	gradient, ok := tinted["value"].(map[string]any)["linear-gradient"].([]any)
	if !ok || len(gradient) != 2 {
		t.Errorf("gradient is %v, want two stops", tinted["value"])
	}
	if got := layer["position"].(map[string]any)["scale"]; got != 1.24 {
		t.Errorf("scale is %v, want 1.24", got)
	}
}

// TestHiddenIsWrittenOnlyWhenSet keeps a manifest from carrying a key for
// every default a layer did not set.
func TestHiddenIsWrittenOnlyWhenSet(t *testing.T) {
	files, err := New(art(16), "Plain").Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	layer := decode(t, files)["groups"].([]any)[0].(map[string]any)["layers"].([]any)[0].(map[string]any)
	if _, ok := layer["hidden"]; ok {
		t.Errorf("wrote hidden for a layer that is not: %v", layer)
	}
	b := Bundle{Groups: []Group{{Layers: []Layer{{Name: "Gone", Image: art(16), Hidden: true}}}}}
	files, err = b.Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	layer = decode(t, files)["groups"].([]any)[0].(map[string]any)["layers"].([]any)[0].(map[string]any)
	if layer["hidden"] != true {
		t.Errorf("hidden is %v, want true", layer["hidden"])
	}
}

// TestEveryKeyIsTheManifestSpelling walks the whole manifest for a key
// spelled the way Go spells a field rather than the way the manifest does. A
// value marshalled through a type that carries no tags looks right until
// actool reads it, and actool does not say which key it wanted.
func TestEveryKeyIsTheManifestSpelling(t *testing.T) {
	material := 1.0
	b := Bundle{
		Fills: []Specialized[Fill]{{Value: NamedFill("system-light")}},
		Groups: []Group{{
			BlendModes:    []Specialized[string]{{Appearance: AppearanceTinted, Value: "normal"}},
			BlurMaterials: []Specialized[float64]{{Value: material}},
			Lighting:      "individual",
			Specular:      true,
			Shadow:        &Shadow{Kind: "layer-color", Opacity: 0.5},
			Translucencies: []Specialized[Translucency]{
				{Value: Translucency{Enabled: true, Value: 0.84}},
				{Appearance: AppearanceDark, Value: Translucency{Enabled: false, Value: 0.5}},
			},
			Layers: []Layer{{
				Name: "One", Image: art(16), Glass: true, Hidden: true,
				Position:   &Position{Scale: 1.24, Translation: [2]float64{0, -12}},
				Fills:      []Specialized[Fill]{{Value: GradientFill("srgb:1,1,1,1")}},
				BlendModes: []Specialized[string]{{Appearance: AppearanceDark, Value: "lighten"}},
			}},
		}},
	}
	// The plain forms travel a different path from the specialised ones, so
	// both are walked.
	plain := Bundle{Groups: []Group{{
		Layers:       []Layer{{Name: "One", Image: art(16)}},
		Shadow:       &Shadow{Kind: ShadowNeutral, Opacity: 0.5},
		Translucency: &Translucency{Enabled: true, Value: 0.5},
	}}}
	for _, bundle := range []Bundle{b, plain} {
		files, err := bundle.Files()
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}
		var walk func(any, string)
		walk = func(node any, path string) {
			switch v := node.(type) {
			case map[string]any:
				for key, child := range v {
					if key != "" && key[0] >= 'A' && key[0] <= 'Z' {
						t.Errorf("%s.%s is spelled the way Go spells a field", path, key)
					}
					walk(child, path+"."+key)
				}
			case []any:
				for i, child := range v {
					walk(child, fmt.Sprintf("%s[%d]", path, i))
				}
			}
		}
		walk(any(decode(t, files)), "")
	}
}

func TestFilesRejectsWhatItCannotWrite(t *testing.T) {
	for _, tt := range []struct {
		name   string
		bundle Bundle
		want   error
	}{
		{"no groups", Bundle{}, ErrNoLayers},
		{"no layers", Bundle{Groups: []Group{{}}}, ErrNoLayers},
		{
			name: "two layers of one name",
			bundle: Bundle{Groups: []Group{{Layers: []Layer{
				{Name: "Same", Image: art(16)},
				{Name: "Same", Image: art(16)},
			}}}},
			want: ErrDuplicateLayer,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.bundle.Files(); !errors.Is(err, tt.want) {
				t.Errorf("error is %v, want %v", err, tt.want)
			}
		})
	}
}

func TestFilesRejectsALayerWithoutArtwork(t *testing.T) {
	b := Bundle{Groups: []Group{{Layers: []Layer{{Name: "Empty"}}}}}
	if _, err := b.Files(); err == nil {
		t.Error("rendering a layer with no image returned no error")
	}
}

// TestLayerNamesBecomeFileNames keeps a name that cannot be a file from
// producing a manifest pointing at something unwritable.
func TestLayerNamesBecomeFileNames(t *testing.T) {
	b := Bundle{Groups: []Group{{Layers: []Layer{
		{Name: "../escape", Image: art(16)},
		{Name: "", Image: art(16)},
	}}}}
	files, err := b.Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	want := []string{"Assets/Layer 2.png", "Assets/escape.png", "icon.json"}
	if got := paths(files); !reflect.DeepEqual(got, want) {
		t.Errorf("files are %v, want %v", got, want)
	}
}

func TestWriteBuildsTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Probe.icon")
	if err := New(art(64), "Probe").Write(dir); err != nil {
		t.Fatalf("writing: %v", err)
	}
	for _, name := range []string{"icon.json", filepath.Join("Assets", "Probe.png")} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("bundle is missing %s: %v", name, err)
		}
	}
	// The manifest on disk is the one Files rendered.
	onDisk, err := os.ReadFile(filepath.Join(dir, "icon.json"))
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	files, err := New(art(64), "Probe").Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	if !bytes.Equal(onDisk, files["icon.json"]) {
		t.Error("the manifest on disk differs from the one rendered")
	}
}
