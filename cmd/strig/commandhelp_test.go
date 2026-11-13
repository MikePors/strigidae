package main

import (
	"strings"
	"testing"
)

func TestEveryRegisteredCommandHasHelp(test *testing.T) {
	descriptions := viewCommandHelp()
	for _, name := range viewCommands() {
		found := false
		for _, description := range descriptions {
			if description.Name != name {
				continue
			}
			found = true
			if strings.TrimSpace(description.Description) == "" || description.View == "" {
				test.Errorf("command %s has incomplete help: %+v", name, description)
			}
		}
		if !found {
			test.Errorf("registered command %s has no help", name)
		}
	}
}
