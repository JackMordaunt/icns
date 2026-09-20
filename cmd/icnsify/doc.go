package main

import (
	"flag"
	"fmt"
	"strconv"
)

// Stamped by goreleaser at build time, through the linker.
var (
	version = "master"
	commit  = ""
	date    = ""
	builtBy = ""
)

// buildInfo renders the version and whatever else the build stamped.
func buildInfo() string {
	out := "icnsify " + version
	for _, part := range []struct{ label, value string }{
		{"commit", commit},
		{"built", date},
		{"by", builtBy},
	} {
		if part.value != "" {
			out += fmt.Sprintf(", %s %s", part.label, part.value)
		}
	}
	return out
}

// option records a flag registered under both a long and a short name, so
// usage can list the pair once, GNU style.
type option struct {
	long, short string
	def, usage  string
}

var options []option

// stringFlag registers a string option reachable as --long or -short.
func stringFlag(p *string, long, short, def, usage string) {
	flag.StringVar(p, long, def, usage)
	flag.StringVar(p, short, def, usage)
	options = append(options, option{long: long, short: short, def: def, usage: usage})
}

// intFlag registers an integer option reachable as --long or -short.
func intFlag(p *int, long, short string, def int, usage string) {
	flag.IntVar(p, long, def, usage)
	flag.IntVar(p, short, def, usage)
	options = append(options, option{long: long, short: short, def: strconv.Itoa(def), usage: usage})
}

// boolFlag registers a boolean option reachable as --long or -short.
func boolFlag(p *bool, long, short string, usage string) {
	flag.BoolVar(p, long, false, usage)
	flag.BoolVar(p, short, false, usage)
	options = append(options, option{long: long, short: short, usage: usage})
}

func usage() {
	w := flag.CommandLine.Output()
	fmt.Fprintf(w, "%s\n\nUsage: icnsify [-i input] [-o output] [-f format] [-r quality] [-c]\n\nOptions:\n", buildInfo())
	for _, o := range options {
		fmt.Fprintf(w, "  -%s, --%s\n        %s", o.short, o.long, o.usage)
		if o.def != "" && o.def != "0" {
			fmt.Fprintf(w, " (default %s)", o.def)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprint(w, `
You can also pipe to stdin and from stdout. Pipes are detected automatically
when --input is not given, and --output is then ignored.

        cat icon.png | icnsify > icon.icns
        cat icon.icns | icnsify > icon.png
        cat icon.png | icnsify -f ico > icon.ico
`)
}
