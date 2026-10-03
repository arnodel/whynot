package styling

import (
	"image/color"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
)

// ScrollbarStyle is how scrollbars look: the three colors a thumb picks
// between (idle, hovered, and dragged), and its geometry.
type ScrollbarStyle struct {
	Idle, Hover, Pressed color.Color
	ScrollbarGeometry
}

// Basic is the field-configured Styles implementation behind
// styles/simpletheme. It also satisfies
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
	// fights with syntax-highlighted spans inside it (see TokenColors) -
	// unlike CodeSpanColor, a single inline `code` word stays fine as an
	// accent since it's a small, isolated highlight within prose.
	CodeBlockColor color.Color

	// UnsupportedColor is the text color for a Markdown construct
	// whynot doesn't understand - rendered as a code block (see
	// CodeBlockMargins/CodeBlockTextStyle, shared with ast.TagCodeBlock) but
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
	BaseTextStyle fonts.TextStyle

	// Background is the whole view's fill color. Background, ViewMargin,
	// Highlight and Scrollbar are named to avoid colliding with the
	// methods returning them.
	Background color.Color

	ViewMargin Margins

	Highlight color.Color

	Scrollbar ScrollbarStyle

	// TokenColors colors code tokens (ast.TagCodeToken) by their Class.
	// A token whose class isn't here shows in its code block's color.
	TokenColors map[string]color.Color

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

func (s *Basic) Margins(node *ast.Node) Margins {
	if node == nil {
		return Margins{}
	}
	switch node.Tag {
	case ast.TagParagraph:
		return s.ParagraphMargins
	case ast.TagHeading1, ast.TagHeading2, ast.TagHeading3, ast.TagHeading4, ast.TagHeading5, ast.TagHeading6:
		return s.HeadingMargins[node.Tag-ast.TagHeading1]
	case ast.TagList:
		return s.ListMargins
	case ast.TagListItem:
		return s.ListItemMargins
	case ast.TagCodeBlock, ast.TagUnsupported:
		return s.CodeBlockMargins
	case ast.TagThematicBreak:
		return s.ThematicBreakMargins
	case ast.TagBlockquote:
		return s.BlockquoteMargins
	case ast.TagTable:
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
func (s *Basic) TextStyle(node *ast.Node) PartialTextStyle {
	if node == nil {
		return PartialTextStyle{s.BaseTextStyle, AllTextStyleFields}
	}
	switch node.Tag {
	case ast.TagParagraph:
		return s.ParagraphTextStyle
	case ast.TagHeading1, ast.TagHeading2, ast.TagHeading3, ast.TagHeading4, ast.TagHeading5, ast.TagHeading6:
		return s.HeadingTextStyles[node.Tag-ast.TagHeading1]
	case ast.TagListItem:
		return s.ListItemTextStyle
	case ast.TagCodeBlock, ast.TagUnsupported:
		return s.CodeBlockTextStyle
	case ast.TagTableCell:
		return s.TableCellTextStyle
	case ast.TagCodeSpan:
		return s.CodeSpanTextStyle
	case ast.TagEmphasis:
		return s.EmphasisTextStyle
	case ast.TagStrong:
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
func (s *Basic) Color(node *ast.Node) color.Color {
	if node == nil {
		return s.TextColor
	}
	switch node.Tag {
	case ast.TagCodeBlock:
		return s.CodeBlockColor
	case ast.TagCodeSpan:
		return s.CodeSpanColor
	case ast.TagCodeToken:
		return s.TokenColors[node.Class]
	case ast.TagUnsupported:
		return s.UnsupportedColor
	case ast.TagLink:
		return s.LinkColor
	default:
		return nil
	}
}

func (s *Basic) BorderColor(node *ast.Node) color.Color {
	if node == nil {
		return nil
	}
	switch node.Tag {
	case ast.TagThematicBreak:
		return s.ThematicBreakColor
	case ast.TagBlockquote:
		return s.BlockquoteBarColor
	case ast.TagTable:
		return s.TableFrameColor
	case ast.TagImage:
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

func (s *Basic) ScrollbarGeometry() ScrollbarGeometry {
	return s.Scrollbar.ScrollbarGeometry
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
// ast.TagStrikethrough - encoding "should this be struck" and "how thick" in
// one value, the same way TextStyle's Weight being font.WeightNormal
// means "not bold".
func (s *Basic) StrikeThickness(node *ast.Node) float64 {
	if !node.HasAncestorTag(ast.TagStrikethrough) {
		return 0
	}
	return s.Dims.Strike
}

func (s *Basic) ThematicBreakThickness(node *ast.Node) float64 {
	return s.Dims.ThematicBreak
}

func (s *Basic) DiagramPadding(node *ast.Node) float64 {
	return s.Dims.DiagramPadding
}

func (s *Basic) BlockquoteGeometry(node *ast.Node) BlockquoteGeometry {
	return s.Dims.Blockquote
}

func (s *Basic) TableGeometry(node *ast.Node) TableGeometry {
	return s.Dims.Table
}

func (s *Basic) LineHeight(node *ast.Node) float64 {
	return s.Dims.LineHeight
}
