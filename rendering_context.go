package whynot

import (
	"github.com/arnodel/whynot/internal/styling"
	"image/color"
	"time"
)

type RenderingContext struct {
	Scale float64
	FaceSelector
	Styles styling.Styles

	// HighlightNode is the styling.Node currently under the mouse (e.g. a
	// hovered link), or nil - see ResolvedColor. Set by View.Hover, which
	// rebuilds the layout tree when it changes.
	HighlightNode *styling.Node

	// ImageCache resolves, fetches, and decodes images, caching the
	// result - see ImageCache. NewView always sets one (FileImageSource
	// unless overridden via WithImageSource). If nil, images aren't
	// loaded at all: an image renders as its alt text, and a diagram as
	// its source code block.
	ImageCache *ImageCache

	// hscroll is the View's horizontal scrolling state, shared with the
	// ScrollBoxes laid out under this context; nil outside a View.
	hscroll *hscrollState

	// Time is elapsed time since the embedder started rendering (its
	// own reference point - only ever used relative to itself, never
	// compared against a wall-clock timestamp), set every View.Layout
	// call. The one consumer today is AnimatedImage.CurrentFrame,
	// reached via DrawInline - kept here rather than read from a direct
	// time.Now() call so frame selection stays a pure, deterministic
	// function of its inputs, easy to test without any real waiting.
	Time time.Duration
}

// The methods below read c.Styles (or, for ScaledMargins, a Marginer -
// typically a Block) and scale the result by c.Scale in one step - layout
// code should always go through these rather than calling Margins/the
// StyleSheet directly, so scaling can't be forgotten or applied twice.
// TextStyle/Color have no scaled equivalent: font size is scaled via DPI
// on the FaceSelector instead, and color doesn't scale at all.

func (c RenderingContext) ScaledMargins(m Marginer) Margins {
	margins := m.Margins(c)
	margins.Left *= c.Scale
	margins.Right *= c.Scale
	margins.Top *= c.Scale
	margins.Bottom *= c.Scale
	return margins
}

func (c RenderingContext) ScaledViewMargins() Margins {
	margins := c.Styles.ViewMargins()
	margins.Left *= c.Scale
	margins.Right *= c.Scale
	margins.Top *= c.Scale
	margins.Bottom *= c.Scale
	return margins
}

func (c RenderingContext) ScaledStrikeThickness(node *styling.Node) float64 {
	return c.Styles.StrikeThickness(node) * c.Scale
}

func (c RenderingContext) ScaledThematicBreakThickness(node *styling.Node) float64 {
	return c.Styles.ThematicBreakThickness(node) * c.Scale
}

func (c RenderingContext) ScaledBlockquoteGeometry(node *styling.Node) styling.BlockquoteGeometry {
	g := c.Styles.BlockquoteGeometry(node)
	return styling.BlockquoteGeometry{
		Indent:   g.Indent * c.Scale,
		BarWidth: g.BarWidth * c.Scale,
	}
}

func (c RenderingContext) ScaledTableGeometry(node *styling.Node) styling.TableGeometry {
	g := c.Styles.TableGeometry(node)
	return styling.TableGeometry{
		FrameThickness:      g.FrameThickness * c.Scale,
		ColumnGap:           g.ColumnGap * c.Scale,
		RowGap:              g.RowGap * c.Scale,
		HeaderGap:           g.HeaderGap * c.Scale,
		ColumnRuleThickness: g.ColumnRuleThickness * c.Scale,
	}
}

// ResolvedTextStyle merges node's ancestry's TextStyle contributions into
// one TextStyle - the nearest ancestor (node itself first) to set a given
// field wins, so e.g. Strong nested inside Emphasis picks up both a bold
// weight (from Strong) and an italic style (from Emphasis). Walks one step
// past the root (node == nil) so StyleSheet's baseline contribution can
// fill in any field nothing along the way ever claimed.
func (c RenderingContext) ResolvedTextStyle(node *styling.Node) TextStyle {
	var result TextStyle
	var resolved styling.TextStyleField
	for n := node; ; n = n.Parent {
		contrib := c.Styles.TextStyle(n)
		if missing := contrib.Set &^ resolved; missing != 0 {
			if missing&styling.FieldSize != 0 {
				result.Size = contrib.Size
			}
			if missing&styling.FieldStyle != 0 {
				result.Style = contrib.Style
			}
			if missing&styling.FieldWeight != 0 {
				result.Weight = contrib.Weight
			}
			if missing&styling.FieldFamily != 0 {
				result.Family = contrib.Family
			}
			resolved |= missing
		}
		if resolved == styling.AllTextStyleFields || n == nil {
			return result
		}
	}
}

// ResolvedColor walks node's ancestry (node itself first) for the nearest
// non-nil Color contribution - mirrors CSS's `color`, which inherits down
// from the nearest ancestor that sets it.
func (c RenderingContext) ResolvedColor(node *styling.Node) color.Color {
	if node.HasAncestor(c.HighlightNode) {
		return c.Styles.HighlightColor()
	}
	for n := node; ; n = n.Parent {
		if col := c.Styles.Color(n); col != nil {
			return col
		}
		if n == nil {
			return nil
		}
	}
}
