package diffview

import (
	"strings"
	"testing"
)

func threadWalk(model *Model, step func(), presses int) string {
	var landed []string
	for range presses {
		step()
		text := cursorRowText(model)
		name := "-"
		for _, author := range []string{"alice", "carol", "dave", "erin", "frank"} {
			if strings.Contains(text, author) {
				name = author
			}
		}
		landed = append(landed, name)
	}
	return strings.Join(landed, ",")
}

func TestSkipResolvedThreadNavigation(test *testing.T) {
	testCases := []struct {
		name         string
		skipResolved bool
		forward      string
		backward     string
	}{
		{"off", false, "carol,alice,dave,erin,frank", "erin,dave,alice,carol,carol"},
		// erin's thread is outdated but unresolved, so it is still visited.
		{"on", true, "alice,dave,erin,frank,frank", "erin,dave,alice,alice"},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			model := threadModel(test, 100, 80)
			model.SetSkipResolved(testCase.skipResolved)
			model.GotoTop()
			steps := strings.Count(testCase.forward, ",") + 1
			if got := threadWalk(model, model.NextThread, steps); got != testCase.forward {
				test.Errorf("forward = %s, want %s", got, testCase.forward)
			}
			// From frank, the last thread, back up.
			steps = strings.Count(testCase.backward, ",") + 1
			if got := threadWalk(model, model.PreviousThread, steps); got != testCase.backward {
				test.Errorf("backward = %s, want %s", got, testCase.backward)
			}
		})
	}
}
