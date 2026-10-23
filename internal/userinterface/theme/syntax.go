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
	NumberSyntaxClasses
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
func ClassOf(tokenType chroma.TokenType) SyntaxClass {
	for _, key := range []chroma.TokenType{tokenType, tokenType.SubCategory(), tokenType.Category()} {
		if class, ok := tokenClasses[key]; ok {
			return class
		}
	}
	return ClassPlain
}

// colour returns the colour of a class's syntax role.
func (roles Roles) syntaxColour(class SyntaxClass) color.Color {
	switch class {
	case ClassKeyword:
		return roles.SyntaxKeyword
	case ClassString:
		return roles.SyntaxString
	case ClassComment:
		return roles.SyntaxComment
	case ClassNumber:
		return roles.SyntaxNumber
	case ClassFunction:
		return roles.SyntaxFunction
	case ClassType:
		return roles.SyntaxType
	case ClassOperator:
		return roles.SyntaxOperator
	case ClassVariable:
		return roles.SyntaxVariable
	case ClassEscape:
		return roles.SyntaxEscape
	}
	return roles.Text
}

// Syntax holds the foreground-only styles for highlighted code tokens, one
// per SyntaxClass.
type Syntax struct {
	byClass [NumberSyntaxClasses]lipgloss.Style
}

func newSyntax(roles Roles) Syntax {
	var syntax Syntax
	for class := range NumberSyntaxClasses {
		if class == ClassPlain {
			continue // plain code keeps the pane's own text colour
		}
		syntax.byClass[class] = lipgloss.NewStyle().Foreground(roles.syntaxColour(class)).Italic(class == ClassComment)
	}
	return syntax
}

// For returns the style for the given class.
func (syntax Syntax) For(class SyntaxClass) lipgloss.Style { return syntax.byClass[class] }

// chromaMarkup holds the entries that do not follow the class mapping: errors
// and diff markup, which only Chroma (code blocks) knows about.
func chromaMarkup(roles Roles) map[chroma.TokenType]string {
	colorToString := HexString
	return map[chroma.TokenType]string{
		chroma.Error:             colorToString(roles.Error),
		chroma.GenericError:      colorToString(roles.Error),
		chroma.GenericDeleted:    colorToString(roles.Removed),
		chroma.GenericInserted:   colorToString(roles.Added),
		chroma.GenericEmph:       "italic",
		chroma.GenericStrong:     "bold",
		chroma.GenericSubheading: colorToString(roles.Accent),
	}
}

// newChromaStyle generates a Chroma style from the syntax roles. It emits an
// entry for every standard token type, taking the colour of ClassOf(type)'s
// role (plain tokens get the text role), so Chroma's own inheritance never
// decides a colour and the result matches the diff pane. It sets foregrounds
// only: no token gets a background, and there is no Background entry, which
// Chroma would otherwise apply to every token.
func newChromaStyle(name string, roles Roles) *chroma.Style {
	markup := chromaMarkup(roles)
	entries := chroma.StyleEntries{}
	for token := range chroma.StandardTypes {
		if token == chroma.Background {
			continue
		}
		if text, ok := markup[token]; ok {
			entries[token] = text
			continue
		}
		class := ClassOf(token)
		entry := HexString(roles.syntaxColour(class))
		if class == ClassComment {
			entry += " italic"
		}
		entries[token] = entry
	}
	return chroma.MustNewStyle(name, entries)
}

var (
	chromaMutex      sync.Mutex
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
func (theme Theme) ChromaStyleName() string {
	name := "strig-" + theme.Identifier
	chromaMutex.Lock()
	defer chromaMutex.Unlock()
	if !chromaRegistered[name] {
		chromastyles.Register(newChromaStyle(name, theme.Roles))
		chromaRegistered[name] = true
	}
	return name
}
