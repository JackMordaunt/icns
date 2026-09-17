package main

import (
	"flag"
	"fmt"
	"strconv"
)

// version is stamped by goreleaser at build time.
var version = "master"

// option records a flag registered under a long and a short name so usage
// can list each one once, GNU style, instead of twice as PrintDefaults would.
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

func usage() {
	w := flag.CommandLine.Output()
	fmt.Fprintf(w, "icnsify %s\n\nUsage: icnsify [-i input] [-o output] [-r quality]\n\nOptions:\n", version)
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
`)
}
