package main

import (
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface/views/dashboard"
	"github.com/MikePors/strigidae/internal/userinterface/views/pullrequestlist"
	"github.com/MikePors/strigidae/internal/userinterface/views/workspace"
)

// sharedActionNames are names more than one view uses on purpose: the same
// action in another view, so a config override rebinds both. Any other name in
// two views is a clash.
var sharedActionNames = map[string]bool{
	"next-section":     true, // workspace Overview and dashboard: next section
	"previous-section": true,
	"next-tab":         true, // dashboard and pull request list: next tab
	"previous-tab":     true,
}

// This lives in cmd/strig because it is the one package that imports every
// view; a new view that declares actions adds one line here.
func TestActionNamesAreUniqueAcrossViews(test *testing.T) {
	views := map[string][]string{
		"workspace": workspace.ActionNames(),
		"dashboard": dashboard.ActionNames(),
		"list":      pullrequestlist.ActionNames(),
	}
	usedBy := map[string][]string{}
	for view, names := range views {
		for _, name := range names {
			usedBy[name] = append(usedBy[name], view)
		}
	}
	for name, owners := range usedBy {
		if len(owners) > 1 && !sharedActionNames[name] {
			test.Errorf("action name %q is used by %v; rename one, or add it to sharedActionNames if it is the same action", name, owners)
		}
	}
}
