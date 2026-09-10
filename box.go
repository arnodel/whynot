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
//
// hitTest identifies what's at p, a point in this Box's own Bounds()
// frame - the caller (BoxAt, or a parent Box recursing into a child) is
// responsible for first checking p actually falls within Bounds(), the
// same way DrawBox checks Overlaps() before calling drawContents; a
// hitTest implementation never re-checks its own bounds. It returns the
// Source of whichever leaf p landed on and that leaf's own bounds - in
// the same frame p arrived in, so a caller can e.g. draw an outline
// around it directly - or ok=false if p fell in dead space with no
// content of its own (a margin/gap EmptyBox, or padding around a child
// narrower than its container). A composite Box that recurses into a
// child is responsible for shifting the returned bounds by whatever
// offset it used to get there, the same way it would if it were
// positioning that child for drawing.
type Box interface {
	Bounds() image.Rectangle
	drawContents(dst Canvas, x, y int)
	hitTest(p image.Point) (source Source, bounds image.Rectangle, ok bool)
}

// hitTestChild is the common pattern composite Box types use to recurse:
// if p actually falls within child's own bounds, recurse into it (the
// caller still has to shift the returned bounds by child's own offset);
// otherwise p is dead space within the parent but outside child (e.g. a
// child narrower than the width it was given), so fall back to the
// parent's own source and bounds, if it has one.
func hitTestChild(child Box, p image.Point, self Source, selfBounds image.Rectangle, selfOK bool) (Source, image.Rectangle, bool) {
	if p.In(child.Bounds()) {
		return child.hitTest(p)
	}
	return self, selfBounds, selfOK
}

// InlineBox's hitTest mirrors DrawInline's own calling convention exactly
// - same (x, y), same "next x" return - so LineBox.hitTest can drive the
// identical accumulation loop drawContents does, just calling hitTest
// instead of DrawInline at each step. p, and the returned bounds, are in
// the same frame BoundsAndAdvance/DrawInline already use - see
// LineBox.hitTest.
type InlineBox interface {
	BoundsAndAdvance() (image.Rectangle, int)
	SpaceWidth() int
	DrawInline(dst Canvas, x, y int) int
	hitTest(p image.Point, x, y int) (source Source, bounds image.Rectangle, ok bool, nextX int)
}

type TextBox struct {
	Text  string
	Face  font.Face
	Color color.Color

	// StrikeThickness is the strikethrough line's thickness in pixels; 0
	// means no strikethrough.
	StrikeThickness int

	// source is the InlineText this TextBox was built from - see Source.
	source Inline

	boundsComputed bool
	bounds         image.Rectangle
	advance        int

	spaceComputed bool
	spaceWidth    int
}

var _ InlineBox = (*TextBox)(nil)

func (b *TextBox) Source() Source {
	return b.source
}

func (b *TextBox) hitTest(p image.Point, x, y int) (Source, image.Rectangle, bool, int) {
	bounds, advance := b.BoundsAndAdvance()
	shifted := bounds.Add(image.Pt(x, y))
	if p.In(shifted) {
		return b.source, shifted, true, x + advance
	}
	return nil, image.Rectangle{}, false, x + advance
}

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

// hitTest mirrors DrawInline exactly: the marker is drawn hanging off to
// the left of x (DrawInline's x-advance-space), not at x itself, so a
// naive check against BoundsAndAdvance's own (zero-width) bounds would
// never match a click on the visible marker glyph.
func (b *ListItemMarkerBox) hitTest(p image.Point, x, y int) (Source, image.Rectangle, bool, int) {
	_, advance := b.Marker.BoundsAndAdvance()
	space := b.Marker.SpaceWidth()
	if source, bounds, ok, _ := b.Marker.hitTest(p, x-advance-space, y); ok {
		return source, bounds, true, x - space
	}
	return nil, image.Rectangle{}, false, x - space
}

type ImageBox struct {
	src    string
	bounds image.Rectangle

	// source is the InlineImage this ImageBox was built from - see Source.
	source Inline
}

var _ InlineBox = (*ImageBox)(nil)

func (b *ImageBox) Source() Source {
	return b.source
}

func (b *ImageBox) BoundsAndAdvance() (image.Rectangle, int) {
	return b.bounds, b.bounds.Dx()
}

func (b *ImageBox) SpaceWidth() int {
	return 0
}

func (b *ImageBox) hitTest(p image.Point, x, y int) (Source, image.Rectangle, bool, int) {
	shifted := b.bounds.Add(image.Pt(x, y))
	if p.In(shifted) {
		return b.source, shifted, true, x + b.bounds.Dx()
	}
	return nil, image.Rectangle{}, false, x + b.bounds.Dx()
}

type LineBox struct {
	parts []InlineBox

	boundsComputed bool
	bounds         image.Rectangle
	advance        int
}

var _ Box = (*LineBox)(nil)

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

// Bounds mirrors the same left-edge convention hitTest/drawContents use:
// a positive bounds.Min.X (e.g. a code line's leading indentation, which
// has real advance but no ink) is real, occupied space starting at this
// box's own x=0 - not excess to crop away - so only a *negative* Min.X
// (an overshooting left bearing) gets compensated for, the same
// adjustment hitTest/drawContents themselves apply. Cropping a positive
// Min.X here (as a plain bounds.Sub(bounds.Min) would) under-reports an
// indented line's width, which made StackBox.hitTest's containment
// pre-check (child.Bounds()) reject real hits on the right-hand side of
// indented code lines.
func (b *LineBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	left := bounds.Min.X
	if left > 0 {
		left = 0
	}
	return image.Rect(0, 0, bounds.Max.X-left, bounds.Dy())
}

// hitTest mirrors drawContents' own accumulation loop exactly, calling
// each part's hitTest instead of DrawInline at each step - deliberately
// structured as the same walk, rather than a separately-derived one, so
// the two can't silently drift apart. p needs no shift to match: y here
// is exactly the internal y drawContents computes for external y=0,
// chosen so a part's drawn footprint (measured from its own baseline)
// already lands directly in Bounds()'s normalized frame - the same
// frame p arrives in, and the same frame a part's own returned bounds
// come back in (no further shifting needed here, unlike other
// composites - InlineBox.hitTest already positions its result using the
// same accumulated (x, y) DrawInline would).
func (b *LineBox) hitTest(p image.Point) (Source, image.Rectangle, bool) {
	lineBounds, _ := b.BoundsAndAdvance()
	y := -lineBounds.Min.Y

	bounds, _ := b.parts[0].BoundsAndAdvance()
	x := 0
	if left := bounds.Min.X; left < 0 {
		x -= left
	}
	prevSpace := b.parts[0].SpaceWidth()

	source, partBounds, ok, next := b.parts[0].hitTest(p, x, y)
	if ok {
		return source, partBounds, true
	}
	x = next
	for _, part := range b.parts[1:] {
		space := part.SpaceWidth()
		source, partBounds, ok, next := part.hitTest(p, x+maxInt(prevSpace, space), y)
		if ok {
			return source, partBounds, true
		}
		x = next
		prevSpace = space
	}
	return nil, image.Rectangle{}, false
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

// hitTest walks slots from the start looking for the one p.Y falls in -
// a plain linear scan, unlike normalizeCursor's cursor-relative walk,
// since resolving this way would force-build every slot up to the match
// regardless. That's fine for a StackBox reached via nested structure
// (a list's items, a blockquote's paragraphs - typically small), but
// would defeat the point for the top-level, potentially huge document
// StackBox - View.BoxAt handles that one specially, starting from the
// current scroll cursor instead of slot 0.
func (b *StackBox) hitTest(p image.Point) (Source, image.Rectangle, bool) {
	y := 0
	for i := range b.slots {
		box := b.boxAt(i)
		h := box.Bounds().Dy()
		if p.Y < y+h {
			source, bounds, ok := hitTestChild(box, image.Pt(p.X, p.Y-y), nil, image.Rectangle{}, false)
			if !ok {
				return nil, image.Rectangle{}, false
			}
			return source, bounds.Add(image.Pt(0, y)), true
		}
		y += h
	}
	return nil, image.Rectangle{}, false
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

// hitTest always misses: an EmptyBox is pure spacing (a margin/gap
// between blocks or lines), never content.
func (b *EmptyBox) hitTest(p image.Point) (Source, image.Rectangle, bool) {
	return nil, image.Rectangle{}, false
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

// hitTest has no source of its own - a pure positioning wrapper (for
// indentation, or cell alignment in a table) - so a miss on inner (e.g.
// content narrower than the space it was given) reports no match rather
// than falling back to anything.
func (b *ContainerBox) hitTest(p image.Point) (Source, image.Rectangle, bool) {
	source, bounds, ok := hitTestChild(b.inner, p.Sub(b.innerPos), nil, image.Rectangle{}, false)
	if !ok {
		return nil, image.Rectangle{}, false
	}
	return source, bounds.Add(b.innerPos), true
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

	// source is the BlockquoteBlock this box was built from - see Source.
	source Block
}

var _ Box = (*BlockquoteBox)(nil)

func (b *BlockquoteBox) Source() Source {
	return b.source
}

func (b *BlockquoteBox) Bounds() image.Rectangle {
	return image.Rect(0, 0, b.width, b.inner.Bounds().Dy())
}

func (b *BlockquoteBox) drawContents(dst Canvas, x, y int) {
	dst.DrawRect(x, y, b.barWidth, b.inner.Bounds().Dy(), b.barColor)
	DrawBox(b.inner, dst, x+b.indent, y)
}

// hitTest treats the whole indent strip (bar plus any padding before
// inner starts) as the blockquote's own, and falls back to it too
// whenever inner declines to match (content narrower than the indented
// width, or a gap between quoted paragraphs) - every hitTest
// implementation degrades gracefully on an out-of-bounds point, so
// there's no need to bounds-check before delegating.
func (b *BlockquoteBox) hitTest(p image.Point) (Source, image.Rectangle, bool) {
	if p.X >= b.indent {
		if source, bounds, ok := b.inner.hitTest(image.Pt(p.X-b.indent, p.Y)); ok {
			return source, bounds.Add(image.Pt(b.indent, 0)), true
		}
	}
	return b.source, b.Bounds(), true
}

// TableBox draws a GFM table: a frame around the whole thing, a rule
// under the header row, a rule between each pair of columns, and each
// cell positioned at its resolved (column, row) offset. columnOffsets/
// rowOffsets have one more entry than there are columns/rows - the last
// entry is the table's own right/bottom edge, so Bounds() doesn't need
// separate width/height fields, and the header rule's position is just
// rowOffsets[1] (see TableBlock.GetBox for why that boundary is exactly
// where the rule belongs). Each column rule is centered in the columnGap
// between adjacent columns' content, at columnOffsets[c] - columnGap/2.
type TableBox struct {
	columnOffsets       []int
	rowOffsets          []int
	frameThickness      int
	columnGap           int
	columnRuleThickness int
	frameColor          color.Color
	cells               [][]Box

	// source is the TableBlock this box was built from - see Source.
	source Block
}

var _ Box = (*TableBox)(nil)

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
			DrawBox(b.cells[row][col], dst, x+b.columnOffsets[col], y+b.rowOffsets[row])
		}
	}
}

// hitTest finds which cell (row, col) p falls in via columnOffsets/
// rowOffsets, and falls back to the table's own source whenever there
// isn't one to recurse into (the frame, a gap between cells/rows) or the
// cell itself declines (its content is narrower/shorter than the cell's
// allotted space).
func (b *TableBox) hitTest(p image.Point) (Source, image.Rectangle, bool) {
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
		if source, bounds, ok := b.cells[row][col].hitTest(local); ok {
			return source, bounds.Add(image.Pt(b.columnOffsets[col], b.rowOffsets[row])), true
		}
	}
	return b.source, b.Bounds(), true
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

var _ Box = (*RuleBox)(nil)

func (b *RuleBox) Source() Source {
	return b.source
}

func (b *RuleBox) Bounds() image.Rectangle {
	return image.Rect(0, 0, b.width, b.thickness)
}

func (b *RuleBox) drawContents(dst Canvas, x, y int) {
	dst.DrawRect(x, y, b.width, b.thickness, b.color)
}

func (b *RuleBox) hitTest(p image.Point) (Source, image.Rectangle, bool) {
	return b.source, b.Bounds(), true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
