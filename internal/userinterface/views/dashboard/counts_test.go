package dashboard

import (
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

func pressCounted(view *View, sequence string, count int) {
	view.Update(keys.Message{Sequence: sequence, Count: count})
}

func TestCounts(test *testing.T) {
	tests := []struct {
		name  string
		start []string
		key   string
		count int
		want  string
	}{
		{"2j moves two rows", nil, "j", 2, "rivka/dotfiles#3"},
		{"j without a count moves once", nil, "j", 0, "acme/gadgets#7"},
		{"a count larger than the list clamps", nil, "j", 9999, "rivka/dotfiles#3"},
		{"a count larger than the list clamps up", []string{"G"}, "k", 9999, "acme/widgets#12"},
		{"2G goes to the second row", nil, "G", 2, "acme/gadgets#7"},
		{"42G clamps to the last row", nil, "G", 42, "rivka/dotfiles#3"},
		{"5] stops at the last section with rows", nil, "]", 5, "rivka/dotfiles#3"},
		{"5[ stops at the first section", []string{"G"}, "[", 5, "acme/widgets#12"},
		{"a count on g t acts once", nil, "g t", 5, "acme/widgets#12"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view, _ := loadedView(test)
			for _, key := range testCase.start {
				press(view, key)
			}
			pressCounted(view, testCase.key, testCase.count)
			if testCase.key == "g t" {
				if view.active != 1 {
					test.Fatalf("active tab = %d, want 1", view.active)
				}
				pressCounted(view, "g T", 1)
			}
			if got := selectedKey(test, view); got != testCase.want {
				test.Errorf("selected %s, want %s", got, testCase.want)
			}
		})
	}
}
