package styling

import (
	"image/color"

	"golang.org/x/image/font"
)

// ScrollbarColors is the three colors a scrollbar thumb picks between -
// idle, hovered, and actively dragged.
type ScrollbarColors struct {
	Idle, Hover, Pressed color.Color
}

// SyntaxColors is the palette a Highlighter's classified tokens draw from
// (see TagCodeKeyword..TagCodeFunction).
type SyntaxColors struct {
	Keyword color.Color
	// Type is shared by a builtin primitive type and a declared custom
	// type/class name.
	Type     color.Color
	Function color.Color
	String   color.Color
	Number   color.Color
	Comment  color.Color
}

// Basic is the field-configured Styles implementation behind whynot's
// default look (Dark, Light) and styles/simpletheme. It also satisfies
// whynot.StyleSheet (see Styles), so it can be handed to a View directly;
// treat it as immutable once it has been.
type Basic struct {
	ParagraphMargins   Margins
	ParagraphTextStyle PartialTextStyle

	HeadingMargins    [6]Margins
	HeadingTextStyles [6]PartialTextStyle

	ListMargins Margins

	ListItemMargins   Margins
	ListItemTextStyle PartialTextStyle

	CodeBlockMargins   Margins
	CodeBlockTextStyle PartialTextStyle
	// CodeBlockColor is deliberately a neutral gray rather than an accent
	// color: a whole block of code in a bright color reads as garish and
	// fights with syntax-highlighted spans inside it (see SyntaxColors) -
	// unlike CodeSpanColor, a single inline `code` word stays fine as an
	// accent since it's a small, isolated highlight within prose.
	CodeBlockColor color.Color

	// UnsupportedColor is the text color for a Markdown construct
	// whynot doesn't understand - rendered as a code block (see
	// CodeBlockMargins/CodeBlockTextStyle, shared with TagCodeBlock) but
	// in this distinct color so it reads as an error, not as normal code.
	UnsupportedColor color.Color

	// CodeSpanTextStyle, EmphasisTextStyle, StrongTextStyle: inline spans
	// have no margins of their own to bundle alongside, unlike the
	// block-level tags above.
	CodeSpanTextStyle PartialTextStyle
	// CodeSpanColor is an accent color, unlike CodeBlockColor - see its
	// own doc comment.
	CodeSpanColor     color.Color
	EmphasisTextStyle PartialTextStyle
	StrongTextStyle   PartialTextStyle

	ThematicBreakMargins Margins
	ThematicBreakColor   color.Color

	LinkColor color.Color

	BlockquoteMargins  Margins
	BlockquoteBarColor color.Color

	TableCellTextStyle PartialTextStyle
	TableMargins       Margins
	TableFrameColor    color.Color

	// ImagePlaceholderColor is the rect InlineImage.GetInlineLayout draws
	// in place of an image that's still pending but already knows its
	// final size.
	ImagePlaceholderColor color.Color

	// TextColor is the root-level fallback color for tags with no color
	// of their own. BaseTextStyle is TextStyle's equivalent - unlike the
	// per-tag PartialTextStyle fields above, it's a plain TextStyle,
	// not a PartialTextStyle: being the ultimate fallback means it
	// always claims every field by definition (see TextStyle below), so
	// there's no meaningful subset for a Set mask to express.
	TextColor     color.Color
	BaseTextStyle TextStyle

	// Background is the whole view's fill color. Background, ViewMargin,
	// Highlight and Scrollbar are named to avoid colliding with the
	// methods returning them.
	Background color.Color

	ViewMargin Margins

	Highlight color.Color

	Scrollbar ScrollbarColors

	// Syntax is the palette a Highlighter's classified code tokens draw
	// from - see SyntaxColors.
	Syntax SyntaxColors

	// Dims holds the dimensions the per-node methods of the same names
	// return.
	Dims Dimensions
}

// Dimensions is Basic's dimensional values, in logical (unscaled) pixels
// except LineHeight, a ratio.
type Dimensions struct {
	Strike         float64
	ThematicBreak  float64
	Blockquote     BlockquoteGeometry
	Table          TableGeometry
	LineHeight     float64
	DiagramPadding float64
}

var _ Styles = (*Basic)(nil)

// Styles returns b itself: it's the one method of whynot.StyleSheet, so
// a *Basic can be handed to a View directly.
func (b *Basic) Styles() Styles {
	return b
}

// Dark returns whynot's built-in dark look - light text on a dark
// background - which is what a View uses when given no StyleSheet.
func Dark() *Basic {
	return &Basic{
		ParagraphMargins:   Margins{Top: 10, Bottom: 10},
		ParagraphTextStyle: PartialTextStyle{TextStyle{Size: 16}, FieldSize},

		HeadingMargins: [6]Margins{
			{Top: 30, Bottom: 10},
			{Top: 26, Bottom: 10},
			{Top: 22, Bottom: 10},
			{Top: 18, Bottom: 10},
			{Top: 14, Bottom: 10},
			{Top: 10, Bottom: 10},
		},
		// Family is left at its zero value (Proportional) for every
		// level: no bold small-caps font ships in
		// golang.org/x/image/font/gofont, and headings are bold, so
		// small caps isn't available as a default.
		HeadingTextStyles: [6]PartialTextStyle{
			{TextStyle{Size: 40, Weight: font.WeightBold}, FieldSize | FieldWeight | FieldFamily},
			{TextStyle{Size: 36, Weight: font.WeightBold}, FieldSize | FieldWeight | FieldFamily},
			{TextStyle{Size: 32, Weight: font.WeightBold}, FieldSize | FieldWeight | FieldFamily},
			{TextStyle{Size: 28, Weight: font.WeightBold}, FieldSize | FieldWeight | FieldFamily},
			{TextStyle{Size: 24, Weight: font.WeightBold}, FieldSize | FieldWeight | FieldFamily},
			{TextStyle{Size: 20, Weight: font.WeightBold}, FieldSize | FieldWeight | FieldFamily},
		},

		ListMargins: Margins{Top: 10, Bottom: 10},

		ListItemMargins:   Margins{Top: 5, Bottom: 5, Left: 40},
		ListItemTextStyle: PartialTextStyle{TextStyle{Size: 16}, FieldSize},

		CodeBlockMargins:   Margins{Top: 20, Bottom: 20, Left: 20},
		CodeBlockTextStyle: PartialTextStyle{TextStyle{Size: 16, Family: Monospace}, FieldSize | FieldFamily},
		CodeBlockColor:     color.RGBA{0xD4, 0xD4, 0xD4, 0xFF},

		// Like ThematicBreakColor/BlockquoteBarColor/TableFrameColor
		// below, a strong red reads as an error against either a light
		// or dark background, so Light leaves it as-is.
		UnsupportedColor: color.RGBA{0xFF, 0x33, 0x33, 0xFF},

		CodeSpanTextStyle: PartialTextStyle{TextStyle{Family: Monospace}, FieldFamily},
		CodeSpanColor:     color.RGBA{0xFF, 0xFF, 0x80, 0xFF},
		EmphasisTextStyle: PartialTextStyle{TextStyle{Style: font.StyleItalic}, FieldStyle},
		StrongTextStyle:   PartialTextStyle{TextStyle{Weight: font.WeightBold}, FieldWeight},

		ThematicBreakMargins: Margins{Top: 20, Bottom: 20},
		ThematicBreakColor:   color.RGBA{0x80, 0x80, 0x80, 0xFF},

		LinkColor: color.RGBA{0x66, 0xB2, 0xFF, 0xFF},

		// Like ThematicBreakColor/BlockquoteBarColor/TableFrameColor
		// below, an orange reads fine against either a light or dark
		// background, so Light leaves it as-is.
		Highlight: color.RGBA{0xFF, 0xA5, 0x00, 0xFF},

		// Light overrides this fully (see below) - unlike
		// Highlight above, these don't read well against both
		// backgrounds.
		Scrollbar: ScrollbarColors{
			Idle:    color.RGBA{0x80, 0x80, 0x80, 0xA0},
			Hover:   color.RGBA{0xA0, 0xA0, 0xA0, 0xC0},
			Pressed: color.RGBA{0xC0, 0xC0, 0xC0, 0xE0},
		},

		// Light overrides this fully (see below) - a palette
		// tuned for a dark background won't read well on light and vice
		// versa, the same reason TextColor/LinkColor/CodeBlockColor/
		// CodeSpanColor differ between the two themes.
		Syntax: SyntaxColors{
			Keyword:  color.RGBA{0xC5, 0x86, 0xF2, 0xFF}, // soft violet
			Type:     color.RGBA{0x4E, 0xC9, 0xB0, 0xFF}, // soft teal
			Function: color.RGBA{0xDC, 0xDC, 0xAA, 0xFF}, // soft yellow-tan
			String:   color.RGBA{0x9E, 0xD9, 0x7A, 0xFF}, // soft green
			Number:   color.RGBA{0xF2, 0xB0, 0x66, 0xFF}, // soft orange
			Comment:  color.RGBA{0x80, 0x80, 0x80, 0xFF}, // matches the existing mid-grey decoration color
		},

		BlockquoteMargins:  Margins{Top: 10, Bottom: 10},
		BlockquoteBarColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},

		TableCellTextStyle: PartialTextStyle{TextStyle{Size: 16}, FieldSize},
		TableMargins:       Margins{Top: 10, Bottom: 10},
		TableFrameColor:    color.RGBA{0x80, 0x80, 0x80, 0xFF},

		ImagePlaceholderColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},

		TextColor:     color.White,
		BaseTextStyle: TextStyle{Size: 16},

		Background: color.Black,
		ViewMargin: Margins{Top: 20, Bottom: 20, Left: 20, Right: 20},

		Dims: Dimensions{
			Strike:         1,
			ThematicBreak:  2,
			LineHeight:     1.2,
			DiagramPadding: 12,
			Blockquote:     BlockquoteGeometry{Indent: 16, BarWidth: 3},
			Table: TableGeometry{
				FrameThickness:      2,
				ColumnGap:           12,
				RowGap:              6,
				HeaderGap:           4,
				ColumnRuleThickness: 1,
			},
		},
	}
}

// Light returns whynot's built-in light look - dark text on a light
// background. Everything but color is identical to Dark (margins, sizes, weights, dimensional constants), so
// it's built from it rather than repeating them: ThematicBreakColor,
// BlockquoteBarColor, TableFrameColor, and ImagePlaceholderColor are
// also left as Dark's mid-grey, which reads fine against
// either a light or dark background, unlike
// TextColor/Background/LinkColor/CodeBlockColor/CodeSpanColor/Scrollbar,
// which need real light-appropriate values.
func Light() *Basic {
	s := Dark()
	s.TextColor = color.RGBA{0x1A, 0x1A, 0x1A, 0xFF}
	s.Background = color.White
	s.LinkColor = color.RGBA{0x03, 0x66, 0xD6, 0xFF}
	s.CodeBlockColor = color.RGBA{0x33, 0x33, 0x33, 0xFF}
	s.CodeSpanColor = color.RGBA{0x8B, 0x5A, 0x00, 0xFF}
	s.Scrollbar = ScrollbarColors{
		Idle:    color.RGBA{0x60, 0x60, 0x60, 0xA0},
		Hover:   color.RGBA{0x40, 0x40, 0x40, 0xC0},
		Pressed: color.RGBA{0x20, 0x20, 0x20, 0xE0},
	}
	s.Syntax = SyntaxColors{
		Keyword:  color.RGBA{0x7A, 0x33, 0xB0, 0xFF},
		Type:     color.RGBA{0x00, 0x7A, 0x6E, 0xFF},
		Function: color.RGBA{0x7A, 0x66, 0x00, 0xFF},
		String:   color.RGBA{0x1E, 0x7A, 0x2E, 0xFF},
		Number:   color.RGBA{0xB0, 0x5A, 0x00, 0xFF},
		Comment:  color.RGBA{0x60, 0x60, 0x60, 0xFF},
	}
	return s
}

func (s *Basic) Margins(node *Node) Margins {
	if node == nil {
		return Margins{}
	}
	switch node.Tag {
	case TagParagraph:
		return s.ParagraphMargins
	case TagHeading1, TagHeading2, TagHeading3, TagHeading4, TagHeading5, TagHeading6:
		return s.HeadingMargins[node.Tag-TagHeading1]
	case TagList:
		return s.ListMargins
	case TagListItem:
		return s.ListItemMargins
	case TagCodeBlock, TagUnsupported:
		return s.CodeBlockMargins
	case TagThematicBreak:
		return s.ThematicBreakMargins
	case TagBlockquote:
		return s.BlockquoteMargins
	case TagTable:
		return s.TableMargins
	default:
		return Margins{}
	}
}

// TextStyle returns each tag's own contribution - only the fields Set
// flags are meaningful; ResolvedTextStyle merges these across a node's
// ancestry, so e.g. Strong nested inside Emphasis picks up both. At the
// root (node == nil) it returns BaseTextStyle claiming every field, the
// fallback ResolvedTextStyle reaches if no ancestor ever set some field -
// guarantees e.g. Size is never silently left at 0.
func (s *Basic) TextStyle(node *Node) PartialTextStyle {
	if node == nil {
		return PartialTextStyle{s.BaseTextStyle, AllTextStyleFields}
	}
	switch node.Tag {
	case TagParagraph:
		return s.ParagraphTextStyle
	case TagHeading1, TagHeading2, TagHeading3, TagHeading4, TagHeading5, TagHeading6:
		return s.HeadingTextStyles[node.Tag-TagHeading1]
	case TagListItem:
		return s.ListItemTextStyle
	case TagCodeBlock, TagUnsupported:
		return s.CodeBlockTextStyle
	case TagTableCell:
		return s.TableCellTextStyle
	case TagCodeSpan:
		return s.CodeSpanTextStyle
	case TagEmphasis:
		return s.EmphasisTextStyle
	case TagStrong:
		return s.StrongTextStyle
	default:
		return PartialTextStyle{}
	}
}

// Color returns node's own contribution to the cascading text color - nil
// for any tag with no opinion, so ResolvedColor's ancestry walk passes
// through it to an outer contributor (or the TextColor default at the
// root). Only Link, the two code tags, and Unsupported have an opinion;
// block-level decoration colors live on BorderColor instead, since e.g.
// a blockquote's bar color must never leak into its inner text color.
func (s *Basic) Color(node *Node) color.Color {
	if node == nil {
		return s.TextColor
	}
	switch node.Tag {
	case TagCodeBlock:
		return s.CodeBlockColor
	case TagCodeSpan:
		return s.CodeSpanColor
	case TagCodeKeyword:
		return s.Syntax.Keyword
	case TagCodeType:
		return s.Syntax.Type
	case TagCodeFunction:
		return s.Syntax.Function
	case TagCodeString:
		return s.Syntax.String
	case TagCodeNumber:
		return s.Syntax.Number
	case TagCodeComment:
		return s.Syntax.Comment
	case TagUnsupported:
		return s.UnsupportedColor
	case TagLink:
		return s.LinkColor
	default:
		return nil
	}
}

func (s *Basic) BorderColor(node *Node) color.Color {
	if node == nil {
		return nil
	}
	switch node.Tag {
	case TagThematicBreak:
		return s.ThematicBreakColor
	case TagBlockquote:
		return s.BlockquoteBarColor
	case TagTable:
		return s.TableFrameColor
	case TagImage:
		return s.ImagePlaceholderColor
	default:
		return nil
	}
}

func (s *Basic) BackgroundColor() color.Color {
	return s.Background
}

func (s *Basic) ViewMargins() Margins {
	return s.ViewMargin
}

func (s *Basic) HighlightColor() color.Color {
	return s.Highlight
}

func (s *Basic) ScrollbarColor(hover, pressed bool) color.Color {
	switch {
	case pressed:
		return s.Scrollbar.Pressed
	case hover:
		return s.Scrollbar.Hover
	default:
		return s.Scrollbar.Idle
	}
}

// StrikeThickness returns 0 unless node (or an ancestor) is tagged
// TagStrikethrough - encoding "should this be struck" and "how thick" in
// one value, the same way TextStyle's Weight being font.WeightNormal
// means "not bold".
func (s *Basic) StrikeThickness(node *Node) float64 {
	if !node.HasAncestorTag(TagStrikethrough) {
		return 0
	}
	return s.Dims.Strike
}

func (s *Basic) ThematicBreakThickness(node *Node) float64 {
	return s.Dims.ThematicBreak
}

func (s *Basic) DiagramPadding(node *Node) float64 {
	return s.Dims.DiagramPadding
}

func (s *Basic) BlockquoteGeometry(node *Node) BlockquoteGeometry {
	return s.Dims.Blockquote
}

func (s *Basic) TableGeometry(node *Node) TableGeometry {
	return s.Dims.Table
}

func (s *Basic) LineHeight(node *Node) float64 {
	return s.Dims.LineHeight
}
