package main

import (
	"fmt"

	"github.com/spf13/pflag"
)

var version = "master"

func usage() {
	fmt.Printf("\n")
	pflag.Usage()
	fmt.Printf(`
You can also pipe to stdin and from stdout. 
Pipes will be detected automatically.
'--input' and '--output' will override the respective pipes.

	cat icon.png | icnsify > icon.icns

`)
}
