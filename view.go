package whynot

import "github.com/hajimehoshi/ebiten/v2"

// View renders a parsed Markdown document for an ebiten game. It owns the
// layout cache (rebuilt only when width or scale change, not every frame),
// viewport culling, and scroll-position anchoring across resizes - the
// parts of rendering a reflowing document that are easy to get wrong and
// not specific to any one game, so a caller doesn't have to reimplement
// them.
//
// A View doesn't read input itself: call Scroll with deltas from whatever
// input source is appropriate for the embedding game, and call Layout
// whenever the available width or the display scale changes (typically
// from the embedding ebiten.Game's own Layout method).
type View struct {
	block   Block
	ctx     RenderingContext
	offsetY float64

	box      Box
	boxWidth int
	boxScale float64
}

// NewView parses source and returns a View ready to render it once Layout
// has been called at least once to establish a width.
func NewView(source []byte, faceSelector FaceSelector) *View {
	return &View{
		block: Parse(source),
		ctx:   RenderingContext{FaceSelector: faceSelector},
	}
}

// Scroll adjusts the vertical scroll position by dy pixels; positive
// scrolls down.
func (v *View) Scroll(dy float64) {
	v.offsetY += dy
}

// Draw renders the document onto dst with its top-left corner at (x, y),
// at the current scroll position. Content outside dst's bounds is skipped
// rather than drawn and clipped, so Draw's cost tracks what's visible, not
// the document's total size.
func (v *View) Draw(dst *ebiten.Image, x, y int) {
	if v.box == nil {
		return
	}
	DrawBox(v.box, dst, x, int(v.offsetY)+y)
}

// Layout sets the pixel width to render at and the display scale to render
// fonts at (DPI = scale * 72, matching main.go's convention). It's cheap to
// call every frame: the layout tree is only rebuilt when width or scale
// actually change since the last call. On a rebuild, the scroll position is
// re-anchored to the same content rather than the same raw pixel offset, so
// reflowing at a new width doesn't change what's visible.
func (v *View) Layout(width int, scale float64) {
	v.ctx.SetDPI(scale * 72)
	v.ctx.Scale = scale

	if width == v.boxWidth && scale == v.boxScale {
		return
	}

	var anchorIndex int
	var anchorRatio float64
	anchored := false
	if oldStack, ok := v.box.(*StackBox); ok {
		anchorIndex, anchorRatio, anchored = oldStack.anchorAt(-int(v.offsetY))
	}

	v.box = v.block.GetBox(v.ctx, width)
	v.boxWidth = width
	v.boxScale = scale

	if anchored {
		if newStack, ok := v.box.(*StackBox); ok {
			if y, ok := newStack.positionOf(anchorIndex, anchorRatio); ok {
				v.offsetY = -float64(y)
			}
		}
	}
}
