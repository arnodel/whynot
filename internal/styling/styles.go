// Package styling is whynot's style machinery: the per-element queries
// the layout engine resolves appearance through (Styles), and the
// field-configured implementation simpletheme builds (Basic). It's
// internal so the public API only exposes an opaque whynot.StyleSheet;
// stylesheets are made by packages under styles/.
package styling

import (
	"image/color"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
)

// Margins is the space around a block, in logical (unscaled) pixels.
type Margins struct {
	Top, Bottom, Left, Right float64
}

// TextStyleField identifies one field of fonts.TextStyle, so a
// PartialTextStyle can say which fields it actually sets - needed because
// its zero values (font.WeightNormal, font.StyleNormal, fonts.Proportional)
// are real, meaningful values, not "unset" sentinels.
type TextStyleField uint8

const (
	FieldSize TextStyleField = 1 << iota
	FieldStyle
	FieldWeight
	FieldFamily

	AllTextStyleFields = FieldSize | FieldStyle | FieldWeight | FieldFamily
)

// PartialTextStyle is what a single node's own tag contributes to its
// fonts.TextStyle - only the fields flagged in Set are meaningful; every other
// field is inherited from further up the node's ancestry.
type PartialTextStyle struct {
	fonts.TextStyle
	Set TextStyleField
}

// BlockquoteGeometry is the dimensions a blockquote's bar and indent
// need, read together.
type BlockquoteGeometry struct {
	Indent   float64
	BarWidth float64
}

// TableGeometry is the dimensions a table's layout needs, read together.
type TableGeometry struct {
	FrameThickness      float64
	ColumnGap           float64
	RowGap              float64
	HeaderGap           float64
	ColumnRuleThickness float64
}

// ScrollbarGeometry is a scrollbar thumb's size, in logical (unscaled)
// pixels, and whether it's always shown or fades out when idle.
type ScrollbarGeometry struct {
	// Thickness is the thumb's width across the direction it scrolls,
	// and Inset its distance from the edge it runs along.
	Thickness, Inset float64
	// MinThumbLength keeps a thumb grabbable on long content.
	MinThumbLength float64
	// AlwaysVisible shows the scrollbar whenever the content can scroll,
	// instead of only while scrolling or pointing at it.
	AlwaysVisible bool
}

// Styles resolves an ast.Node's semantic role to concrete appearance - what the
// layout engine queries. Every per-element method takes the node,
// uniformly, even where nothing varies a value by node today - an
// implementation is free to ignore it. Dimensions are in logical
// (unscaled) pixels.
type Styles interface {
	// Margins returns the margins for node's own tag.
	Margins(node *ast.Node) Margins
	// TextStyle returns node's own tag's contribution to its text style -
	// combined with its ancestors' contributions by the engine, since e.g.
	// Strong nested inside Emphasis needs both a bold and an italic
	// contribution to survive. At the root (a nil node) it must set every
	// field, so resolution never leaves one unset.
	TextStyle(node *ast.Node) PartialTextStyle
	// Color returns node's own tag's contribution to the cascading text
	// color, or nil if it has no opinion (inherits from an ancestor, or
	// the document default at the root - a nil node). Mirrors CSS's
	// `color`, which inherits by default.
	Color(node *ast.Node) color.Color
	// BorderColor returns node's own single, non-inherited decoration
	// color - a thematic break's rule, a blockquote's bar, a table's
	// frame. Unlike Color, it's never resolved by walking ancestry -
	// mirrors CSS's border-color, which doesn't inherit.
	BorderColor(node *ast.Node) color.Color
	// BackgroundColor is the color the whole view is filled with.
	BackgroundColor() color.Color
	// ViewMargins is the space between the view's edge and the document.
	ViewMargins() Margins
	// HighlightColor is the text color of whatever's highlighted, e.g. a
	// hovered link, overriding the normal cascade.
	HighlightColor() color.Color
	// ScrollbarColor is the color of a scrollbar thumb: hover whenever
	// it's highlighted (including while dragged), pressed only while
	// dragged.
	ScrollbarColor(hover, pressed bool) color.Color
	// ScrollbarGeometry is a scrollbar's size and visibility.
	ScrollbarGeometry() ScrollbarGeometry

	// StrikeThickness returns the thickness of a strikethrough line at
	// node, or 0 if node isn't (nor is any ancestor) struck through -
	// both "should this be struck" and "how thick".
	StrikeThickness(node *ast.Node) float64
	ThematicBreakThickness(node *ast.Node) float64
	BlockquoteGeometry(node *ast.Node) BlockquoteGeometry
	TableGeometry(node *ast.Node) TableGeometry
	// DiagramPadding is the space between a rendered diagram's frame and
	// the diagram itself.
	DiagramPadding(node *ast.Node) float64
	// LineHeight returns the multiplier applied to a line's natural
	// Ascent+Descent to get the vertical space reserved for it - CSS's
	// unitless line-height (1.2 means 120% of the font's own single-line
	// height). A font's own metrics don't reliably encode comfortable
	// reading spacing, so this is what separates wrapped lines within a
	// paragraph - distinct from Margins, which separate blocks.
	LineHeight(node *ast.Node) float64
}
