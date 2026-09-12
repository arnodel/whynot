package whynot

import (
	"image/color"
)

type RenderingContext struct {
	Scale float64
	FaceSelector
	StyleSheet StyleSheet
}

// The methods below read c.StyleSheet (or, for ScaledMargins, a Marginer -
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

func (c RenderingContext) ScaledStrikeThickness(node *ASTNode) float64 {
	return c.StyleSheet.StrikeThickness(node) * c.Scale
}

func (c RenderingContext) ScaledThematicBreakThickness(node *ASTNode) float64 {
	return c.StyleSheet.ThematicBreakThickness(node) * c.Scale
}

func (c RenderingContext) ScaledBlockquoteGeometry(node *ASTNode) BlockquoteGeometry {
	g := c.StyleSheet.BlockquoteGeometry(node)
	return BlockquoteGeometry{
		Indent:   g.Indent * c.Scale,
		BarWidth: g.BarWidth * c.Scale,
	}
}

func (c RenderingContext) ScaledTableGeometry(node *ASTNode) TableGeometry {
	g := c.StyleSheet.TableGeometry(node)
	return TableGeometry{
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
func (c RenderingContext) ResolvedTextStyle(node *ASTNode) TextStyle {
	var result TextStyle
	var resolved TextStyleField
	for n := node; ; n = n.Parent {
		contrib := c.StyleSheet.TextStyle(n)
		if missing := contrib.Set &^ resolved; missing != 0 {
			if missing&FieldSize != 0 {
				result.Size = contrib.Size
			}
			if missing&FieldStyle != 0 {
				result.Style = contrib.Style
			}
			if missing&FieldWeight != 0 {
				result.Weight = contrib.Weight
			}
			if missing&FieldFamily != 0 {
				result.Family = contrib.Family
			}
			resolved |= missing
		}
		if resolved == allTextStyleFields || n == nil {
			return result
		}
	}
}

// ResolvedColor walks node's ancestry (node itself first) for the nearest
// non-nil Color contribution - mirrors CSS's `color`, which inherits down
// from the nearest ancestor that sets it.
func (c RenderingContext) ResolvedColor(node *ASTNode) color.Color {
	for n := node; ; n = n.Parent {
		if col := c.StyleSheet.Color(n); col != nil {
			return col
		}
		if n == nil {
			return nil
		}
	}
}
