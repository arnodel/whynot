package whynot

import (
	"image"
	"image/color"

	"golang.org/x/image/font"
)

// Box is drawn via the package-level DrawBox, not by calling drawContents
// directly, so that every Box gets the same off-screen skip for free
// regardless of where it sits in the tree. drawContents holds only the
// type-specific drawing logic.
type Box interface {
	Bounds() image.Rectangle
	drawContents(dst Canvas, x, y int)
}

type InlineBox interface {
	BoundsAndAdvance() (image.Rectangle, int)
	SpaceWidth() int
	DrawInline(dst Canvas, x, y int) int
}

type TextBox struct {
	Text  string
	Face  font.Face
	Color color.Color

	boundsComputed bool
	bounds         image.Rectangle
	advance        int

	spaceComputed bool
	spaceWidth    int
}

var _ InlineBox = (*TextBox)(nil)

// Text/Face never change after construction, and a TextBox is always
// rebuilt from scratch (never mutated) whenever the source Block tree is
// re-laid out, so this measurement is valid for the entire lifetime of
// the instance: compute it once, on first use.
func (b *TextBox) BoundsAndAdvance() (image.Rectangle, int) {
	if !b.boundsComputed {
		bounds, advance := font.BoundString(b.Face, b.Text)
		metrics := b.Face.Metrics()
		b.bounds = image.Rect(
			bounds.Min.X.Floor(),
			-metrics.Ascent.Ceil(),
			bounds.Max.X.Ceil(),
			metrics.Descent.Ceil(),
		)
		b.advance = advance.Ceil()
		b.boundsComputed = true
	}
	return b.bounds, b.advance
}

func (b *TextBox) SpaceWidth() int {
	if !b.spaceComputed {
		adv, _ := b.Face.GlyphAdvance(' ')
		b.spaceWidth = adv.Ceil()
		b.spaceComputed = true
	}
	return b.spaceWidth
}

type ListItemMarkerBox struct {
	Marker InlineBox
}

var _ InlineBox = (*ListItemMarkerBox)(nil)

func (b *ListItemMarkerBox) BoundsAndAdvance() (image.Rectangle, int) {
	bounds, _ := b.Marker.BoundsAndAdvance()
	return image.Rect(0, bounds.Min.Y, 0, bounds.Max.Y), 0
}

func (b *ListItemMarkerBox) SpaceWidth() int {
	return b.Marker.SpaceWidth()
}

type ImageBox struct {
	src    string
	bounds image.Rectangle
}

var _ InlineBox = (*ImageBox)(nil)

func (b *ImageBox) BoundsAndAdvance() (image.Rectangle, int) {
	return b.bounds, b.bounds.Dx()
}

func (b *ImageBox) SpaceWidth() int {
	return 0
}

type LineBox struct {
	parts []InlineBox
	space int

	boundsComputed bool
	bounds         image.Rectangle
	advance        int
}

var _ Box = (*LineBox)(nil)

// Same reasoning as TextBox: parts/space are fixed at construction and a
// LineBox is never reused across a re-layout, so this is safe to compute
// once and reuse for the instance's whole life.
func (b *LineBox) BoundsAndAdvance() (image.Rectangle, int) {
	if !b.boundsComputed {
		bounds, advance := b.parts[0].BoundsAndAdvance()
		left := bounds.Min.X
		if left < 0 {
			bounds = bounds.Add(image.Pt(-left, 0))
			advance -= left
		}
		for _, box := range b.parts[1:] {
			advance += b.space
			boxBounds, boxAdvance := box.BoundsAndAdvance()
			bounds = bounds.Union(boxBounds.Add(image.Pt(advance, 0)))
			advance += boxAdvance
		}
		b.bounds = bounds
		b.advance = advance
		b.boundsComputed = true
	}
	return b.bounds, b.advance
}

func (b *LineBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	return bounds.Sub(bounds.Min)
}

// stackSlot is a StackBox child that's either already resolved (box set -
// true of gap spacers, which are cheap enough to build eagerly) or needs
// building from a Block on first access (block set). A content slot's
// width is already margin-reduced if wrap is set.
type stackSlot struct {
	box   Box
	block Block

	width      int
	leftMargin int
	wrap       bool
}

type StackBox struct {
	slots []stackSlot
	ctx   RenderingContext
	width int

	boundsComputed bool
	bounds         image.Rectangle
}

// preResolvedSlots wraps already-built boxes as pre-resolved slots, for
// StackBoxes whose children are cheap to build up front (e.g. a
// paragraph's lines: line-breaking is inherently a whole-paragraph
// computation, so there's nothing to defer per-line the way there is per
// top-level block in StackBlock.GetBox).
func preResolvedSlots(boxes []Box) []stackSlot {
	slots := make([]stackSlot, len(boxes))
	for i, box := range boxes {
		slots[i] = stackSlot{box: box}
	}
	return slots
}

// asStackBox returns box already a *StackBox, or wraps it as a single-slot
// one otherwise, so callers that need cursor-based scrolling (View) always
// have a StackBox to work with regardless of what a document's top-level
// Block produces.
func asStackBox(box Box) *StackBox {
	if stack, ok := box.(*StackBox); ok {
		return stack
	}
	return &StackBox{slots: preResolvedSlots([]Box{box})}
}

func (b *StackBox) Bounds() image.Rectangle {
	if !b.boundsComputed {
		var bounds image.Rectangle
		for i := range b.slots {
			box := b.boxAt(i)
			bounds = bounds.Union(box.Bounds().Add(image.Pt(0, bounds.Max.Y)))
		}
		b.bounds = bounds
		b.boundsComputed = true
	}
	return b.bounds
}

// boxAt returns the child at index i, building it from its Block and
// memoizing the result on first access if it isn't already resolved. A
// caller that only ever asks for slots near a scroll cursor only ever
// pays to build those.
func (b *StackBox) boxAt(i int) Box {
	slot := &b.slots[i]
	if slot.box == nil {
		inner := slot.block.GetBox(b.ctx, slot.width)
		if slot.wrap {
			slot.box = NewContainerBox(inner, b.width, inner.Bounds().Dy(), slot.leftMargin, 0)
		} else {
			slot.box = inner
		}
	}
	return slot.box
}

// stackCursor is a position within a StackBox: which slot, and how far
// into it. Like a text cursor, it's only meaningful relative to the
// specific StackBox it was resolved against - the same (index, offset)
// pair means something different for a different StackBox. offset is
// float64 so scroll deltas can accumulate sub-pixel amounts directly;
// DrawFrom truncates to a pixel only right before it becomes a screen
// coordinate.
type stackCursor struct {
	index  int
	offset float64
}

// normalizeCursor adjusts c so that 0 <= offset < boxAt(index)'s height,
// walking to neighboring slots as needed rather than scanning from the
// start: only the slots actually walked over are touched (via boxAt), so
// cost is proportional to how far a position moved, not to how far into
// the document it already was.
//
// Past the very end of the document, normalizeCursor clamps to (lastIndex,
// height(lastIndex)) - offset equal to the height, not less than it -
// rather than reporting an ever-growing out-of-range offset. normalizeCursor
// is idempotent everywhere except exactly that clamped value.
func (b *StackBox) normalizeCursor(c stackCursor) stackCursor {
	if len(b.slots) == 0 {
		return stackCursor{}
	}
	index, offset := c.index, c.offset

	for offset < 0 && index > 0 {
		index--
		offset += float64(b.boxAt(index).Bounds().Dy())
	}
	if offset < 0 {
		// Walked back to the very first slot and it's still negative:
		// this position is above the top of the document.
		return stackCursor{}
	}

	for {
		h := float64(b.boxAt(index).Bounds().Dy())
		if offset < h {
			return stackCursor{index: index, offset: offset}
		}
		if index == len(b.slots)-1 {
			// Walked to the very last slot and offset is still at or past
			// its bottom edge: this position is past the end of the
			// document. Clamp - see the invariant exception above.
			return stackCursor{index: index, offset: h}
		}
		offset -= h
		index++
	}
}

// moveCursor returns c shifted by dy (in the offset's own sign convention:
// positive moves forward through the document) and normalized, for callers
// that have a cursor already and want to move it rather than construct a
// new one from scratch - e.g. View.Scroll. Layout's resize re-anchoring
// isn't a move like this (offset is recomputed from a ratio through a
// slot's new height, not shifted from its old value), so it calls
// normalizeCursor directly instead.
func (b *StackBox) moveCursor(c stackCursor, dy float64) stackCursor {
	return b.normalizeCursor(stackCursor{index: c.index, offset: c.offset + dy})
}

type EmptyBox struct {
	bounds image.Rectangle
}

func NewEmptyBox(w, h int) *EmptyBox {
	return &EmptyBox{
		bounds: image.Rect(0, 0, w, h),
	}
}

func (b *EmptyBox) Bounds() image.Rectangle {
	return b.bounds
}

type ContainerBox struct {
	bounds   image.Rectangle
	innerPos image.Point
	inner    Box
}

func NewContainerBox(inner Box, w, h, x, y int) *ContainerBox {
	return &ContainerBox{
		bounds:   image.Rect(0, 0, w, h),
		innerPos: image.Pt(x, y),
		inner:    inner,
	}
}

func (b *ContainerBox) Bounds() image.Rectangle {
	return b.bounds
}

// BlockquoteBox draws a vertical bar down the left edge and positions its
// inner content (the quote's own blocks, already laid out at the reduced
// width) to the right of it - the visual marker for a `>` blockquote.
type BlockquoteBox struct {
	width    int
	indent   int
	barWidth int
	barColor color.Color
	inner    Box
}

var _ Box = (*BlockquoteBox)(nil)

func (b *BlockquoteBox) Bounds() image.Rectangle {
	return image.Rect(0, 0, b.width, b.inner.Bounds().Dy())
}

func (b *BlockquoteBox) drawContents(dst Canvas, x, y int) {
	dst.DrawRect(x, y, b.barWidth, b.inner.Bounds().Dy(), b.barColor)
	DrawBox(b.inner, dst, x+b.indent, y)
}

// RuleBox is a single filled horizontal bar - the box for a thematic break
// (`---`). Its own height is just the bar's thickness; the visual spacing
// above and below comes from ThematicBreakBlock's Margins, same as any
// other block.
type RuleBox struct {
	width     int
	thickness int
	color     color.Color
}

var _ Box = (*RuleBox)(nil)

func (b *RuleBox) Bounds() image.Rectangle {
	return image.Rect(0, 0, b.width, b.thickness)
}

func (b *RuleBox) drawContents(dst Canvas, x, y int) {
	dst.DrawRect(x, y, b.width, b.thickness, b.color)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
