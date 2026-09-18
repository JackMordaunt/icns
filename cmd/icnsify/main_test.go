package main

import "testing"

// TestTarget covers how the output format is chosen, since a wrong choice
// writes one format under another's name.
func TestTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc   string
		want   string // --format
		out    string // output path
		piping bool
		got    string // what the input turned out to be
		expect string
	}{
		{"the output path names it", "", "icon.ico", false, "png", ".ico"},
		{"an icns from artwork", "", "icon.icns", false, "png", ".icns"},
		{"a plain image out of a container", "", "icon.png", false, "icns", ".png"},
		{"the flag wins over the path", "ico", "icon.icns", false, "png", ".ico"},
		{"the flag takes a dot", ".jpg", "", false, "icns", ".jpg"},
		{"the flag takes capitals", "ICO", "", false, "png", ".ico"},
		{"a pipe packs artwork", "", "", true, "png", ".icns"},
		{"a pipe unpacks an icns", "", "", true, "icns", ".png"},
		{"a pipe unpacks an ico", "", "", true, "ico", ".png"},
		{"a pipe takes the flag", "ico", "", true, "png", ".ico"},
		{"an extension we cannot write, from artwork", "", "icon.gif", false, "png", ".icns"},
		{"an extension we cannot write, from a container", "", "icon.gif", false, "icns", ".png"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			if got := target(tt.want, tt.out, tt.piping, tt.got); got != tt.expect {
				st.Errorf("target = %s, want %s", got, tt.expect)
			}
		})
	}
}

func TestSanitiseInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc   string
		in     string
		out    string
		format string
		expect string
	}{
		{"artwork defaults to icns", "icon.png", "", "", "icon.icns"},
		{"the format flag sets the default", "icon.png", "", "ico", "icon.ico"},
		{"an icns defaults to png", "icon.icns", "", "", "icon.png"},
		{"an ico defaults to png", "icon.ico", "", "", "icon.png"},
		{"an icns converts to an ico", "icon.icns", "", "ico", "icon.ico"},
		{"a name without an extension gains one", "icon.png", "out", "", "out.icns"},
		{"a name without an extension, unpacking", "icon.icns", "out", "", "out.png"},
		{"the output path is kept", "icon.png", "out.ico", "", "out.ico"},
		{"the output lands beside us", "path/to/icon.png", "", "", "icon.icns"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			_, out, _ := sanitiseInputs(tt.in, tt.out, tt.format, 5)
			if out != tt.expect {
				st.Errorf("output = %s, want %s", out, tt.expect)
			}
		})
	}
}

func TestSanitiseInputsClampsQuality(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		resize int
		expect int
	}{{-1, 0}, {0, 0}, {3, 3}, {5, 5}, {9, 5}} {
		if _, _, got := sanitiseInputs("icon.png", "", "", tt.resize); int(got) != tt.expect {
			t.Errorf("resize %d became %d, want %d", tt.resize, got, tt.expect)
		}
	}
}
