package theme

import (
	"image/color"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// SyntaxClass is a coarse token category. Highlighters report classes rather
// than colours, so cached results stay valid when the theme changes. Every
// class has one syntax role, and the diff pane and the generated Chroma style
// (Markdown code blocks) both resolve token types through ClassOf, so the same
// code is coloured identically in both.
type SyntaxClass int

// Syntax classes.
const (
	ClassPlain SyntaxClass = iota
	ClassKeyword
	ClassString
	ClassComment
	ClassNumber // numbers and constants
	ClassFunction
	ClassType
	ClassOperator
	ClassVariable // variables and tags
	ClassEscape   // string escapes and regular expressions
	NumSyntaxClasses
)

// tokenClasses is the one mapping from Chroma token types to classes. A token
// type not listed here inherits from its subcategory, then its category, then
// is plain.
var tokenClasses = map[chroma.TokenType]SyntaxClass{
	chroma.Keyword:             ClassKeyword,
	chroma.OperatorWord:        ClassKeyword,
	chroma.KeywordType:         ClassType,
	chroma.Operator:            ClassOperator,
	chroma.Comment:             ClassComment,
	chroma.CommentPreproc:      ClassOperator,
	chroma.LiteralString:       ClassString,
	chroma.LiteralStringEscape: ClassEscape,
	chroma.LiteralStringRegex:  ClassEscape,
	chroma.Literal:             ClassNumber,
	chroma.LiteralNumber:       ClassNumber,
	chroma.NameConstant:        ClassNumber,
	chroma.NameBuiltin:         ClassFunction,
	chroma.NameFunction:        ClassFunction,
	chroma.NameFunctionMagic:   ClassFunction,
	chroma.NameDecorator:       ClassFunction,
	chroma.NameClass:           ClassType,
	chroma.NameNamespace:       ClassType,
	chroma.NameException:       ClassType,
	chroma.NameAttribute:       ClassType,
	chroma.NameTag:             ClassVariable,
	chroma.NameVariable:        ClassVariable,
}

// ClassOf returns the syntax class of a Chroma token type.
func ClassOf(t chroma.TokenType) SyntaxClass {
	for _, k := range []chroma.TokenType{t, t.SubCategory(), t.Category()} {
		if c, ok := tokenClasses[k]; ok {
			return c
		}
	}
	return ClassPlain
}

// colour returns the colour of a class's syntax role.
func (r Roles) syntaxColour(c SyntaxClass) color.Color {
	switch c {
	case ClassKeyword:
		return r.SynKeyword
	case ClassString:
		return r.SynString
	case ClassComment:
		return r.SynComment
	case ClassNumber:
		return r.SynNumber
	case ClassFunction:
		return r.SynFunction
	case ClassType:
		return r.SynType
	case ClassOperator:
		return r.SynOperator
	case ClassVariable:
		return r.SynVariable
	case ClassEscape:
		return r.SynEscape
	}
	return r.Text
}

// Syntax holds the foreground-only styles for highlighted code tokens, one
// per SyntaxClass.
type Syntax struct {
	byClass [NumSyntaxClasses]lipgloss.Style
}

func newSyntax(r Roles) Syntax {
	var s Syntax
	for c := range NumSyntaxClasses {
		if c == ClassPlain {
			continue // plain code keeps the pane's own text colour
		}
		s.byClass[c] = lipgloss.NewStyle().Foreground(r.syntaxColour(c)).Italic(c == ClassComment)
	}
	return s
}

// For returns the style for class c.
func (s Syntax) For(c SyntaxClass) lipgloss.Style { return s.byClass[c] }

// newChromaStyle generates a Chroma style from the syntax roles: every entry
// of tokenClasses takes its class's colour. The few entries with no class
// (plain text, errors, diff markup) come from other roles.
func newChromaStyle(name string, r Roles) *chroma.Style {
	c := HexString
	entries := chroma.StyleEntries{
		chroma.Text:              c(r.Text),
		chroma.Punctuation:       c(r.Text),
		chroma.Name:              c(r.Text),
		chroma.Error:             c(r.Error),
		chroma.GenericDeleted:    c(r.Removed),
		chroma.GenericInserted:   c(r.Added),
		chroma.GenericEmph:       "italic",
		chroma.GenericStrong:     "bold",
		chroma.GenericSubheading: c(r.Accent),
	}
	for tok, class := range tokenClasses {
		entry := c(r.syntaxColour(class))
		if class == ClassComment {
			entry += " italic"
		}
		entries[tok] = entry
	}
	return chroma.MustNewStyle(name, entries)
}

var (
	chromaMu         sync.Mutex
	chromaRegistered = map[string]bool{}
)

// ChromaStyleName registers the theme's generated Chroma style in Chroma's
// style registry (once per theme ID) and returns its name. Glamour finds code
// block styles by registry name, so this is the one place the theme package
// writes Chroma's global registry.
//
// The mutex only guards chromaRegistered. Chroma's registry is a plain map
// that Glamour reads without a lock on every code block it renders, so it is
// not what makes this safe. The rule is that this method and every Glamour
// render run on the UI goroutine (never in a tea.Cmd).
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
