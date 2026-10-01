package whynot

import (
	"image/color"
	"log"
	"sync"
	"time"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/imagecache"
	"github.com/arnodel/whynot/internal/styling"
)

type RenderingContext struct {
	Scale        float64
	FaceSelector fonts.FaceSelector
	Styles       styling.Styles

	// HighlightNode is the ast.Node currently under the mouse (e.g. a
	// hovered link), or nil - see ResolvedColor. Set by View.Hover, which
	// rebuilds the layout tree when it changes.
	HighlightNode *ast.Node

	// ImageCache resolves, fetches, and decodes images, caching the
	// result - see imagecache.Cache. NewView always sets one (images.FileSource
	// unless overridden via WithImageSource). If nil, images aren't
	// loaded at all: an image renders as its alt text, and a diagram as
	// its source code block.
	ImageCache *imagecache.Cache

	// hscroll is the View's horizontal scrolling state, shared with the
	// ScrollBoxes laid out under this context; nil outside a View.
	hscroll *hscrollState

	// Time is elapsed time since the embedder started rendering (its
	// own reference point - only ever used relative to itself, never
	// compared against a wall-clock timestamp), set every View.Layout
	// call. The one consumer today is imagecache.Animation.CurrentFrame,
	// reached via DrawInline - kept here rather than read from a direct
	// time.Now() call so frame selection stays a pure, deterministic
	// function of its inputs, easy to test without any real waiting.
	Time time.Duration
}

// The methods below read c.Styles (or, for ScaledMargins, a Marginer -
// typically a Block) and scale the result by c.Scale in one step - layout
// code should always go through these rather than calling Margins/the
// StyleSheet directly, so scaling can't be forgotten or applied twice.
// TextStyle/Color have no scaled equivalent: font size is scaled by the
// dpi selectFace passes, and color doesn't scale at all.

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

func (c RenderingContext) ScaledStrikeThickness(node *ast.Node) float64 {
	return c.Styles.StrikeThickness(node) * c.Scale
}

func (c RenderingContext) ScaledThematicBreakThickness(node *ast.Node) float64 {
	return c.Styles.ThematicBreakThickness(node) * c.Scale
}

func (c RenderingContext) ScaledBlockquoteGeometry(node *ast.Node) styling.BlockquoteGeometry {
	g := c.Styles.BlockquoteGeometry(node)
	return styling.BlockquoteGeometry{
		Indent:   g.Indent * c.Scale,
		BarWidth: g.BarWidth * c.Scale,
	}
}

func (c RenderingContext) ScaledTableGeometry(node *ast.Node) styling.TableGeometry {
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
func (c RenderingContext) ResolvedTextStyle(node *ast.Node) fonts.TextStyle {
	var result fonts.TextStyle
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
func (c RenderingContext) ResolvedColor(node *ast.Node) color.Color {
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

// fallbackFaces serves text when the FaceSelector fails, so a missing font
// degrades to the bundled Go fonts rather than breaking layout. Shared by
// every View, so guarded.
var (
	fallbackFaces   = fonts.NewGoSelector()
	fallbackFacesMu sync.Mutex
	logFaceError    sync.Once
)

// selectFace returns the face for style at the context's scale: dpi is
// Scale × 72, so zooming magnifies a font's design rather than changing
// its point size. If the FaceSelector fails, it falls back to the bundled
// Go fonts, logging the first failure.
func (c RenderingContext) selectFace(style fonts.TextStyle) font.Face {
	dpi := c.Scale * 72
	face, err := c.FaceSelector.SelectFace(style, dpi)
	if err == nil {
		return face
	}
	logFaceError.Do(func() {
		log.Printf("whynot: no face for %+v (%v), using the Go fonts instead", style, err)
	})
	fallbackFacesMu.Lock()
	defer fallbackFacesMu.Unlock()
	if face, err = fallbackFaces.SelectFace(style, dpi); err == nil {
		return face
	}
	// The Go fonts lack this exact style (e.g. bold small caps); their
	// regular face always exists.
	face, err = fallbackFaces.SelectFace(fonts.TextStyle{Size: style.Size}, dpi)
	if err != nil {
		panic(err) // the bundled fonts themselves failing is a bug
	}
	return face
}
