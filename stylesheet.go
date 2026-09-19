package whynot

import (
	"image/color"

	"golang.org/x/image/font"
)

// BlockquoteGeometry is the dimensional constants a blockquote's own bar
// and indent need - grouped since a blockquote's GetBlockLayout always reads both
// together.
type BlockquoteGeometry struct {
	Indent   float64
	BarWidth float64
}

// TableGeometry is the dimensional constants a table's layout needs -
// grouped since TableBlock.GetBlockLayout always reads all five together.
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
	// BackgroundColor returns the color the document's viewport is filled
	// with before anything else draws. Unlike every other method here,
	// it takes no node: it isn't a property of any particular tag, just
	// painted once, for the whole view, before any content-specific
	// resolution happens - a node parameter would have nothing real to
	// mean.
	BackgroundColor() color.Color
	// ViewMargins returns the space between the document's viewport edge
	// and its content, on all four sides - the same shape as a tag's own
	// Margins, but for the whole view rather than one node, the same way
	// BackgroundColor is a whole-view counterpart to Color.
	ViewMargins() Margins
	// HighlightColor returns the text color for whatever's under
	// RenderingContext.HighlightNode (e.g. a hovered link) - takes no
	// node for the same reason BackgroundColor doesn't: it isn't a
	// property of any one tag, it's a transient interaction state
	// RenderingContext.ResolvedColor substitutes in ahead of the normal
	// per-node cascade.
	HighlightColor() color.Color

	// StrikeThickness returns the thickness for a strikethrough line at
	// node, or 0 if node isn't (nor is any ancestor) struck through -
	// this is both "should this be struck" and "how thick", the same way
	// TextStyle's Weight being font.WeightNormal means "not bold".
	StrikeThickness(node *ASTNode) float64
	ThematicBreakThickness(node *ASTNode) float64
	BlockquoteGeometry(node *ASTNode) BlockquoteGeometry
	TableGeometry(node *ASTNode) TableGeometry
}

// ScrollbarStyleSheet is an optional StyleSheet capability: a StyleSheet
// that also knows how to color a scrollbar thumb. It's optional, not
// part of StyleSheet itself, because a scrollbar is drawn by whatever
// embeds a View (cmd/whynot today, a future ebitenrenderer.Panel later)
// - View itself never draws one - so a StyleSheet that's never paired
// with a scrollbar (e.g. a test StyleSheet) isn't forced to implement
// it. A caller that wants a scrollbar type-asserts its StyleSheet
// against this interface and falls back to a reasonable default color
// when it isn't implemented.
type ScrollbarStyleSheet interface {
	// ScrollbarColor returns the color to draw the scrollbar thumb in.
	// hover is true whenever the thumb is highlighted (including while
	// dragging); pressed is true only while actively being dragged.
	ScrollbarColor(hover, pressed bool) color.Color
}

// ScrollbarColors is the three colors DefaultStyleSheet.ScrollbarColor
// picks between - idle, hovered, and actively-dragged - kept as one
// struct (like BlockquoteGeometry/TableGeometry) since overriding one
// almost always means reconsidering all three together, as one "gets
// more prominent as you interact" progression.
type ScrollbarColors struct {
	Idle, Hover, Pressed color.Color
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

	// UnsupportedColor is the text color for a Markdown construct
	// whynot doesn't understand - rendered as a code block (see
	// CodeBlockMargins/CodeBlockTextStyle, shared with TagCodeBlock) but
	// in this distinct color so it reads as an error, not as normal code.
	UnsupportedColor color.Color

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

	// Background is the whole viewport's fill color - see BackgroundColor.
	// Named without the "Color" suffix only to avoid colliding with the
	// BackgroundColor method (color.Color already makes the field's own
	// meaning unambiguous).
	Background color.Color

	// ViewMargin is the whole viewport's own inset - see ViewMargins. Named
	// in the singular only to avoid colliding with the plural ViewMargins
	// method (Margins is a struct - like the field, not a color.Color -
	// so unlike Background/BackgroundColor there's no type-derived word
	// to hang the distinction on instead).
	ViewMargin Margins

	// Highlight is the text color for whatever's under
	// RenderingContext.HighlightNode - see HighlightColor. Named without
	// the "Color" suffix for the same reason Background is.
	Highlight color.Color

	// Scrollbar is the thumb's three interaction-state colors - see
	// ScrollbarColor. Unlike ThematicBreakColor/BlockquoteBarColor/
	// TableFrameColor (whynot's own content decorations), a scrollbar is
	// drawn outside whynot entirely, by whatever embeds a View - this
	// field only supplies the color that embedder asks for. Named
	// without the "Color" suffix for the same reason Background/
	// Highlight are.
	Scrollbar ScrollbarColors

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
var _ ScrollbarStyleSheet = (*DefaultStyleSheet)(nil)

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

		// Like ThematicBreakColor/BlockquoteBarColor/TableFrameColor
		// below, a strong red reads as an error against either a light
		// or dark background, so NewLightStyleSheet leaves it as-is.
		UnsupportedColor: color.RGBA{0xFF, 0x33, 0x33, 0xFF},

		CodeSpanTextStyle: PartialTextStyle{TextStyle{Family: Monospace}, FieldFamily},
		EmphasisTextStyle: PartialTextStyle{TextStyle{Style: font.StyleItalic}, FieldStyle},
		StrongTextStyle:   PartialTextStyle{TextStyle{Weight: font.WeightBold}, FieldWeight},

		ThematicBreakMargins: Margins{Top: 20, Bottom: 20},
		ThematicBreakColor:   color.RGBA{0x80, 0x80, 0x80, 0xFF},

		LinkColor: color.RGBA{0x66, 0xB2, 0xFF, 0xFF},

		// Like ThematicBreakColor/BlockquoteBarColor/TableFrameColor
		// below, an orange reads fine against either a light or dark
		// background, so NewLightStyleSheet leaves it as-is.
		Highlight: color.RGBA{0xFF, 0xA5, 0x00, 0xFF},

		// NewLightStyleSheet overrides this fully (see below) - unlike
		// Highlight above, these don't read well against both
		// backgrounds.
		Scrollbar: ScrollbarColors{
			Idle:    color.RGBA{0x80, 0x80, 0x80, 0xA0},
			Hover:   color.RGBA{0xA0, 0xA0, 0xA0, 0xC0},
			Pressed: color.RGBA{0xC0, 0xC0, 0xC0, 0xE0},
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
// BlockquoteBarColor, TableFrameColor, and ImagePlaceholderColor are
// also left as NewDarkStyleSheet's mid-grey, which reads fine against
// either a light or dark background, unlike
// TextColor/Background/LinkColor/CodeColor/Scrollbar, which need real
// light-appropriate values.
func NewLightStyleSheet() *DefaultStyleSheet {
	s := NewDarkStyleSheet()
	s.TextColor = color.RGBA{0x1A, 0x1A, 0x1A, 0xFF}
	s.Background = color.White
	s.LinkColor = color.RGBA{0x03, 0x66, 0xD6, 0xFF}
	s.CodeColor = color.RGBA{0x8B, 0x5A, 0x00, 0xFF}
	s.Scrollbar = ScrollbarColors{
		Idle:    color.RGBA{0x60, 0x60, 0x60, 0xA0},
		Hover:   color.RGBA{0x40, 0x40, 0x40, 0xC0},
		Pressed: color.RGBA{0x20, 0x20, 0x20, 0xE0},
	}
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
func (s *DefaultStyleSheet) Color(node *ASTNode) color.Color {
	if node == nil {
		return s.TextColor
	}
	switch node.Tag {
	case TagCodeBlock, TagCodeSpan:
		return s.CodeColor
	case TagUnsupported:
		return s.UnsupportedColor
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
	case TagImage:
		return s.ImagePlaceholderColor
	default:
		return nil
	}
}

func (s *DefaultStyleSheet) BackgroundColor() color.Color {
	return s.Background
}

func (s *DefaultStyleSheet) ViewMargins() Margins {
	return s.ViewMargin
}

func (s *DefaultStyleSheet) HighlightColor() color.Color {
	return s.Highlight
}

func (s *DefaultStyleSheet) ScrollbarColor(hover, pressed bool) color.Color {
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
