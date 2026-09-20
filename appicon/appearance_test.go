package appicon

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestFillWritesTheShapeItNames(t *testing.T) {
	for _, tt := range []struct {
		name string
		fill Fill
		want string
	}{
		{"a fill the system provides", NamedFill("automatic"), `"automatic"`},
		{"one colour", SolidFill("srgb:1.00000,0.25279,1.00000,1.00000"), `{"solid":"srgb:1.00000,0.25279,1.00000,1.00000"}`},
		{
			name: "a gradient",
			fill: GradientFill("display-p3:0.90000,0.90000,0.90000,0.83000", "srgb:1.00000,1.00000,1.00000,0.41987"),
			want: `{"linear-gradient":["display-p3:0.90000,0.90000,0.90000,0.83000","srgb:1.00000,1.00000,1.00000,0.41987"]}`,
		},
		// A gradient wins over the other two, so a fill given more than one
		// is written the way it is documented to be.
		{"a gradient beside a colour", Fill{Gradient: []string{"srgb:0,0,0,1"}, Solid: "srgb:1,1,1,1"}, `{"linear-gradient":["srgb:0,0,0,1"]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.fill)
			if err != nil {
				t.Fatalf("writing: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("wrote %s, want %s", got, tt.want)
			}
		})
	}
}

func TestFillRejectsNamingNothing(t *testing.T) {
	if _, err := json.Marshal(Fill{}); !errors.Is(err, ErrEmptyFill) {
		t.Errorf("writing an empty fill returned %v, want ErrEmptyFill", err)
	}
}

func TestFillReadsBackWhatItWrote(t *testing.T) {
	for _, want := range []Fill{
		NamedFill("system-dark"),
		SolidFill("srgb:1,0,0,1"),
		GradientFill("srgb:1,1,1,1", "srgb:0,0,0,1"),
	} {
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("writing %v: %v", want, err)
		}
		var got Fill
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("reading %s: %v", data, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("read %v from %s, want %v", got, data, want)
		}
	}
}

func TestSpecializedWritesTheAppearanceOnlyWhenItHasOne(t *testing.T) {
	data, err := json.Marshal([]Specialized[Fill]{
		{Value: NamedFill("system-light")},
		{Appearance: AppearanceDark, Value: NamedFill("system-dark")},
	})
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	want := `[{"value":"system-light"},{"appearance":"dark","value":"system-dark"}]`
	if string(data) != want {
		t.Errorf("wrote %s, want %s", data, want)
	}
}

func TestForFallsBackToTheDefault(t *testing.T) {
	values := []Specialized[string]{
		{Value: "normal"},
		{Appearance: AppearanceTinted, Value: "lighten"},
	}
	for _, tt := range []struct {
		appearance Appearance
		want       string
	}{
		{AppearanceDefault, "normal"},
		{AppearanceTinted, "lighten"},
		{AppearanceDark, "normal"},
	} {
		got, ok := For(values, tt.appearance)
		if !ok || got != tt.want {
			t.Errorf("For(%q) = %q, %v; want %q, true", tt.appearance, got, ok, tt.want)
		}
	}
	if _, ok := For[string](nil, AppearanceDark); ok {
		t.Error("For on nothing reported a value")
	}
	// Without a default there is nothing to fall back to.
	only := []Specialized[string]{{Appearance: AppearanceTinted, Value: "lighten"}}
	if _, ok := For(only, AppearanceDark); ok {
		t.Error("For fell back to an appearance that is not the default")
	}
}
