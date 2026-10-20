// Command strig is a terminal UI for reviewing GitHub pull requests.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/MikePors/strigidae/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("strig", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintf(stdout, "strig %s\n", version.String())
		return 0
	}

	fmt.Fprintln(stderr, "strig: nothing to see yet; try --version")
	return 1
}
