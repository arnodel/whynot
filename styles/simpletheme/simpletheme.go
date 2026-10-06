package simpletheme

import (
	"image/color"

	"golang.org/x/image/font"

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

// Theme describes how a document looks. Each kind of Markdown element
// has a field for its margins, its text style and its color, where it
// has one; a nil color means the text color of the enclosing element.
//
// Each TextStyle only sets its non-zero fields, inheriting the rest from
// the enclosing element, and ultimately from BaseTextStyle: strong text
// inside a heading keeps the heading's size, for instance. Sizes and
// margins are in logical pixels, which a View multiplies by its scale and
// zoom.
type Theme struct {
	// ParagraphMargins and ParagraphTextStyle are a paragraph's.
	ParagraphMargins   Margins
	ParagraphTextStyle TextStyle

	// HeadingMargins and HeadingTextStyles are each heading level's,
	// indexed by level minus one: index 0 is for a level 1 heading (#).
	HeadingMargins    [6]Margins
	HeadingTextStyles [6]TextStyle

	// ListMargins are around a whole list, nested lists included.
	ListMargins Margins
	// ListItemMargins are around each item. Left is the item's indent,
	// in which its bullet or number sits, just before the text.
	ListItemMargins   Margins
	ListItemTextStyle TextStyle

	// CodeBlockMargins, CodeBlockTextStyle and CodeBlockColor are a code
	// block's. The color is for code a plugin hasn't classified, and for
	// tokens of a class SyntaxColors doesn't color.
	CodeBlockMargins   Margins
	CodeBlockTextStyle TextStyle
	CodeBlockColor     color.Color

	// UnsupportedColor is the text color of Markdown whynot can't show,
	// such as HTML, which is shown as its source, and of the text shown
	// in place of an image that isn't loaded. Such text has
	// CodeBlockTextStyle, and as a block, CodeBlockMargins.
	UnsupportedColor color.Color

	// CodeSpanTextStyle and CodeSpanColor are those of `code` within a
	// line of text.
	CodeSpanTextStyle TextStyle
	CodeSpanColor     color.Color
	// EmphasisTextStyle is that of *emphasis*.
	EmphasisTextStyle TextStyle
	// StrongTextStyle is that of **strong** text.
	StrongTextStyle TextStyle

	// ThematicBreakMargins and ThematicBreakColor are those of the line a
	// thematic break (---) draws across the page.
	ThematicBreakMargins Margins
	ThematicBreakColor   color.Color

	// LinkColor is the text color of a link; see also HighlightColor.
	LinkColor color.Color

	// BlockquoteMargins are around a blockquote, and BlockquoteBarColor
	// is the color of the bar down its left side. Its text is indented
	// past the bar.
	BlockquoteMargins  Margins
	BlockquoteBarColor color.Color

	// TableCellTextStyle is the text style of each cell, header cells
	// included. TableMargins are around the whole table, and
	// TableFrameColor is the color of its frame and of the rules between
	// its columns and below its header.
	TableCellTextStyle TextStyle
	TableMargins       Margins
	TableFrameColor    color.Color

	// ImagePlaceholderColor fills the space of an image that's still
	// loading, once its size is known, and frames a diagram a code-block
	// plugin renders.
	ImagePlaceholderColor color.Color

	// TextColor is the color of text with no color of its own.
	TextColor color.Color
	// BaseTextStyle is where every text style inherits from; a field it
	// leaves zero takes a built-in default: 16 pixels, normal style and
	// weight, proportional family.
	BaseTextStyle TextStyle

	// BackgroundColor fills the whole view.
	BackgroundColor color.Color
	// ViewMargins is the space between the view's edge and the document.
	ViewMargins Margins
	// HighlightColor is the text color of the link under the pointer,
	// instead of LinkColor.
	HighlightColor color.Color

	// Scrollbar is how scrollbars look: the View's own, and those of code
	// blocks and tables that scroll sideways.
	Scrollbar Scrollbar
	// SyntaxColors colors code tokens, when a code-block plugin
	// classifies them (see codeblocks.Tokens).
	SyntaxColors SyntaxColors

	// LineHeight is the space a line of text takes, as a multiple of its
	// font's natural height: 1.2 adds 20%. It applies to all text. Zero
	// means 1.2.
	LineHeight float64
}

// Margins is the space around a block, in logical pixels. Left and Right
// indent the block. Top and Bottom collapse with the neighboring blocks'
// margins, as in CSS: the space between two blocks is the larger of the
// first one's Bottom and the second one's Top, not their sum. At the top
// and bottom of the document, ViewMargins applies instead.
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
// ([codeblocks.ClassKeyword] and so on). Tokens of other classes, or of
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
	rule := color.RGBA{0x3D, 0x44, 0x4D, 0xFF}
	return &Theme{
		ParagraphMargins:   Margins{Top: 12, Bottom: 12},
		ParagraphTextStyle: TextStyle{Size: 16},

		HeadingMargins: [6]Margins{
			{Top: 32, Bottom: 14},
			{Top: 30, Bottom: 12},
			{Top: 26, Bottom: 10},
			{Top: 22, Bottom: 8},
			{Top: 20, Bottom: 8},
			{Top: 20, Bottom: 8},
		},
		// Proportional even inside small caps: the bundled Go fonts have
		// no bold small caps.
		HeadingTextStyles: [6]TextStyle{
			{Size: 32, Weight: WeightBold, Family: Proportional},
			{Size: 26, Weight: WeightBold, Family: Proportional},
			{Size: 21, Weight: WeightBold, Family: Proportional},
			{Size: 18, Weight: WeightBold, Family: Proportional},
			{Size: 16, Weight: WeightBold, Family: Proportional},
			{Size: 14, Weight: WeightBold, Family: Proportional},
		},

		ListMargins:       Margins{Top: 12, Bottom: 12},
		ListItemMargins:   Margins{Top: 3, Bottom: 3, Left: 28},
		ListItemTextStyle: TextStyle{Size: 16},

		CodeBlockMargins:   Margins{Top: 16, Bottom: 16, Left: 20},
		CodeBlockTextStyle: TextStyle{Size: 14, Family: Monospace},
		CodeBlockColor:     color.RGBA{0xCD, 0xD3, 0xDA, 0xFF},

		UnsupportedColor: color.RGBA{0xF2, 0x7B, 0x7B, 0xFF},

		CodeSpanTextStyle: TextStyle{Size: 14.5, Family: Monospace},
		CodeSpanColor:     color.RGBA{0xE8, 0xB9, 0x6A, 0xFF},
		EmphasisTextStyle: TextStyle{Style: StyleItalic},
		StrongTextStyle:   TextStyle{Weight: WeightBold},

		ThematicBreakMargins: Margins{Top: 24, Bottom: 24},
		ThematicBreakColor:   rule,

		LinkColor: color.RGBA{0x6C, 0xB6, 0xFF, 0xFF},

		BlockquoteMargins:  Margins{Top: 12, Bottom: 12},
		BlockquoteBarColor: color.RGBA{0x55, 0x60, 0x6C, 0xFF},

		TableCellTextStyle: TextStyle{Size: 15},
		TableMargins:       Margins{Top: 12, Bottom: 12},
		TableFrameColor:    rule,

		ImagePlaceholderColor: color.RGBA{0x2A, 0x30, 0x37, 0xFF},

		TextColor:     color.RGBA{0xE1, 0xE4, 0xE8, 0xFF},
		BaseTextStyle: TextStyle{Size: 16},

		BackgroundColor: color.RGBA{0x17, 0x1A, 0x1F, 0xFF},
		ViewMargins:     Margins{Top: 24, Bottom: 24, Left: 28, Right: 28},
		HighlightColor:  color.RGBA{0xA8, 0xD4, 0xFF, 0xFF},

		Scrollbar: Scrollbar{
			Idle:    color.RGBA{0x80, 0x88, 0x92, 0x80},
			Hover:   color.RGBA{0xA0, 0xA8, 0xB2, 0xB0},
			Pressed: color.RGBA{0xC0, 0xC8, 0xD2, 0xE0},
		},
		SyntaxColors: SyntaxColors{
			Keyword:  color.RGBA{0xC6, 0x9C, 0xF0, 0xFF},
			Type:     color.RGBA{0x5F, 0xC8, 0xB4, 0xFF},
			Function: color.RGBA{0x7C, 0xB7, 0xF2, 0xFF},
			String:   color.RGBA{0xA5, 0xD6, 0x87, 0xFF},
			Number:   color.RGBA{0xE8, 0xA8, 0x6B, 0xFF},
			Comment:  color.RGBA{0x7D, 0x87, 0x93, 0xFF},
		},

		LineHeight: 1.3,
	}
}

// Light returns the light look, dark text on a light background, to start
// from. It differs from Dark only in colors.
func Light() *Theme {
	t := Dark()
	rule := color.RGBA{0xD5, 0xDA, 0xDF, 0xFF}
	t.TextColor = color.RGBA{0x1F, 0x24, 0x2A, 0xFF}
	t.BackgroundColor = color.RGBA{0xFC, 0xFC, 0xFD, 0xFF}
	t.LinkColor = color.RGBA{0x0B, 0x63, 0xC9, 0xFF}
	t.HighlightColor = color.RGBA{0x06, 0x3F, 0x85, 0xFF}
	t.CodeBlockColor = color.RGBA{0x2B, 0x31, 0x38, 0xFF}
	t.CodeSpanColor = color.RGBA{0xA3, 0x4A, 0x12, 0xFF}
	t.UnsupportedColor = color.RGBA{0xC2, 0x2E, 0x2E, 0xFF}
	t.ThematicBreakColor = rule
	t.TableFrameColor = rule
	t.BlockquoteBarColor = color.RGBA{0xB4, 0xBC, 0xC5, 0xFF}
	t.ImagePlaceholderColor = color.RGBA{0xEC, 0xEF, 0xF2, 0xFF}
	t.Scrollbar.Idle = color.RGBA{0x60, 0x68, 0x72, 0x70}
	t.Scrollbar.Hover = color.RGBA{0x48, 0x50, 0x5A, 0xA0}
	t.Scrollbar.Pressed = color.RGBA{0x30, 0x38, 0x42, 0xD0}
	t.SyntaxColors = SyntaxColors{
		Keyword:  color.RGBA{0x83, 0x2B, 0xB5, 0xFF},
		Type:     color.RGBA{0x0B, 0x7A, 0x69, 0xFF},
		Function: color.RGBA{0x14, 0x5E, 0xA8, 0xFF},
		String:   color.RGBA{0x23, 0x7A, 0x2F, 0xFF},
		Number:   color.RGBA{0xB0, 0x52, 0x0B, 0xFF},
		Comment:  color.RGBA{0x6E, 0x77, 0x81, 0xFF},
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

// StyleSheet is a stylesheet made from a [Theme], for a whynot View (see
// [github.com/arnodel/whynot.WithStyleSheet]). Make one with
// [Theme.StyleSheet]: the zero StyleSheet can't be used.
type StyleSheet struct {
	styles *styling.Basic
}

// Styles is for whynot's own use.
func (s StyleSheet) Styles() styling.Styles {
	return s.styles
}

// StyleSheet returns a snapshot of t: changing t afterwards doesn't
// affect it.
func (t *Theme) StyleSheet() StyleSheet {
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
	return StyleSheet{b}
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
