package whynot

import (
	"image/color"

	"golang.org/x/image/font"
)

// BlockquoteGeometry is the dimensional constants a blockquote's own bar
// and indent need - grouped since a blockquote's GetBox always reads both
// together.
type BlockquoteGeometry struct {
	Indent   float64
	BarWidth float64
}

// TableGeometry is the dimensional constants a table's layout needs -
// grouped since TableBlock.GetBox always reads all five together.
type TableGeometry struct {
	FrameThickness      float64
	ColumnGap           float64
	RowGap              float64
	HeaderGap           float64
	ColumnRuleThickness float64
}

// TextStyleField identifies one field of TextStyle, so a
// PartialTextStyle can say which fields it actually sets - needed
// because TextStyle's own zero values (font.WeightNormal, font.StyleNormal,
// Proportional) are real, meaningful values, not "unset" sentinels.
type TextStyleField uint8

const (
	FieldSize TextStyleField = 1 << iota
	FieldStyle
	FieldWeight
	FieldFamily

	allTextStyleFields = FieldSize | FieldStyle | FieldWeight | FieldFamily
)

// PartialTextStyle is what a single node's own tag contributes to
// TextStyle - only the fields flagged in Set are meaningful; every other
// field is inherited from further up the node's ancestry (see
// RenderingContext.ResolvedTextStyle).
type PartialTextStyle struct {
	TextStyle
	Set TextStyleField
}

// StyleSheet resolves an ASTNode's semantic role to concrete appearance -
// margins, text styling, color, and a few dimensional constants. Every
// method takes the node, uniformly, even where nothing in whynot itself
// varies a value by node today (there's only one table's worth of column
// gap, for instance) - an implementation is free to ignore it, but the
// shape leaves room for one that doesn't want to. See DefaultStyleSheet
// for whynot's built-in appearance.
type StyleSheet interface {
	// Margins returns the margins for node's own tag.
	Margins(node *ASTNode) Margins
	// TextStyle returns node's own tag's contribution to TextStyle -
	// combined with its ancestors' contributions by
	// RenderingContext.ResolvedTextStyle, since e.g. Strong nested inside
	// Emphasis needs both a bold and an italic contribution to survive.
	TextStyle(node *ASTNode) PartialTextStyle
	// Color returns node's own tag's contribution to the cascading text
	// color, or nil if it has no opinion (inherits from an ancestor, or
	// the document default at the root - see
	// RenderingContext.ResolvedColor). Mirrors CSS's `color`, which
	// inherits by default.
	Color(node *ASTNode) color.Color
	// BorderColor returns node's own single, non-inherited decoration
	// color - a thematic break's rule, a blockquote's bar, a table's
	// frame. Unlike Color, this is always resolved directly against one
	// node, never by walking ancestry - mirrors CSS's border-color,
	// which doesn't inherit.
	BorderColor(node *ASTNode) color.Color
	// Background returns the color the document's viewport is filled
	// with before anything else draws. Unlike every other method here,
	// it takes no node: it isn't a property of any particular tag, just
	// painted once, for the whole view, before any content-specific
	// resolution happens - a node parameter would have nothing real to
	// mean.
	Background() color.Color

	// StrikeThickness returns the thickness for a strikethrough line at
	// node, or 0 if node isn't (nor is any ancestor) struck through -
	// this is both "should this be struck" and "how thick", the same way
	// TextStyle's Weight being font.WeightNormal means "not bold".
	StrikeThickness(node *ASTNode) float64
	ThematicBreakThickness(node *ASTNode) float64
	BlockquoteGeometry(node *ASTNode) BlockquoteGeometry
	TableGeometry(node *ASTNode) TableGeometry
}

// DefaultStyleSheet is the concrete, configurable StyleSheet implementation
// whynot's built-in themes (NewDarkStyleSheet, NewLightStyleSheet) are both
// built from - the two differ only in the field values their constructors
// fill in, not in shape. A caller who wants to change one thing can start
// from either constructor and mutate a field (ParagraphMargins, LinkColor,
// ParagraphTextStyle.Size, ...), or embed it in a custom StyleSheet and
// override individual methods for full control. The PartialTextStyle
// fields (as opposed to a plain TextStyle) mean a tag's contribution can
// be extended, not just adjusted - e.g. style.EmphasisTextStyle.Weight =
// font.WeightBold; style.EmphasisTextStyle.Set |= FieldWeight makes
// emphasis bold as well as italic, which a plain TextStyle field couldn't
// express: the fields it doesn't set wouldn't be distinguishable from
// fields deliberately set to their zero value.
type DefaultStyleSheet struct {
	ParagraphMargins   Margins
	ParagraphTextStyle PartialTextStyle

	HeadingMargins    [6]Margins
	HeadingTextStyles [6]PartialTextStyle

	ListMargins Margins

	ListItemMargins   Margins
	ListItemTextStyle PartialTextStyle

	CodeBlockMargins   Margins
	CodeBlockTextStyle PartialTextStyle
	CodeColor          color.Color

	// CodeSpanTextStyle, EmphasisTextStyle, StrongTextStyle: inline spans
	// have no margins of their own to bundle alongside, unlike the
	// block-level tags above.
	CodeSpanTextStyle PartialTextStyle
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

	// TextColor is the root-level fallback color for tags with no color
	// of their own. BaseTextStyle is TextStyle's equivalent - unlike the
	// per-tag PartialTextStyle fields above, it's a plain TextStyle,
	// not a PartialTextStyle: being the ultimate fallback means it
	// always claims every field by definition (see TextStyle below), so
	// there's no meaningful subset for a Set mask to express.
	TextColor     color.Color
	BaseTextStyle TextStyle

	// BackgroundColor is the whole viewport's fill color - see Background.
	BackgroundColor color.Color

	// Dimensional constants. Unexported: unlike the fields above, these
	// aren't the primary customization surface (a game reaches for
	// colors/margins/fonts, rarely a table's own column-gap width) - a
	// caller who does want to change one overrides the method instead,
	// via a custom StyleSheet embedding this one.
	strikeThickness        float64
	thematicBreakThickness float64
	blockquoteGeometry     BlockquoteGeometry
	tableGeometry          TableGeometry
}

var _ StyleSheet = (*DefaultStyleSheet)(nil)

// NewDarkStyleSheet returns whynot's built-in dark theme - light text on a
// dark background - and is what NewView uses when no StyleSheet is given.
func NewDarkStyleSheet() *DefaultStyleSheet {
	return &DefaultStyleSheet{
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
		CodeColor:          color.RGBA{0xFF, 0xFF, 0x80, 0xFF},

		CodeSpanTextStyle: PartialTextStyle{TextStyle{Family: Monospace}, FieldFamily},
		EmphasisTextStyle: PartialTextStyle{TextStyle{Style: font.StyleItalic}, FieldStyle},
		StrongTextStyle:   PartialTextStyle{TextStyle{Weight: font.WeightBold}, FieldWeight},

		ThematicBreakMargins: Margins{Top: 20, Bottom: 20},
		ThematicBreakColor:   color.RGBA{0x80, 0x80, 0x80, 0xFF},

		LinkColor: color.RGBA{0x66, 0xB2, 0xFF, 0xFF},

		BlockquoteMargins:  Margins{Top: 10, Bottom: 10},
		BlockquoteBarColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},

		TableCellTextStyle: PartialTextStyle{TextStyle{Size: 16}, FieldSize},
		TableMargins:       Margins{Top: 10, Bottom: 10},
		TableFrameColor:    color.RGBA{0x80, 0x80, 0x80, 0xFF},

		TextColor:     color.White,
		BaseTextStyle: TextStyle{Size: 16},

		BackgroundColor: color.Black,

		strikeThickness:        1,
		thematicBreakThickness: 2,
		blockquoteGeometry:     BlockquoteGeometry{Indent: 16, BarWidth: 3},
		tableGeometry: TableGeometry{
			FrameThickness:      2,
			ColumnGap:           12,
			RowGap:              6,
			HeaderGap:           4,
			ColumnRuleThickness: 1,
		},
	}
}

// NewLightStyleSheet returns whynot's built-in light theme - dark text on
// a light background. Everything but color is identical to
// NewDarkStyleSheet (margins, sizes, weights, dimensional constants), so
// it's built from it rather than repeating them: ThematicBreakColor,
// BlockquoteBarColor, and TableFrameColor are also left as
// NewDarkStyleSheet's mid-grey, which reads fine against either a light
// or dark background, unlike TextColor/BackgroundColor/LinkColor/
// CodeColor, which need real light-appropriate values.
func NewLightStyleSheet() *DefaultStyleSheet {
	s := NewDarkStyleSheet()
	s.TextColor = color.RGBA{0x1A, 0x1A, 0x1A, 0xFF}
	s.BackgroundColor = color.White
	s.LinkColor = color.RGBA{0x03, 0x66, 0xD6, 0xFF}
	s.CodeColor = color.RGBA{0x8B, 0x5A, 0x00, 0xFF}
	return s
}

func (s *DefaultStyleSheet) Margins(node *ASTNode) Margins {
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
	case TagCodeBlock:
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
func (s *DefaultStyleSheet) TextStyle(node *ASTNode) PartialTextStyle {
	if node == nil {
		return PartialTextStyle{s.BaseTextStyle, allTextStyleFields}
	}
	switch node.Tag {
	case TagParagraph:
		return s.ParagraphTextStyle
	case TagHeading1, TagHeading2, TagHeading3, TagHeading4, TagHeading5, TagHeading6:
		return s.HeadingTextStyles[node.Tag-TagHeading1]
	case TagListItem:
		return s.ListItemTextStyle
	case TagCodeBlock:
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
// root). Only Link and the two code tags have an opinion; block-level
// decoration colors live on BorderColor instead, since e.g. a blockquote's
// bar color must never leak into its inner text color.
func (s *DefaultStyleSheet) Color(node *ASTNode) color.Color {
	if node == nil {
		return s.TextColor
	}
	switch node.Tag {
	case TagCodeBlock, TagCodeSpan:
		return s.CodeColor
	case TagLink:
		return s.LinkColor
	default:
		return nil
	}
}

func (s *DefaultStyleSheet) BorderColor(node *ASTNode) color.Color {
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
	default:
		return nil
	}
}

func (s *DefaultStyleSheet) Background() color.Color {
	return s.BackgroundColor
}

// StrikeThickness returns 0 unless node (or an ancestor) is tagged
// TagStrikethrough - encoding "should this be struck" and "how thick" in
// one value, the same way TextStyle's Weight being font.WeightNormal
// means "not bold".
func (s *DefaultStyleSheet) StrikeThickness(node *ASTNode) float64 {
	if !node.HasAncestorTag(TagStrikethrough) {
		return 0
	}
	return s.strikeThickness
}

func (s *DefaultStyleSheet) ThematicBreakThickness(node *ASTNode) float64 {
	return s.thematicBreakThickness
}

func (s *DefaultStyleSheet) BlockquoteGeometry(node *ASTNode) BlockquoteGeometry {
	return s.blockquoteGeometry
}

func (s *DefaultStyleSheet) TableGeometry(node *ASTNode) TableGeometry {
	return s.tableGeometry
}
