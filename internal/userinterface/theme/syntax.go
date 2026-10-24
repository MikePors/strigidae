package theme

import (
	"fmt"
	"image/color"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// SyntaxClass is a coarse token category. Every class has one syntax role:
// a generated style colours each token type by its class (ClassOf), and a
// syn-<class> override recolours every token type in its class.
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

// tokenStyle is how one token type is drawn: a foreground and attributes,
// never a background.
type tokenStyle struct {
	colour                  color.Color // nil: the pane's own text colour
	bold, italic, underline bool
}

// Syntax is the theme's syntax highlighting: one Chroma style, either
// generated from the syntax roles or a named bundled style, resolved into an
// entry for every standard token type. Markdown code blocks draw with the
// Chroma style and the diff pane with the Lip Gloss style per token type
// built from the same entries, so both panes colour code identically.
type Syntax struct {
	name   string // the named Chroma style, or "" when generated
	styles map[chroma.TokenType]lipgloss.Style
	chroma *chroma.Style
}

// Name returns the named Chroma style the theme uses, or "" when the style
// is generated from the palette.
func (syntax Syntax) Name() string { return syntax.name }

// For returns the foreground-only style for a token type. A type that is not
// a standard one takes its subcategory's, then its category's style.
func (syntax Syntax) For(tokenType chroma.TokenType) lipgloss.Style {
	for _, key := range []chroma.TokenType{tokenType, tokenType.SubCategory(), tokenType.Category(), chroma.Text} {
		if style, ok := syntax.styles[key]; ok {
			return style
		}
	}
	return lipgloss.NewStyle()
}

// TokenTypes lists every token type For has its own style for.
func (syntax Syntax) TokenTypes() []chroma.TokenType {
	types := make([]chroma.TokenType, 0, len(syntax.styles))
	for tokenType := range syntax.styles {
		types = append(types, tokenType)
	}
	slices.Sort(types)
	return types
}

// newSyntax resolves every standard token type, from the named style when
// one is given and from the syntax roles otherwise, then builds the Chroma
// style registered as chromaName and the diff pane's Lip Gloss styles from
// the result.
func newSyntax(name, chromaName string, roles Roles, overridden map[string]bool) Syntax {
	var tokens map[chroma.TokenType]tokenStyle
	if named := bundledChromaStyles[name]; named != nil {
		tokens = namedTokens(name, named, roles, overridden)
	} else {
		tokens = generatedTokens(roles)
	}
	syntax := Syntax{name: name, styles: map[chroma.TokenType]lipgloss.Style{}}
	builder := chroma.NewStyleBuilder(chromaName)
	for tokenType, token := range tokens {
		style := lipgloss.NewStyle().Bold(token.bold).Italic(token.italic).Underline(token.underline)
		colour := token.colour
		if colour != nil {
			style = style.Foreground(colour)
		} else {
			colour = roles.Text // Chroma has no "the pane's colour"
		}
		syntax.styles[tokenType] = style
		builder.AddEntry(tokenType, chroma.StyleEntry{
			Colour:    chroma.ParseColour(HexString(colour)),
			Bold:      trilean(token.bold),
			Italic:    trilean(token.italic),
			Underline: trilean(token.underline),
		})
	}
	style, err := builder.Build()
	if err != nil { // only string entries can fail to parse
		panic(err)
	}
	syntax.chroma = style
	return syntax
}

// trilean says yes or no outright: Pass would let Chroma inherit the
// attribute from the token's category, which the diff pane would not show.
func trilean(on bool) chroma.Trilean {
	if on {
		return chroma.Yes
	}
	return chroma.No
}

// standardTypes lists the token types a theme resolves: every standard one
// except Background, whose page colour strig never takes from a style.
func standardTypes() []chroma.TokenType {
	var types []chroma.TokenType
	for tokenType := range chroma.StandardTypes {
		if tokenType != chroma.Background {
			types = append(types, tokenType)
		}
	}
	return types
}

// generatedTokens gives every token type the colour of ClassOf(type)'s role,
// so Chroma's own inheritance never decides a colour. Plain tokens keep the
// pane's text colour, and the few markup entries (errors, diff markup) that
// do not follow the class mapping come from chromaMarkup.
func generatedTokens(roles Roles) map[chroma.TokenType]tokenStyle {
	markup := chromaMarkup(roles)
	tokens := map[chroma.TokenType]tokenStyle{}
	for _, tokenType := range standardTypes() {
		if token, ok := markup[tokenType]; ok {
			tokens[tokenType] = token
			continue
		}
		class := ClassOf(tokenType)
		token := tokenStyle{italic: class == ClassComment}
		if class != ClassPlain {
			token.colour = roles.syntaxColour(class)
		}
		tokens[tokenType] = token
	}
	return tokens
}

// chromaMarkup holds the generated entries that do not follow the class
// mapping: errors and diff markup.
func chromaMarkup(roles Roles) map[chroma.TokenType]tokenStyle {
	return map[chroma.TokenType]tokenStyle{
		chroma.Error:             {colour: roles.Error},
		chroma.GenericError:      {colour: roles.Error},
		chroma.GenericDeleted:    {colour: roles.Removed},
		chroma.GenericInserted:   {colour: roles.Added},
		chroma.GenericEmph:       {italic: true},
		chroma.GenericStrong:     {bold: true},
		chroma.GenericSubheading: {colour: roles.Accent},
	}
}

// namedTokens takes every token type's foreground and attributes from a named
// style, resolved through Chroma's inheritance; the style's backgrounds are
// ignored. A foreground that is unreadable on the theme's background is
// nudged and logged, like a derived role. A syn-<class> role the theme
// overrides then recolours every token type in that class, exactly as
// written.
func namedTokens(name string, named *chroma.Style, roles Roles, overridden map[string]bool) map[chroma.TokenType]tokenStyle {
	tokens := map[chroma.TokenType]tokenStyle{}
	warned := map[string]bool{}
	for _, tokenType := range standardTypes() {
		entry := named.Get(tokenType)
		token := tokenStyle{
			colour:    roles.Text,
			bold:      entry.Bold == chroma.Yes,
			italic:    entry.Italic == chroma.Yes,
			underline: entry.Underline == chroma.Yes,
		}
		if entry.Colour.IsSet() {
			token.colour = mustParseHex(entry.Colour.String())
		}
		if ratio := contrast(token.colour, roles.Background); ratio < minimumAccentContrast {
			hex := HexString(token.colour)
			if !warned[hex] {
				warned[hex] = true
				slog.Warn("syntax style colour is hard to read", "syntax", name, "colour", hex,
					"contrast", math.Round(ratio*100)/100, "want", minimumAccentContrast)
			}
			token.colour = nudge(token.colour, roles.Background, minimumAccentContrast)
		}
		if class := ClassOf(tokenType); class != ClassPlain && overridden[syntaxRoleNames[class]] {
			token.colour = roles.syntaxColour(class)
		}
		tokens[tokenType] = token
	}
	return tokens
}

// syntaxRoleNames is the role each class takes its colour from.
var syntaxRoleNames = [NumberSyntaxClasses]string{
	ClassKeyword:  "syn-keyword",
	ClassString:   "syn-string",
	ClassComment:  "syn-comment",
	ClassNumber:   "syn-number",
	ClassFunction: "syn-function",
	ClassType:     "syn-type",
	ClassOperator: "syn-operator",
	ClassVariable: "syn-variable",
	ClassEscape:   "syn-escape",
}

func mustParseHex(text string) color.Color {
	colour, err := ParseHex(text)
	if err != nil { // Chroma formats every colour as #rrggbb
		panic(err)
	}
	return colour
}

// generatedPrefix starts the registry name of every style strig generates.
const generatedPrefix = "strig-"

// bundledChromaStyles is a copy of Chroma's registry taken when the package
// starts, before ChromaStyleName registers anything, so it holds only the
// styles Chroma bundles and never strig's own. Unlike the registry, it can be
// read from any goroutine (theme files load in a tea.Cmd).
var bundledChromaStyles = maps.Clone(chromastyles.Registry)

// SyntaxStyleNames lists the bundled Chroma styles a theme's syntax may name,
// sorted.
func SyntaxStyleNames() []string {
	names := make([]string, 0, len(bundledChromaStyles))
	for name := range bundledChromaStyles {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// CheckSyntaxStyle reports whether name is a bundled Chroma style. An unknown
// name is an error that lists the bundled ones.
func CheckSyntaxStyle(name string) error {
	if bundledChromaStyles[name] == nil {
		return fmt.Errorf("unknown syntax style %q (bundled styles: %s)", name, strings.Join(SyntaxStyleNames(), ", "))
	}
	return nil
}

var (
	chromaMutex      sync.Mutex
	chromaRegistered = map[string]bool{}
)

// ChromaStyleName registers the theme's Chroma style in Chroma's style
// registry (once per theme Identifier) and returns its name. Glamour finds
// code block styles by registry name, so this is the one place the theme
// package writes Chroma's global registry. A named style is registered as a
// copy under strig's own name, since its resolved entries differ from the
// original (no backgrounds, guarded colours, overrides).
//
// The mutex only guards chromaRegistered. Chroma's registry is a plain map
// that Glamour reads without a lock on every code block it renders, so it is
// not what makes this safe. The rule is that this method and every Glamour
// render run on the UI goroutine (never in a tea.Cmd).
func (theme Theme) ChromaStyleName() string {
	name := theme.Syntax.chroma.Name
	chromaMutex.Lock()
	defer chromaMutex.Unlock()
	if !chromaRegistered[name] {
		chromastyles.Register(theme.Syntax.chroma)
		chromaRegistered[name] = true
	}
	return name
}
