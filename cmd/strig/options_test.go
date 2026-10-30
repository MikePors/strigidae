package main

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/options"
)

// TestConfigOptionsReachTheUserInterface fails if the options a config file
// sets stop being handed to the shell.
func TestConfigOptionsReachTheUserInterface(test *testing.T) {
	configuration, err := config.Parse("c.yml", []byte("wrapscan: false\nhlsearch: false\n"))
	if err != nil {
		test.Fatal(err)
	}
	got := application{config: configuration}.userInterfaceOptions(nil, "a notice")
	if diff := cmp.Diff(options.Values{"wrapscan": false, "hlsearch": false}, got.StartingOptions); diff != "" {
		test.Errorf("starting options (-want +got):\n%s", diff)
	}
	if !slices.Contains(got.ViewCommands, "merge") {
		test.Errorf("view commands = %v, want the workspace commands", got.ViewCommands)
	}
	if got.Notice != "a notice" {
		test.Errorf("notice = %q", got.Notice)
	}
}
