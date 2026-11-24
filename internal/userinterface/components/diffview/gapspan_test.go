package diffview

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGapsAreLaidOutRightAroundAnyHunkShape(test *testing.T) {
	// Each file is 8 lines. The gaps are opened one after the other, so what
	// is shown is every unchanged line, written old:new.
	for _, testCase := range []struct {
		name  string
		patch string
		want  []string
	}{
		{"pure insertion", "@@ -3,0 +4,2 @@\n+x\n+y\n", []string{"1:1", "2:2", "3:3", "4:6", "5:7", "6:8"}},
		{"pure deletion", "@@ -3,2 +2,0 @@\n-a\n-b\n", []string{"1:1", "2:2", "5:3", "6:4", "7:5", "8:6", "9:7", "10:8"}},
		{"insertion at the start of the file", "@@ -0,0 +1,1 @@\n+x\n", []string{"1:2", "2:3", "3:4", "4:5", "5:6", "6:7", "7:8"}},
		{"change at the start of the file", "@@ -1,2 +1,2 @@\n a\n-b\n+B\n", []string{"3:3", "4:4", "5:5", "6:6", "7:7", "8:8"}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			model := gapModelWithText(test, testCase.patch, 8)
			for index := range len(model.file.Hunks) + 1 {
				for model.gapRowIndex(index) >= 0 {
					goToGap(test, model, index)
					model.ExpandGap()
				}
			}
			var got []string
			for _, line := range shownLines(model) {
				if want := "line " + strconv.Itoa(line.New); line.Text != want {
					test.Errorf("line %d shows %q, want %q", line.New, line.Text, want)
				}
				got = append(got, fmt.Sprintf("%d:%d", line.Old, line.New))
			}
			if difference := cmp.Diff(testCase.want, got); difference != "" {
				test.Errorf("lines shown, old:new (-want +got):\n%s", difference)
			}
		})
	}
}
