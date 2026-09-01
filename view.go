package whynot

// View renders a parsed Markdown document onto a Canvas. It owns the
// layout cache (rebuilt only when width or scale change, not every frame),
// viewport culling, and scroll-position anchoring across resizes - the
// parts of rendering a reflowing document that are easy to get wrong and
// not specific to any one game, so a caller doesn't have to reimplement
// them.
//
// A View doesn't read input itself: call Scroll with deltas from whatever
// input source is appropriate for the embedding game, and call Layout
// whenever the available width or the display scale changes (typically
// from the embedding game's own layout/resize callback).
type View struct {
	block Block
	ctx   RenderingContext

	// The scroll position, as the slot at the top of the viewport and how
	// far into it (pixels). This is what stays meaningful across a
	// rebuild, unlike a raw pixel offset: reflow can change a slot's
	// height, but not which slot content belongs to.
	index  int
	offset int

	// pendingOffset holds the fractional part of accumulated Scroll deltas
	// that hasn't yet added up to a whole pixel, so slow, sub-pixel wheel
	// deltas don't get rounded away every single call - only once they
	// actually accumulate to a pixel does offset move.
	pendingOffset float64

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

// Scroll adjusts the vertical scroll position by dy pixels, using the same
// sign convention the previous offsetY-based implementation did (offsetY
// += dy, content drawn at localY + offsetY): negative dy moves the anchor
// forward through the document, revealing later content ("scrolling
// down"); positive moves back toward the start.
func (v *View) Scroll(dy float64) {
	v.pendingOffset += dy
	delta := int(v.pendingOffset)
	if delta == 0 {
		return
	}
	v.pendingOffset -= float64(delta)

	stack, ok := v.box.(*StackBox)
	if !ok {
		return
	}
	v.index, v.offset = stack.resolve(v.index, v.offset-delta)
}

// Draw renders the document onto dst with its top-left corner at (x, y),
// at the current scroll position. Content above the current anchor, and
// content outside dst's bounds, is never resolved or drawn - Draw's cost
// tracks what's visible, not the document's total size or how far into it
// the scroll position is.
func (v *View) Draw(dst Canvas, x, y int) {
	stack, ok := v.box.(*StackBox)
	if !ok {
		return
	}
	stack.DrawFrom(dst, v.index, v.offset, x, y)
}

// Layout sets the pixel width to render at and the display scale to render
// fonts at (DPI = scale * 72, matching main.go's convention). It's cheap to
// call every frame: the layout tree is only rebuilt when width or scale
// actually change since the last call. On a rebuild, the scroll position is
// re-anchored to the same content rather than the same raw pixel offset, so
// reflowing at a new width doesn't change what's visible - and since the
// anchor is already (index, offset), this only needs the old and new
// height of that one slot, not a scan of the tree.
func (v *View) Layout(width int, scale float64) {
	v.ctx.SetDPI(scale * 72)
	v.ctx.Scale = scale

	if width == v.boxWidth && scale == v.boxScale {
		return
	}

	ratio := 0.0
	if oldStack, ok := v.box.(*StackBox); ok && v.index < len(oldStack.slots) {
		if h := oldStack.boxAt(v.index).Bounds().Dy(); h > 0 {
			ratio = float64(v.offset) / float64(h)
		}
	}

	v.box = v.block.GetBox(v.ctx, width)
	v.boxWidth = width
	v.boxScale = scale

	newStack, ok := v.box.(*StackBox)
	if !ok || len(newStack.slots) == 0 {
		return
	}
	if v.index >= len(newStack.slots) {
		v.index = len(newStack.slots) - 1
	}
	newHeight := newStack.boxAt(v.index).Bounds().Dy()
	v.index, v.offset = newStack.resolve(v.index, int(ratio*float64(newHeight)))
}
