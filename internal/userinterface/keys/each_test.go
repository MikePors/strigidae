package keys

import "testing"

func TestMessageEach(test *testing.T) {
	tests := []struct {
		name     string
		count    int
		movable  int // how many steps change the position
		wantRuns int
	}{
		{"no count runs once", 0, 100, 1},
		{"count repeats", 5, 100, 5},
		{"stops when a step does not move", 9999, 3, 4},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			position, runs := 0, 0
			Message{Sequence: "j", Count: testCase.count}.Each(
				func() int { return position },
				func() {
					runs++
					if runs <= testCase.movable {
						position++
					}
				},
			)
			if runs != testCase.wantRuns {
				test.Errorf("step ran %d times, want %d", runs, testCase.wantRuns)
			}
		})
	}
}
