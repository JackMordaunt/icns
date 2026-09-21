//go:build darwin

package appicon

import (
	"image"
	"image/color"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// These tests hold the package against actool, the tool that turns a bundle
// into the catalog macOS reads. A manifest this package writes has to be one
// actool takes, and nothing short of running it says whether it is: actool
// crashes on some manifests rather than reporting a problem with them, so a
// bundle that is merely well formed proves nothing.

// compile runs actool over a bundle and returns the directory it wrote into.
// A missing tool fails the test rather than skipping it, since a skipped
// oracle checks nothing while appearing to pass.
func compile(t *testing.T, bundle string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(bundle)
	name = name[:len(name)-len(filepath.Ext(name))]
	cmd := exec.Command("xcrun", "actool", bundle,
		"--compile", out,
		"--output-format", "human-readable-text",
		"--errors",
		"--output-partial-info-plist", filepath.Join(out, "partial.plist"),
		"--app-icon", name,
		"--include-all-app-icons",
		"--enable-on-demand-resources", "NO",
		"--development-region", "en",
		"--target-device", "mac",
		"--minimum-deployment-target", "26.0",
		"--platform", "macosx",
	)
	printed, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("actool %s: %v\n%s", name, err, printed)
	}
	return out
}

// layer draws a disc on transparency, which is the shape of artwork a bundle
// carries: full bleed, for the compiler to mask and light.
func layer(radius float64, c color.NRGBA) image.Image {
	const side = 512
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	centre := float64(side) / 2
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			if math.Hypot(float64(x)-centre, float64(y)-centre) > radius {
				continue
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// written writes a bundle under a temporary directory and returns its path.
func written(t *testing.T, name string, b Bundle) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name+".icon")
	if err := b.Write(dir); err != nil {
		t.Fatalf("writing the bundle: %v", err)
	}
	return dir
}

// compiled fails unless actool produced the catalog and the plist naming it.
func compiled(t *testing.T, out string) {
	t.Helper()
	for _, name := range []string{"Assets.car", "partial.plist"} {
		info, err := os.Stat(filepath.Join(out, name))
		if err != nil {
			t.Errorf("actool wrote no %s: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("actool wrote an empty %s", name)
		}
	}
}

func TestActoolTakesASingleLayer(t *testing.T) {
	bundle := written(t, "Single", New(layer(210, color.NRGBA{R: 40, G: 70, B: 200, A: 255}), "Single"))
	compiled(t, compile(t, bundle))
}

// TestActoolTakesTheAppearanceFields covers the fields that vary by
// appearance together, in the arrangement a shipping app uses. They have
// nowhere else to be checked: a manifest actool ignores looks exactly like
// one it honours.
func TestActoolTakesTheAppearanceFields(t *testing.T) {
	material := 1.0
	bundle := written(t, "Composed", Bundle{
		Fills: []Specialized[Fill]{
			{Value: NamedFill("system-light")},
			{Appearance: AppearanceDark, Value: NamedFill("system-dark")},
		},
		Groups: []Group{
			{
				BlendModes:    []Specialized[string]{{Appearance: AppearanceTinted, Value: "normal"}},
				BlurMaterials: []Specialized[float64]{{Value: material}},
				Lighting:      "individual",
				Specular:      true,
				Shadow:        &Shadow{Kind: "layer-color", Opacity: 0.5},
				Translucencies: []Specialized[Translucency]{
					{Value: Translucency{Enabled: true, Value: 0.84}},
					{Appearance: AppearanceTinted, Value: Translucency{Enabled: false, Value: 0.84}},
				},
				Layers: []Layer{{
					Name:  "Back",
					Image: layer(210, color.NRGBA{R: 40, G: 70, B: 200, A: 255}),
				}},
			},
			{
				Layers: []Layer{{
					Name:     "Front",
					Image:    layer(130, color.NRGBA{R: 240, G: 240, B: 250, A: 255}),
					Glass:    true,
					Hidden:   false,
					Position: &Position{Scale: 1.24, Translation: [2]float64{0, -12}},
					Fills: []Specialized[Fill]{
						{Appearance: AppearanceDark, Value: NamedFill("automatic")},
						{Appearance: AppearanceTinted, Value: GradientFill(
							"display-p3:0.90000,0.90000,0.90000,0.83000",
							"srgb:1.00000,1.00000,1.00000,0.41987",
						)},
					},
					BlendModes: []Specialized[string]{{Appearance: AppearanceDark, Value: "lighten"}},
				}},
			},
		},
	})
	compiled(t, compile(t, bundle))
}

// TestActoolWritesAnIcnsBeside covers the half of the output that keeps older
// systems working, which the compiler produces from the same bundle.
func TestActoolWritesAnIcnsBeside(t *testing.T) {
	bundle := written(t, "Legacy", New(layer(210, color.NRGBA{R: 200, G: 60, B: 40, A: 255}), "Legacy"))
	out := compile(t, bundle)
	if _, err := os.Stat(filepath.Join(out, "Legacy.icns")); err != nil {
		t.Errorf("actool wrote no icns beside the catalog: %v", err)
	}
}
