// Package simpletheme makes a whynot.StyleSheet from a fixed set of
// fields: margins, text styles and colors for each kind of Markdown
// element.
//
// Use a ready-made stylesheet:
//
//	view := whynot.NewView(doc, faces, simpletheme.DarkStyleSheet)
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
	"github.com/arnodel/whynot/fonts"
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
	// font's natural height: 1.2 adds 20%. Zero means 1.2.
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

// Dark returns the dark look, light text on a dark background, to start
// from.
func Dark() *Theme {
	gray := color.RGBA{0x80, 0x80, 0x80, 0xFF}
	return &Theme{
		ParagraphMargins:   whynot.Margins{Top: 10, Bottom: 10},
		ParagraphTextStyle: TextStyle{Size: 16},

		HeadingMargins: [6]whynot.Margins{
			{Top: 30, Bottom: 10},
			{Top: 26, Bottom: 10},
			{Top: 22, Bottom: 10},
			{Top: 18, Bottom: 10},
			{Top: 14, Bottom: 10},
			{Top: 10, Bottom: 10},
		},
		// Proportional even inside small caps: the bundled Go fonts have
		// no bold small caps.
		HeadingTextStyles: [6]TextStyle{
			{Size: 40, Weight: WeightBold, Family: Proportional},
			{Size: 36, Weight: WeightBold, Family: Proportional},
			{Size: 32, Weight: WeightBold, Family: Proportional},
			{Size: 28, Weight: WeightBold, Family: Proportional},
			{Size: 24, Weight: WeightBold, Family: Proportional},
			{Size: 20, Weight: WeightBold, Family: Proportional},
		},

		ListMargins:       whynot.Margins{Top: 10, Bottom: 10},
		ListItemMargins:   whynot.Margins{Top: 5, Bottom: 5, Left: 40},
		ListItemTextStyle: TextStyle{Size: 16},

		CodeBlockMargins:   whynot.Margins{Top: 20, Bottom: 20, Left: 20},
		CodeBlockTextStyle: TextStyle{Size: 16, Family: Monospace},
		// A neutral gray: a whole block in an accent color reads as garish
		// and fights with syntax colors inside it.
		CodeBlockColor: color.RGBA{0xD4, 0xD4, 0xD4, 0xFF},

		UnsupportedColor: color.RGBA{0xFF, 0x33, 0x33, 0xFF},

		CodeSpanTextStyle: TextStyle{Family: Monospace},
		// An accent color, unlike CodeBlockColor: a single word in prose
		// reads fine highlighted.
		CodeSpanColor:     color.RGBA{0xFF, 0xFF, 0x80, 0xFF},
		EmphasisTextStyle: TextStyle{Style: StyleItalic},
		StrongTextStyle:   TextStyle{Weight: WeightBold},

		ThematicBreakMargins: whynot.Margins{Top: 20, Bottom: 20},
		ThematicBreakColor:   gray,

		LinkColor: color.RGBA{0x66, 0xB2, 0xFF, 0xFF},

		BlockquoteMargins:  whynot.Margins{Top: 10, Bottom: 10},
		BlockquoteBarColor: gray,

		TableCellTextStyle: TextStyle{Size: 16},
		TableMargins:       whynot.Margins{Top: 10, Bottom: 10},
		TableFrameColor:    gray,

		ImagePlaceholderColor: gray,

		TextColor:     color.White,
		BaseTextStyle: TextStyle{Size: 16},

		BackgroundColor: color.Black,
		ViewMargins:     whynot.Margins{Top: 20, Bottom: 20, Left: 20, Right: 20},
		HighlightColor:  color.RGBA{0xFF, 0xA5, 0x00, 0xFF},

		ScrollbarColors: ScrollbarColors{
			Idle:    color.RGBA{0x80, 0x80, 0x80, 0xA0},
			Hover:   color.RGBA{0xA0, 0xA0, 0xA0, 0xC0},
			Pressed: color.RGBA{0xC0, 0xC0, 0xC0, 0xE0},
		},
		SyntaxColors: SyntaxColors{
			Keyword:  color.RGBA{0xC5, 0x86, 0xF2, 0xFF}, // soft violet
			Type:     color.RGBA{0x4E, 0xC9, 0xB0, 0xFF}, // soft teal
			Function: color.RGBA{0xDC, 0xDC, 0xAA, 0xFF}, // soft yellow-tan
			String:   color.RGBA{0x9E, 0xD9, 0x7A, 0xFF}, // soft green
			Number:   color.RGBA{0xF2, 0xB0, 0x66, 0xFF}, // soft orange
			Comment:  gray,
		},

		LineHeight: 1.2,
	}
}

// Light returns the light look, dark text on a light background, to start
// from. It differs from Dark only in colors; the grays and the red and
// orange accents read well on either background, so they're shared.
func Light() *Theme {
	t := Dark()
	t.TextColor = color.RGBA{0x1A, 0x1A, 0x1A, 0xFF}
	t.BackgroundColor = color.White
	t.LinkColor = color.RGBA{0x03, 0x66, 0xD6, 0xFF}
	t.CodeBlockColor = color.RGBA{0x33, 0x33, 0x33, 0xFF}
	t.CodeSpanColor = color.RGBA{0x8B, 0x5A, 0x00, 0xFF}
	t.ScrollbarColors = ScrollbarColors{
		Idle:    color.RGBA{0x60, 0x60, 0x60, 0xA0},
		Hover:   color.RGBA{0x40, 0x40, 0x40, 0xC0},
		Pressed: color.RGBA{0x20, 0x20, 0x20, 0xE0},
	}
	t.SyntaxColors = SyntaxColors{
		Keyword:  color.RGBA{0x7A, 0x33, 0xB0, 0xFF},
		Type:     color.RGBA{0x00, 0x7A, 0x6E, 0xFF},
		Function: color.RGBA{0x7A, 0x66, 0x00, 0xFF},
		String:   color.RGBA{0x1E, 0x7A, 0x2E, 0xFF},
		Number:   color.RGBA{0xB0, 0x5A, 0x00, 0xFF},
		Comment:  color.RGBA{0x60, 0x60, 0x60, 0xFF},
	}
	return t
}

// Dimensions a Theme doesn't expose. Most are borders and padding in all
// but name (blockquote bar and indent, table frame, diagram padding), which
// a box model would expose properly rather than as one-off fields.
var dims = styling.Dimensions{
	Strike:         1,
	ThematicBreak:  2,
	DiagramPadding: 12,
	Blockquote:     styling.BlockquoteGeometry{Indent: 16, BarWidth: 3},
	Table: styling.TableGeometry{
		FrameThickness:      2,
		ColumnGap:           12,
		RowGap:              6,
		HeaderGap:           4,
		ColumnRuleThickness: 1,
	},
}

// Defaults for what a Theme leaves zero where there's nothing to inherit
// from: BaseTextStyle's fields, and LineHeight.
var (
	defaultBaseTextStyle = fonts.TextStyle{Size: 16, Style: font.StyleNormal, Weight: font.WeightNormal, Family: fonts.Proportional}
	defaultLineHeight    = 1.2
)

// StyleSheet returns a snapshot of t, for a whynot.View: changing t
// afterwards doesn't affect it.
func (t *Theme) StyleSheet() whynot.StyleSheet {
	b := &styling.Basic{Dims: dims}
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
	b.BaseTextStyle = t.BaseTextStyle.over(defaultBaseTextStyle)
	b.Background = t.BackgroundColor
	b.ViewMargin = t.ViewMargins
	b.Highlight = t.HighlightColor
	b.Scrollbar = styling.ScrollbarColors(t.ScrollbarColors)
	b.Syntax = styling.SyntaxColors(t.SyntaxColors)
	b.Dims.LineHeight = t.LineHeight
	if b.Dims.LineHeight == 0 {
		b.Dims.LineHeight = defaultLineHeight
	}
	return b
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
		p.Family = fonts.Family(s.Family - Proportional)
		p.Set |= styling.FieldFamily
	}
	return p
}

// over returns base with s's non-zero fields applied.
func (s TextStyle) over(base fonts.TextStyle) fonts.TextStyle {
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
