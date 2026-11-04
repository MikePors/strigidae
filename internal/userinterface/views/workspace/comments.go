package workspace

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/MikePors/strigidae/internal/userinterface/components/scroll"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

type commentsPane struct {
	dirty                bool
	initialized          bool
	viewport             viewport.Model
	search               search.State
	items                []conversationItem
	window               scroll.Window
	selected             string
	unresolved           bool
	starts               []int
	total, open, drafts  int
	renderedSelected     string
	renderedStart        int
	matchingCommentStart int
}
