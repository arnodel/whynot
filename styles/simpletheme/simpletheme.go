package simpletheme

import (
	"image/color"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/codeblocks"
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
	ParagraphMargins   Margins
	ParagraphTextStyle TextStyle

	// HeadingMargins and HeadingTextStyles are indexed by heading level
	// minus one: index 0 is a level 1 heading (#).
	HeadingMargins    [6]Margins
	HeadingTextStyles [6]TextStyle

	ListMargins       Margins
	ListItemMargins   Margins
	ListItemTextStyle TextStyle

	CodeBlockMargins   Margins
	CodeBlockTextStyle TextStyle
	CodeBlockColor     color.Color

	// UnsupportedColor is the text color of Markdown whynot doesn't
	// support, shown as a code block.
	UnsupportedColor color.Color

	CodeSpanTextStyle TextStyle
	CodeSpanColor     color.Color
	EmphasisTextStyle TextStyle
	StrongTextStyle   TextStyle

	ThematicBreakMargins Margins
	ThematicBreakColor   color.Color

	LinkColor color.Color

	BlockquoteMargins  Margins
	BlockquoteBarColor color.Color

	TableCellTextStyle TextStyle
	TableMargins       Margins
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
	ViewMargins Margins
	// HighlightColor is the text color of a hovered link.
	HighlightColor color.Color

	// Scrollbar is how scrollbars look: the View's own, and those of code
	// blocks and tables that scroll sideways.
	Scrollbar Scrollbar
	// SyntaxColors colors code tokens, when a code-block plugin
	// classifies them (see codeblocks.Tokens).
	SyntaxColors SyntaxColors

	// LineHeight is the space a line of text takes, as a multiple of its
	// font's natural height: 1.2 adds 20%. Zero means 1.2.
	LineHeight float64
}

// Margins is the space around a block, in logical (unscaled) pixels.
type Margins struct {
	Top, Bottom, Left, Right float64
}

// Scrollbar is a scrollbar thumb's color when idle, hovered, and being
// dragged, and its size in logical (unscaled) pixels. A zero size means
// the default: 6 thick, 2 from the edge, at least 24 long.
type Scrollbar struct {
	Idle, Hover, Pressed color.Color

	Thickness, Inset, MinThumbLength float64

	// AlwaysVisible shows scrollbars whenever the content can scroll,
	// instead of only while scrolling or pointing at them.
	AlwaysVisible bool
}

// SyntaxColors is the color of each conventional class of code token
// (codeblocks.ClassKeyword and so on). Tokens of other classes, or of
// these when the color is nil, show in the code block's color.
type SyntaxColors struct {
	Keyword  color.Color
	Type     color.Color
	Function color.Color
	String   color.Color
	Number   color.Color
	Comment  color.Color
}

// style converts s to the engine's form, with defaults for zero sizes.
func (s Scrollbar) style() styling.ScrollbarStyle {
	orDefault := func(v, def float64) float64 {
		if v == 0 {
			return def
		}
		return v
	}
	return styling.ScrollbarStyle{
		Idle:    s.Idle,
		Hover:   s.Hover,
		Pressed: s.Pressed,
		ScrollbarGeometry: styling.ScrollbarGeometry{
			Thickness:      orDefault(s.Thickness, 6),
			Inset:          orDefault(s.Inset, 2),
			MinThumbLength: orDefault(s.MinThumbLength, 24),
			AlwaysVisible:  s.AlwaysVisible,
		},
	}
}

// byClass maps c's colors to the token classes they color. A nil color
// acts like a missing class: the token shows in its code block's color.
func (c SyntaxColors) byClass() map[string]color.Color {
	return map[string]color.Color{
		codeblocks.ClassKeyword:  c.Keyword,
		codeblocks.ClassType:     c.Type,
		codeblocks.ClassFunction: c.Function,
		codeblocks.ClassString:   c.String,
		codeblocks.ClassNumber:   c.Number,
		codeblocks.ClassComment:  c.Comment,
	}
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
		ParagraphMargins:   Margins{Top: 10, Bottom: 10},
		ParagraphTextStyle: TextStyle{Size: 16},

		HeadingMargins: [6]Margins{
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

		ListMargins:       Margins{Top: 10, Bottom: 10},
		ListItemMargins:   Margins{Top: 5, Bottom: 5, Left: 40},
		ListItemTextStyle: TextStyle{Size: 16},

		CodeBlockMargins:   Margins{Top: 20, Bottom: 20, Left: 20},
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

		ThematicBreakMargins: Margins{Top: 20, Bottom: 20},
		ThematicBreakColor:   gray,

		LinkColor: color.RGBA{0x66, 0xB2, 0xFF, 0xFF},

		BlockquoteMargins:  Margins{Top: 10, Bottom: 10},
		BlockquoteBarColor: gray,

		TableCellTextStyle: TextStyle{Size: 16},
		TableMargins:       Margins{Top: 10, Bottom: 10},
		TableFrameColor:    gray,

		ImagePlaceholderColor: gray,

		TextColor:     color.White,
		BaseTextStyle: TextStyle{Size: 16},

		BackgroundColor: color.Black,
		ViewMargins:     Margins{Top: 20, Bottom: 20, Left: 20, Right: 20},
		HighlightColor:  color.RGBA{0xFF, 0xA5, 0x00, 0xFF},

		Scrollbar: Scrollbar{
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
	t.Scrollbar.Idle = color.RGBA{0x60, 0x60, 0x60, 0xA0}
	t.Scrollbar.Hover = color.RGBA{0x40, 0x40, 0x40, 0xC0}
	t.Scrollbar.Pressed = color.RGBA{0x20, 0x20, 0x20, 0xE0}
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
	b.ParagraphMargins = styling.Margins(t.ParagraphMargins)
	b.ParagraphTextStyle = t.ParagraphTextStyle.partial()
	for i, m := range t.HeadingMargins {
		b.HeadingMargins[i] = styling.Margins(m)
	}
	for i, s := range t.HeadingTextStyles {
		b.HeadingTextStyles[i] = s.partial()
	}
	b.ListMargins = styling.Margins(t.ListMargins)
	b.ListItemMargins = styling.Margins(t.ListItemMargins)
	b.ListItemTextStyle = t.ListItemTextStyle.partial()
	b.CodeBlockMargins = styling.Margins(t.CodeBlockMargins)
	b.CodeBlockTextStyle = t.CodeBlockTextStyle.partial()
	b.CodeBlockColor = t.CodeBlockColor
	b.UnsupportedColor = t.UnsupportedColor
	b.CodeSpanTextStyle = t.CodeSpanTextStyle.partial()
	b.CodeSpanColor = t.CodeSpanColor
	b.EmphasisTextStyle = t.EmphasisTextStyle.partial()
	b.StrongTextStyle = t.StrongTextStyle.partial()
	b.ThematicBreakMargins = styling.Margins(t.ThematicBreakMargins)
	b.ThematicBreakColor = t.ThematicBreakColor
	b.LinkColor = t.LinkColor
	b.BlockquoteMargins = styling.Margins(t.BlockquoteMargins)
	b.BlockquoteBarColor = t.BlockquoteBarColor
	b.TableCellTextStyle = t.TableCellTextStyle.partial()
	b.TableMargins = styling.Margins(t.TableMargins)
	b.TableFrameColor = t.TableFrameColor
	b.ImagePlaceholderColor = t.ImagePlaceholderColor
	b.TextColor = t.TextColor
	b.BaseTextStyle = t.BaseTextStyle.over(defaultBaseTextStyle)
	b.Background = t.BackgroundColor
	b.ViewMargin = styling.Margins(t.ViewMargins)
	b.Highlight = t.HighlightColor
	b.Scrollbar = t.Scrollbar.style()
	b.TokenColors = t.SyntaxColors.byClass()
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
