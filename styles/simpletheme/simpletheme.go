// Package simpletheme makes a whynot.StyleSheet from a fixed set of
// fields: margins, text styles and colors for each kind of Markdown
// element.
//
// Use a ready-made stylesheet:
//
//	view := whynot.NewView(doc, faces, whynot.WithStyleSheet(simpletheme.DarkStyleSheet))
//
// or start from a preset and change what you need:
//
//	theme := simpletheme.Dark()
//	theme.LinkColor = color.RGBA{0xFF, 0x40, 0x40, 0xFF}
//	view.SetStyleSheet(theme.StyleSheet())
package simpletheme

import (
	"image/color"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/internal/styling"
)

// DarkStyleSheet and LightStyleSheet are the Dark and Light presets, ready
// to use.
var (
	DarkStyleSheet  = Dark().StyleSheet()
	LightStyleSheet = Light().StyleSheet()
)

// Theme describes how a document looks. Margins are in logical (unscaled)
// pixels. Each TextStyle only sets its non-zero fields, inheriting the
// rest from the enclosing element, and ultimately from BaseTextStyle.
type Theme struct {
	ParagraphMargins   whynot.Margins
	ParagraphTextStyle TextStyle

	// HeadingMargins and HeadingTextStyles are indexed by heading level
	// minus one: index 0 is a level 1 heading (#).
	HeadingMargins    [6]whynot.Margins
	HeadingTextStyles [6]TextStyle

	ListMargins       whynot.Margins
	ListItemMargins   whynot.Margins
	ListItemTextStyle TextStyle

	CodeBlockMargins   whynot.Margins
	CodeBlockTextStyle TextStyle
	CodeBlockColor     color.Color

	// UnsupportedColor is the text color of Markdown whynot doesn't
	// support, shown as a code block.
	UnsupportedColor color.Color

	CodeSpanTextStyle TextStyle
	CodeSpanColor     color.Color
	EmphasisTextStyle TextStyle
	StrongTextStyle   TextStyle

	ThematicBreakMargins whynot.Margins
	ThematicBreakColor   color.Color

	LinkColor color.Color

	BlockquoteMargins  whynot.Margins
	BlockquoteBarColor color.Color

	TableCellTextStyle TextStyle
	TableMargins       whynot.Margins
	TableFrameColor    color.Color

	// ImagePlaceholderColor fills the space of an image that's still
	// loading.
	ImagePlaceholderColor color.Color

	// TextColor is the color of text with no color of its own.
	TextColor color.Color
	// BaseTextStyle is where every text style inherits from; a field it
	// leaves zero takes a built-in default.
	BaseTextStyle TextStyle

	// BackgroundColor fills the whole view.
	BackgroundColor color.Color
	// ViewMargins is the space between the view's edge and the document.
	ViewMargins whynot.Margins
	// HighlightColor is the text color of a hovered link.
	HighlightColor color.Color

	ScrollbarColors ScrollbarColors
	// SyntaxColors colors code tokens, when a Highlighter is used (see
	// whynot.WithSyntaxHighlighter).
	SyntaxColors SyntaxColors

	// LineHeight is the space a line of text takes, as a multiple of its
	// font's natural height: 1.2 adds 20%.
	LineHeight float64
}

// ScrollbarColors is a scrollbar thumb's color when idle, hovered, and
// being dragged.
type ScrollbarColors struct {
	Idle, Hover, Pressed color.Color
}

// SyntaxColors is the color of each kind of code token (see
// whynot.TokenClass).
type SyntaxColors struct {
	Keyword  color.Color
	Type     color.Color
	Function color.Color
	String   color.Color
	Number   color.Color
	Comment  color.Color
}

// TextStyle is a partial text style: a zero field inherits from the
// enclosing element.
type TextStyle struct {
	// Size is in logical (unscaled) points.
	Size   float64
	Style  Style
	Weight Weight
	Family Family
}

// Style is a font style; zero inherits.
type Style int

const (
	StyleInherit Style = iota
	StyleNormal
	StyleItalic
	StyleOblique
)

// Weight is a font weight; zero inherits.
type Weight int

const (
	WeightInherit Weight = iota
	WeightThin
	WeightExtraLight
	WeightLight
	WeightNormal
	WeightMedium
	WeightSemiBold
	WeightBold
	WeightExtraBold
	WeightBlack
)

// Family is a font family; zero inherits.
type Family int

const (
	FamilyInherit Family = iota
	Proportional
	Monospace
	SmallCaps
)

// Dark returns whynot's default look, light text on a dark background, to
// start from.
func Dark() *Theme {
	return fromBasic(styling.Dark())
}

// Light returns whynot's light look, dark text on a light background, to
// start from.
func Light() *Theme {
	return fromBasic(styling.Light())
}

// StyleSheet returns a snapshot of t, for a whynot.View: changing t
// afterwards doesn't affect it.
func (t *Theme) StyleSheet() whynot.StyleSheet {
	b := styling.Dark() // for the dimensions a Theme doesn't expose
	b.ParagraphMargins = t.ParagraphMargins
	b.ParagraphTextStyle = t.ParagraphTextStyle.partial()
	b.HeadingMargins = t.HeadingMargins
	for i, s := range t.HeadingTextStyles {
		b.HeadingTextStyles[i] = s.partial()
	}
	b.ListMargins = t.ListMargins
	b.ListItemMargins = t.ListItemMargins
	b.ListItemTextStyle = t.ListItemTextStyle.partial()
	b.CodeBlockMargins = t.CodeBlockMargins
	b.CodeBlockTextStyle = t.CodeBlockTextStyle.partial()
	b.CodeBlockColor = t.CodeBlockColor
	b.UnsupportedColor = t.UnsupportedColor
	b.CodeSpanTextStyle = t.CodeSpanTextStyle.partial()
	b.CodeSpanColor = t.CodeSpanColor
	b.EmphasisTextStyle = t.EmphasisTextStyle.partial()
	b.StrongTextStyle = t.StrongTextStyle.partial()
	b.ThematicBreakMargins = t.ThematicBreakMargins
	b.ThematicBreakColor = t.ThematicBreakColor
	b.LinkColor = t.LinkColor
	b.BlockquoteMargins = t.BlockquoteMargins
	b.BlockquoteBarColor = t.BlockquoteBarColor
	b.TableCellTextStyle = t.TableCellTextStyle.partial()
	b.TableMargins = t.TableMargins
	b.TableFrameColor = t.TableFrameColor
	b.ImagePlaceholderColor = t.ImagePlaceholderColor
	b.TextColor = t.TextColor
	b.BaseTextStyle = t.BaseTextStyle.over(b.BaseTextStyle)
	b.Background = t.BackgroundColor
	b.ViewMargin = t.ViewMargins
	b.Highlight = t.HighlightColor
	b.Scrollbar = styling.ScrollbarColors(t.ScrollbarColors)
	b.Syntax = styling.SyntaxColors(t.SyntaxColors)
	if t.LineHeight != 0 {
		b.Dims.LineHeight = t.LineHeight
	}
	return b
}

func fromBasic(b *styling.Basic) *Theme {
	t := &Theme{
		ParagraphMargins:      b.ParagraphMargins,
		ParagraphTextStyle:    fromPartial(b.ParagraphTextStyle),
		HeadingMargins:        b.HeadingMargins,
		ListMargins:           b.ListMargins,
		ListItemMargins:       b.ListItemMargins,
		ListItemTextStyle:     fromPartial(b.ListItemTextStyle),
		CodeBlockMargins:      b.CodeBlockMargins,
		CodeBlockTextStyle:    fromPartial(b.CodeBlockTextStyle),
		CodeBlockColor:        b.CodeBlockColor,
		UnsupportedColor:      b.UnsupportedColor,
		CodeSpanTextStyle:     fromPartial(b.CodeSpanTextStyle),
		CodeSpanColor:         b.CodeSpanColor,
		EmphasisTextStyle:     fromPartial(b.EmphasisTextStyle),
		StrongTextStyle:       fromPartial(b.StrongTextStyle),
		ThematicBreakMargins:  b.ThematicBreakMargins,
		ThematicBreakColor:    b.ThematicBreakColor,
		LinkColor:             b.LinkColor,
		BlockquoteMargins:     b.BlockquoteMargins,
		BlockquoteBarColor:    b.BlockquoteBarColor,
		TableCellTextStyle:    fromPartial(b.TableCellTextStyle),
		TableMargins:          b.TableMargins,
		TableFrameColor:       b.TableFrameColor,
		ImagePlaceholderColor: b.ImagePlaceholderColor,
		TextColor:             b.TextColor,
		BaseTextStyle:         fromPartial(styling.PartialTextStyle{TextStyle: b.BaseTextStyle, Set: styling.AllTextStyleFields}),
		BackgroundColor:       b.Background,
		ViewMargins:           b.ViewMargin,
		HighlightColor:        b.Highlight,
		ScrollbarColors:       ScrollbarColors(b.Scrollbar),
		SyntaxColors:          SyntaxColors(b.Syntax),
		LineHeight:            b.Dims.LineHeight,
	}
	for i, s := range b.HeadingTextStyles {
		t.HeadingTextStyles[i] = fromPartial(s)
	}
	return t
}

// partial converts s to the engine's form, flagging its non-zero fields.
func (s TextStyle) partial() styling.PartialTextStyle {
	var p styling.PartialTextStyle
	if s.Size != 0 {
		p.Size = s.Size
		p.Set |= styling.FieldSize
	}
	if s.Style != StyleInherit {
		p.Style = font.Style(s.Style - StyleNormal)
		p.Set |= styling.FieldStyle
	}
	if s.Weight != WeightInherit {
		p.Weight = font.Weight(s.Weight - WeightNormal)
		p.Set |= styling.FieldWeight
	}
	if s.Family != FamilyInherit {
		p.Family = styling.FontFamily(s.Family - Proportional)
		p.Set |= styling.FieldFamily
	}
	return p
}

// over returns base with s's non-zero fields applied.
func (s TextStyle) over(base styling.TextStyle) styling.TextStyle {
	p := s.partial()
	if p.Set&styling.FieldSize != 0 {
		base.Size = p.Size
	}
	if p.Set&styling.FieldStyle != 0 {
		base.Style = p.Style
	}
	if p.Set&styling.FieldWeight != 0 {
		base.Weight = p.Weight
	}
	if p.Set&styling.FieldFamily != 0 {
		base.Family = p.Family
	}
	return base
}

func fromPartial(p styling.PartialTextStyle) TextStyle {
	var s TextStyle
	if p.Set&styling.FieldSize != 0 {
		s.Size = p.Size
	}
	if p.Set&styling.FieldStyle != 0 {
		s.Style = Style(p.Style) + StyleNormal
	}
	if p.Set&styling.FieldWeight != 0 {
		s.Weight = Weight(p.Weight) + WeightNormal
	}
	if p.Set&styling.FieldFamily != 0 {
		s.Family = Family(p.Family) + Proportional
	}
	return s
}
