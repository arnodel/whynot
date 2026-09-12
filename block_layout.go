package whynot

import (
	"image"
	"image/color"
)

// Hit is what HitTest returns for an actual match: something that can
// report its own Bounds() and Source() - every BlockLayout and InlineLayout
// qualifies, so HitTest can return either kind of leaf uniformly. A
// composite's Source() is nil when it aggregates children with no
// single identity of their own (see StackBox.Source), the same way
// StackBlock.Node does for the semantic tree.
type Hit interface {
	Bounds() image.Rectangle
	Source() Source
}

// BlockLayout is drawn via the package-level DrawBlockLayout, not by calling drawContents
// directly, so that every BlockLayout gets the same off-screen skip for free
// regardless of where it sits in the tree. drawContents holds only the
// type-specific drawing logic.
//
// HitTest identifies what's at p, a point in this BlockLayout's own Bounds()
// frame - callers must check p is in Bounds() first, since an
// implementation never re-checks. A nil hit means no match (dead space,
// e.g. a margin or padding); offset is where hit.Bounds() should be
// placed to land in the same frame p arrived in - a composite BlockLayout
// recursing into a child must add its own offset to the child's.
type BlockLayout interface {
	Bounds() image.Rectangle
	Source() Source
	drawContents(dst Canvas, x, y int)
	HitTest(p image.Point) (hit Hit, offset image.Point)
}

// DrawBlockLayout is the sole entry point for drawing a BlockLayout: it skips drawContents
// entirely when box's bounds don't overlap dst, so every BlockLayout gets that for
// free regardless of who's calling it or where it sits in the tree.
func DrawBlockLayout(box BlockLayout, dst Canvas, x, y int) {
	if !box.Bounds().Add(image.Pt(x, y)).Overlaps(dst.Bounds()) {
		return
	}
	box.drawContents(dst, x, y)
}

type LineBox struct {
	parts []InlineLayout

	boundsComputed bool
	bounds         image.Rectangle
	advance        int
}

var _ BlockLayout = (*LineBox)(nil)

// Source is always nil: a line can mix parts with different identities
// (e.g. plain text next to emphasized text), so there's no single node
// to report at the line level - HitTest already returns the specific
// part that matched instead.
func (b *LineBox) Source() Source {
	return nil
}

// Same reasoning as TextBox: parts are fixed at construction and a
// LineBox is never reused across a re-layout, so this is safe to compute
// once and reuse for the instance's whole life. Inter-word spacing here
// must match drawContents/splitBoxes exactly (each word's own
// SpaceWidth(), not a fixed value) - this is what a caller measures via
// Bounds() to decide how much room the line actually needs, so any
// mismatch with what actually gets drawn silently under- or
// over-reports it.
func (b *LineBox) BoundsAndAdvance() (image.Rectangle, int) {
	if !b.boundsComputed {
		bounds, advance := b.parts[0].BoundsAndAdvance()
		left := bounds.Min.X
		if left < 0 {
			bounds = bounds.Add(image.Pt(-left, 0))
			advance -= left
		}
		prevSpace := b.parts[0].SpaceWidth()
		for _, box := range b.parts[1:] {
			space := box.SpaceWidth()
			advance += maxInt(prevSpace, space)
			prevSpace = space
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

// Bounds only compensates for a negative bounds.Min.X (an overshooting
// left bearing), not a positive one: a code line's leading indentation
// has real advance but no ink, so it's occupied space starting at this
// box's own x=0, not excess to crop away. Cropping it under-reported
// indented lines' width and broke hit-testing on their right-hand side.
func (b *LineBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	left := bounds.Min.X
	if left > 0 {
		left = 0
	}
	return image.Rect(0, 0, bounds.Max.X-left, bounds.Dy())
}

// HitTest mirrors drawContents' own accumulation loop, calling each
// part's HitTest instead of DrawInline at each step, so the two can't
// drift apart. p needs no shift: y is the same internal y drawContents
// computes for external y=0, which already lands a part's footprint in
// Bounds()'s frame - the same frame p arrives in.
func (b *LineBox) HitTest(p image.Point) (Hit, image.Point) {
	lineBounds, _ := b.BoundsAndAdvance()
	y := -lineBounds.Min.Y

	bounds, _ := b.parts[0].BoundsAndAdvance()
	x := 0
	if left := bounds.Min.X; left < 0 {
		x -= left
	}
	prevSpace := b.parts[0].SpaceWidth()

	hit, offset, next := b.parts[0].HitTest(p, x, y)
	if hit != nil {
		return hit, offset
	}
	x = next
	for _, part := range b.parts[1:] {
		space := part.SpaceWidth()
		hit, offset, next := part.HitTest(p, x+maxInt(prevSpace, space), y)
		if hit != nil {
			return hit, offset
		}
		x = next
		prevSpace = space
	}
	return nil, image.Point{}
}

func (b *LineBox) drawContents(dst Canvas, x, y int) {
	lineBounds, _ := b.BoundsAndAdvance()
	y -= lineBounds.Min.Y

	bounds, _ := b.parts[0].BoundsAndAdvance()
	left := bounds.Min.X
	if left < 0 {
		x -= left
	}
	prevSpace := b.parts[0].SpaceWidth()

	x = b.parts[0].DrawInline(dst, x, y)
	for _, box := range b.parts[1:] {
		space := box.SpaceWidth()
		x = box.DrawInline(dst, x+maxInt(prevSpace, space), y)
		prevSpace = space
	}
}

// stackSlot is a StackBox child that's either already resolved (box set -
// true of gap spacers, which are cheap enough to build eagerly) or needs
// building from a Block on first access (block set). A content slot's
// width is already margin-reduced if wrap is set.
type stackSlot struct {
	box   BlockLayout
	block Block

	width      int
	leftMargin int
	wrap       bool
}

type StackBox struct {
	slots []stackSlot
	ctx   RenderingContext
	width int

	// source is the single Block this StackBox's slots were all built
	// from - e.g. a paragraph's wrapped lines - or nil when the slots
	// are an aggregate of otherwise-unrelated blocks (StackBlock.GetBlockLayout's
	// own StackBox, including the top-level document). See Source.
	source Block

	boundsComputed bool
	bounds         image.Rectangle
}

// preResolvedSlots wraps already-built boxes as pre-resolved slots, for
// StackBoxes whose children are cheap to build up front (e.g. a
// paragraph's lines: line-breaking is inherently a whole-paragraph
// computation, so there's nothing to defer per-line the way there is per
// top-level block in StackBlock.GetBlockLayout).
func preResolvedSlots(boxes []BlockLayout) []stackSlot {
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
func asStackBox(box BlockLayout) *StackBox {
	if stack, ok := box.(*StackBox); ok {
		return stack
	}
	return &StackBox{slots: preResolvedSlots([]BlockLayout{box})}
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

func (b *StackBox) Source() Source {
	return b.source
}

// boxAt returns the child at index i, building it from its Block and
// memoizing the result on first access if it isn't already resolved. A
// caller that only ever asks for slots near a scroll cursor only ever
// pays to build those.
func (b *StackBox) boxAt(i int) BlockLayout {
	slot := &b.slots[i]
	if slot.box == nil {
		inner := slot.block.GetBlockLayout(b.ctx, slot.width)
		if slot.wrap {
			slot.box = NewContainerBox(inner, b.width, inner.Bounds().Dy(), slot.leftMargin, 0)
		} else {
			slot.box = inner
		}
	}
	return slot.box
}

// HitTest walks slots from the start looking for the one p.Y falls in -
// a plain linear scan, unlike normalizeCursor's cursor-relative walk,
// since resolving this way would force-build every slot up to the match
// regardless. That's fine for a StackBox reached via nested structure
// (a list's items, a blockquote's paragraphs - typically small), but
// would defeat the point for the top-level, potentially huge document
// StackBox - View.HitTest handles that one specially, starting from the
// current scroll cursor instead of slot 0.
//
// A miss on the matched slot (outside its bounds, or its own HitTest
// declines - e.g. clicking past the end of a paragraph's short last
// line) falls back to this StackBox itself when it has a Source - the
// same fallback-to-self BlockquoteBox/TableBox already do. When it
// doesn't (an aggregate of unrelated blocks - see Source), it's a
// genuine miss instead, same as before Source existed.
func (b *StackBox) HitTest(p image.Point) (Hit, image.Point) {
	y := 0
	for i := range b.slots {
		box := b.boxAt(i)
		h := box.Bounds().Dy()
		if p.Y < y+h {
			local := image.Pt(p.X, p.Y-y)
			if local.In(box.Bounds()) {
				if hit, offset := box.HitTest(local); hit != nil {
					return hit, offset.Add(image.Pt(0, y))
				}
			}
			if b.source != nil {
				return b, image.Point{}
			}
			return nil, image.Point{}
		}
		y += h
	}
	return nil, image.Point{}
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

func (b *StackBox) drawContents(dst Canvas, x, y int) {
	viewport := dst.Bounds()
	for i := range b.slots {
		box := b.boxAt(i)
		childBounds := box.Bounds()
		if childBounds.Add(image.Pt(x, y)).Min.Y > viewport.Max.Y {
			// This child, and every one after it, starts below the
			// viewport: nothing further down can be visible.
			break
		}
		DrawBlockLayout(box, dst, x, y)
		y += childBounds.Max.Y
	}
}

// DrawFrom draws starting at c, so that c's position lands at (x, y) on
// dst - unlike DrawBlockLayout, it never calls Bounds() on the whole tree first,
// and never resolves or draws slots before c.index. Intended for View to
// call at the scroll cursor.
func (b *StackBox) DrawFrom(dst Canvas, c stackCursor, x, y int) {
	if c.index < 0 || c.index >= len(b.slots) {
		return
	}
	viewport := dst.Bounds()
	y -= int(c.offset)
	for i := c.index; i < len(b.slots); i++ {
		box := b.boxAt(i)
		childBounds := box.Bounds()
		if childBounds.Add(image.Pt(x, y)).Min.Y > viewport.Max.Y {
			break
		}
		DrawBlockLayout(box, dst, x, y)
		y += childBounds.Max.Y
	}
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

func (b *EmptyBox) Source() Source {
	return nil
}

// HitTest always misses: an EmptyBox is pure spacing (a margin/gap
// between blocks or lines), never content.
func (b *EmptyBox) HitTest(p image.Point) (Hit, image.Point) {
	return nil, image.Point{}
}

func (b *EmptyBox) drawContents(dst Canvas, x, y int) {
}

type ContainerBox struct {
	bounds   image.Rectangle
	innerPos image.Point
	inner    BlockLayout
}

func NewContainerBox(inner BlockLayout, w, h, x, y int) *ContainerBox {
	return &ContainerBox{
		bounds:   image.Rect(0, 0, w, h),
		innerPos: image.Pt(x, y),
		inner:    inner,
	}
}

func (b *ContainerBox) Bounds() image.Rectangle {
	return b.bounds
}

// Source delegates to inner: a pure positioning wrapper (for
// indentation, or cell alignment in a table) has no identity of its
// own, but always wraps exactly one thing, unlike StackBox.
func (b *ContainerBox) Source() Source {
	return b.inner.Source()
}

// HitTest has no source of its own - a pure positioning wrapper (for
// indentation, or cell alignment in a table) - so a miss on inner (e.g.
// content narrower than the space it was given) reports no match rather
// than falling back to anything.
func (b *ContainerBox) HitTest(p image.Point) (Hit, image.Point) {
	local := p.Sub(b.innerPos)
	if !local.In(b.inner.Bounds()) {
		return nil, image.Point{}
	}
	hit, offset := b.inner.HitTest(local)
	if hit == nil {
		return nil, image.Point{}
	}
	return hit, offset.Add(b.innerPos)
}

func (b *ContainerBox) drawContents(dst Canvas, x, y int) {
	DrawBlockLayout(b.inner, dst, x+b.innerPos.X, y+b.innerPos.Y)
}

// BlockquoteBox draws a vertical bar down the left edge and positions its
// inner content (the quote's own blocks, already laid out at the reduced
// width) to the right of it - the visual marker for a `>` blockquote.
type BlockquoteBox struct {
	width    int
	indent   int
	barWidth int
	barColor color.Color
	inner    BlockLayout

	// source is the BlockquoteBlock this box was built from - see Source.
	source Block
}

var _ BlockLayout = (*BlockquoteBox)(nil)

func (b *BlockquoteBox) Source() Source {
	return b.source
}

func (b *BlockquoteBox) Bounds() image.Rectangle {
	return image.Rect(0, 0, b.width, b.inner.Bounds().Dy())
}

func (b *BlockquoteBox) drawContents(dst Canvas, x, y int) {
	dst.DrawRect(x, y, b.barWidth, b.inner.Bounds().Dy(), b.barColor)
	DrawBlockLayout(b.inner, dst, x+b.indent, y)
}

// HitTest treats the whole indent strip (bar plus any padding before
// inner starts) as the blockquote's own, and falls back to it too
// whenever inner declines to match (content narrower than the indented
// width, or a gap between quoted paragraphs) - every HitTest
// implementation degrades gracefully on an out-of-bounds point, so
// there's no need to bounds-check before delegating.
func (b *BlockquoteBox) HitTest(p image.Point) (Hit, image.Point) {
	if p.X >= b.indent {
		if hit, offset := b.inner.HitTest(image.Pt(p.X-b.indent, p.Y)); hit != nil {
			return hit, offset.Add(image.Pt(b.indent, 0))
		}
	}
	return b, image.Point{}
}

// TableBox draws a GFM table: a frame around the whole thing, a rule
// under the header row, a rule between each pair of columns, and each
// cell positioned at its resolved (column, row) offset. columnOffsets/
// rowOffsets have one more entry than there are columns/rows - the last
// entry is the table's own right/bottom edge, so Bounds() doesn't need
// separate width/height fields, and the header rule's position is just
// rowOffsets[1] (see TableBlock.GetBlockLayout for why that boundary is exactly
// where the rule belongs). Each column rule is centered in the columnGap
// between adjacent columns' content, at columnOffsets[c] - columnGap/2.
type TableBox struct {
	columnOffsets       []int
	rowOffsets          []int
	frameThickness      int
	columnGap           int
	columnRuleThickness int
	frameColor          color.Color
	cells               [][]BlockLayout

	// source is the TableBlock this box was built from - see Source.
	source Block
}

var _ BlockLayout = (*TableBox)(nil)

func (b *TableBox) Source() Source {
	return b.source
}

func (b *TableBox) Bounds() image.Rectangle {
	return image.Rect(0, 0,
		b.columnOffsets[len(b.columnOffsets)-1],
		b.rowOffsets[len(b.rowOffsets)-1],
	)
}

func (b *TableBox) drawContents(dst Canvas, x, y int) {
	width := b.columnOffsets[len(b.columnOffsets)-1]
	height := b.rowOffsets[len(b.rowOffsets)-1]

	dst.DrawRect(x, y, width, b.frameThickness, b.frameColor)                         // top
	dst.DrawRect(x, y+height-b.frameThickness, width, b.frameThickness, b.frameColor) // bottom
	dst.DrawRect(x, y, b.frameThickness, height, b.frameColor)                        // left
	dst.DrawRect(x+width-b.frameThickness, y, b.frameThickness, height, b.frameColor) // right

	// Flush against the top of the first body row - rowOffsets[1] minus
	// the rule's own thickness, so it sits inside the header->body gap
	// rather than overlapping either row's content.
	dst.DrawRect(x+b.frameThickness, y+b.rowOffsets[1]-b.frameThickness,
		width-2*b.frameThickness, b.frameThickness, b.frameColor)

	// One rule per internal column boundary (columnOffsets[0] and
	// columnOffsets[len-1] are the table's own edges, already covered by
	// the frame), centered in that column's gap and spanning the full
	// height between the top and bottom frame.
	for c := 1; c < len(b.columnOffsets)-1; c++ {
		ruleX := b.columnOffsets[c] - b.columnGap/2 - b.columnRuleThickness/2
		dst.DrawRect(x+ruleX, y+b.frameThickness,
			b.columnRuleThickness, height-2*b.frameThickness, b.frameColor)
	}

	for row := range b.cells {
		for col := range b.cells[row] {
			DrawBlockLayout(b.cells[row][col], dst, x+b.columnOffsets[col], y+b.rowOffsets[row])
		}
	}
}

// HitTest finds which cell (row, col) p falls in via columnOffsets/
// rowOffsets, and falls back to the table's own source whenever there
// isn't one to recurse into (the frame, a gap between cells/rows) or the
// cell itself declines (its content is narrower/shorter than the cell's
// allotted space).
func (b *TableBox) HitTest(p image.Point) (Hit, image.Point) {
	row := -1
	for r := 0; r < len(b.rowOffsets)-1; r++ {
		if p.Y >= b.rowOffsets[r] && p.Y < b.rowOffsets[r+1] {
			row = r
			break
		}
	}
	col := -1
	for c := 0; c < len(b.columnOffsets)-1; c++ {
		if p.X >= b.columnOffsets[c] && p.X < b.columnOffsets[c+1] {
			col = c
			break
		}
	}
	if row >= 0 && col >= 0 {
		local := image.Pt(p.X-b.columnOffsets[col], p.Y-b.rowOffsets[row])
		if hit, offset := b.cells[row][col].HitTest(local); hit != nil {
			return hit, offset.Add(image.Pt(b.columnOffsets[col], b.rowOffsets[row]))
		}
	}
	return b, image.Point{}
}

// RuleBox is a single filled horizontal bar - the box for a thematic break
// (`---`). Its own height is just the bar's thickness; the visual spacing
// above and below comes from ThematicBreakBlock's Margins, same as any
// other block.
type RuleBox struct {
	width     int
	thickness int
	color     color.Color

	// source is the ThematicBreakBlock this box was built from - see
	// Source.
	source Block
}

var _ BlockLayout = (*RuleBox)(nil)

func (b *RuleBox) Source() Source {
	return b.source
}

func (b *RuleBox) Bounds() image.Rectangle {
	return image.Rect(0, 0, b.width, b.thickness)
}

func (b *RuleBox) drawContents(dst Canvas, x, y int) {
	dst.DrawRect(x, y, b.width, b.thickness, b.color)
}

func (b *RuleBox) HitTest(p image.Point) (Hit, image.Point) {
	return b, image.Point{}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
