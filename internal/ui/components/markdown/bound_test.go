package markdown

import "testing"

func TestCacheStaysBoundedAcrossResizes(t *testing.T) {
	r := New(true)
	for w := 20; w < 120; w++ {
		r.Render("# Title\n\ntext", w)
		r.Render("other body", w)
	}
	if len(r.cache) != 2 {
		t.Errorf("cache size = %d, want 2 (current width only)", len(r.cache))
	}
}
