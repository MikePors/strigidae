package theme

import (
	"image/color"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// Syntax holds the foreground-only styles for highlighted code tokens.
type Syntax struct {
	Keyword, String, Comment, Number, Function, Type, Operator lipgloss.Style
}

func newSyntax(r Roles) Syntax {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	return Syntax{
		Keyword:  fg(r.SynKeyword),
		String:   fg(r.SynString),
		Comment:  fg(r.SynComment).Italic(true),
		Number:   fg(r.SynNumber),
		Function: fg(r.SynFunction),
		Type:     fg(r.SynType),
		Operator: fg(r.SynOperator),
	}
}

// newChromaStyle generates a Chroma style from the syntax roles, following
// the Base16 styling conventions: strings green, keywords magenta, functions
// blue, constants orange, types yellow, comments the muted shade.
func newChromaStyle(name string, r Roles) *chroma.Style {
	c := HexString
	return chroma.MustNewStyle(name, chroma.StyleEntries{
		chroma.Text:                c(r.Text),
		chroma.Error:               c(r.Error),
		chroma.Comment:             c(r.SynComment) + " italic",
		chroma.CommentPreproc:      c(r.SynOperator),
		chroma.Keyword:             c(r.SynKeyword),
		chroma.KeywordType:         c(r.SynType),
		chroma.Operator:            c(r.SynOperator),
		chroma.Punctuation:         c(r.Text),
		chroma.Name:                c(r.Text),
		chroma.NameBuiltin:         c(r.SynFunction),
		chroma.NameTag:             c(r.SynVariable),
		chroma.NameVariable:        c(r.SynVariable),
		chroma.NameAttribute:       c(r.SynType),
		chroma.NameClass:           c(r.SynType),
		chroma.NameNamespace:       c(r.SynType),
		chroma.NameException:       c(r.SynType),
		chroma.NameConstant:        c(r.SynNumber),
		chroma.NameDecorator:       c(r.SynFunction),
		chroma.NameFunction:        c(r.SynFunction),
		chroma.Literal:             c(r.SynNumber),
		chroma.LiteralNumber:       c(r.SynNumber),
		chroma.LiteralString:       c(r.SynString),
		chroma.LiteralStringEscape: c(r.SynEscape),
		chroma.LiteralStringRegex:  c(r.SynEscape),
		chroma.GenericDeleted:      c(r.Removed),
		chroma.GenericInserted:     c(r.Added),
		chroma.GenericEmph:         "italic",
		chroma.GenericStrong:       "bold",
		chroma.GenericSubheading:   c(r.Accent),
	})
}

var (
	chromaMu         sync.Mutex
	chromaRegistered = map[string]bool{}
)

// ChromaStyleName registers the theme's generated Chroma style in Chroma's
// style registry (once per palette) and returns its name. Glamour finds code
// block styles by registry name, so this is the one place the theme package
// touches Chroma's global registry. Call it from the UI goroutine only.
func (t Theme) ChromaStyleName() string {
	name := "strig-" + t.ID
	chromaMu.Lock()
	defer chromaMu.Unlock()
	if !chromaRegistered[name] {
		chromastyles.Register(newChromaStyle(name, t.Roles))
		chromaRegistered[name] = true
	}
	return name
}
