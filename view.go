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

	// cursor is the scroll position: which slot is at the top of the
	// viewport, and how far into it. This is what stays meaningful across
	// a rebuild, unlike a raw pixel offset - reflow can change a slot's
	// height, but not which slot content belongs to.
	cursor stackCursor

	box      *StackBox
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

// Scroll adjusts the vertical scroll position by dy pixels: negative dy
// moves the cursor forward through the document, revealing later content
// ("scrolling down"); positive moves back toward the start. This matches
// ebiten.Wheel()'s dy passed straight through, so callers don't need to
// negate it.
func (v *View) Scroll(dy float64) {
	if v.box == nil {
		return
	}
	v.cursor = v.box.resolve(stackCursor{index: v.cursor.index, offset: v.cursor.offset - dy})
}

// Draw renders the document onto dst with its top-left corner at (x, y),
// at the current scroll position. Content above the current cursor, and
// content outside dst's bounds, is never resolved or drawn - Draw's cost
// tracks what's visible, not the document's total size or how far into it
// the scroll position is.
func (v *View) Draw(dst Canvas, x, y int) {
	if v.box == nil {
		return
	}
	v.box.DrawFrom(dst, v.cursor, x, y)
}

// Layout sets the pixel width and display scale to render at (DPI = scale
// * 72, matching main.go's convention). Cheap to call every frame: the
// layout tree only rebuilds when width or scale actually change. A
// rebuild re-anchors the scroll position to the same content, not the
// same pixel offset, so reflowing doesn't change what's visible - and
// since the cursor is already (index, offset), this only needs one old
// and one new height, not a scan of the tree.
func (v *View) Layout(width int, scale float64) {
	v.ctx.SetDPI(scale * 72)
	v.ctx.Scale = scale

	if width == v.boxWidth && scale == v.boxScale {
		return
	}

	ratio := 0.0
	if v.box != nil && v.cursor.index < len(v.box.slots) {
		if h := v.box.boxAt(v.cursor.index).Bounds().Dy(); h > 0 {
			ratio = v.cursor.offset / float64(h)
		}
	}

	v.box = asStackBox(v.block.GetBox(v.ctx, width))
	v.boxWidth = width
	v.boxScale = scale

	if len(v.box.slots) == 0 {
		return
	}
	if v.cursor.index >= len(v.box.slots) {
		v.cursor.index = len(v.box.slots) - 1
	}
	newHeight := v.box.boxAt(v.cursor.index).Bounds().Dy()
	v.cursor = v.box.resolve(stackCursor{index: v.cursor.index, offset: ratio * float64(newHeight)})
}
