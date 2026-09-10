package whynot

import (
	"image"
	"testing"
)

// sourceBlock is a minimal Block for hitTest tests that just need a
// known Node() to trace back to - GetBox/Margins are never called by
// these tests.
type sourceBlock struct {
	node *ASTNode
}

func (b *sourceBlock) GetBox(ctx RenderingContext, width int) Box { panic("not implemented") }
func (b *sourceBlock) Margins(ctx RenderingContext) Margins       { panic("not implemented") }
func (b *sourceBlock) Node() *ASTNode                             { return b.node }

func TestRuleBoxHitTest(t *testing.T) {
	src := &sourceBlock{node: &ASTNode{Tag: TagThematicBreak}}
	b := &RuleBox{width: 100, thickness: 2, source: src}

	source, bounds, ok := b.hitTest(image.Pt(50, 1))
	if !ok {
		t.Fatal("hitTest = not ok, want a match")
	}
	if source.Node().Tag != TagThematicBreak {
		t.Errorf("Tag = %v, want TagThematicBreak", source.Node().Tag)
	}
	if want := b.Bounds(); bounds != want {
		t.Errorf("bounds = %v, want %v (the whole rule)", bounds, want)
	}
}

func TestEmptyBoxHitTest(t *testing.T) {
	b := NewEmptyBox(100, 10)
	if _, _, ok := b.hitTest(image.Pt(50, 5)); ok {
		t.Error("hitTest on EmptyBox = ok, want always a miss (pure spacing)")
	}
}

func TestContainerBoxHitTest(t *testing.T) {
	src := &sourceBlock{node: &ASTNode{Tag: TagThematicBreak}}
	inner := &RuleBox{width: 50, thickness: 2, source: src}
	b := NewContainerBox(inner, 100, 10, 20, 3) // inner offset by (20, 3)

	// Inside inner's shifted footprint.
	source, bounds, ok := b.hitTest(image.Pt(30, 4))
	if !ok || source.Node().Tag != TagThematicBreak {
		t.Fatalf("hit inside inner = (%v, %v), want (ThematicBreak, true)", source, ok)
	}
	if want := inner.Bounds().Add(image.Pt(20, 3)); bounds != want {
		t.Errorf("bounds = %v, want %v (inner's bounds shifted by innerPos)", bounds, want)
	}
	// Outside inner's footprint but within the container - pure padding,
	// no source of its own.
	if _, _, ok := b.hitTest(image.Pt(5, 4)); ok {
		t.Error("hit in the padding around inner = ok, want a miss")
	}
}

func TestBlockquoteBoxHitTest(t *testing.T) {
	quoteSrc := &sourceBlock{node: &ASTNode{Tag: TagBlockquote}}
	contentSrc := &sourceBlock{node: &ASTNode{Tag: TagParagraph}}
	content := &RuleBox{width: 80, thickness: 10, source: contentSrc}
	b := &BlockquoteBox{width: 100, indent: 16, barWidth: 3, inner: content, source: quoteSrc}

	if source, bounds, ok := b.hitTest(image.Pt(1, 1)); !ok || source.Node().Tag != TagBlockquote {
		t.Errorf("bar hit = (%v, %v, %v), want (Blockquote, _, true)", source, bounds, ok)
	} else if want := b.Bounds(); bounds != want {
		t.Errorf("bar hit bounds = %v, want %v (the whole quote)", bounds, want)
	}
	if source, bounds, ok := b.hitTest(image.Pt(50, 1)); !ok || source.Node().Tag != TagParagraph {
		t.Errorf("content hit = (%v, %v, %v), want (Paragraph, _, true)", source, bounds, ok)
	} else if want := content.Bounds().Add(image.Pt(16, 0)); bounds != want {
		t.Errorf("content hit bounds = %v, want %v (content's bounds shifted by indent)", bounds, want)
	}

	// A gap inside the content area (EmptyBox always declines) falls
	// back to the blockquote itself rather than reporting no match.
	empty := &BlockquoteBox{width: 100, indent: 16, barWidth: 3, inner: NewEmptyBox(80, 10), source: quoteSrc}
	if source, _, ok := empty.hitTest(image.Pt(50, 1)); !ok || source.Node().Tag != TagBlockquote {
		t.Errorf("gap-in-content hit = (%v, %v), want fallback (Blockquote, true)", source, ok)
	}
}

func TestTableBoxHitTest(t *testing.T) {
	tableSrc := &sourceBlock{node: &ASTNode{Tag: TagTable}}
	cellSrc := &sourceBlock{node: &ASTNode{Tag: TagTableCell}}
	cell := &RuleBox{width: 30, thickness: 10, source: cellSrc}

	// One column [5, 45), one row [5, 25) - everything outside that
	// range (but still within the table's own overall bounds) is frame/
	// edge gap, not a second column with no cell.
	b := &TableBox{
		columnOffsets: []int{5, 45},
		rowOffsets:    []int{5, 25},
		cells:         [][]Box{{cell}},
		source:        tableSrc,
	}

	if source, bounds, ok := b.hitTest(image.Pt(20, 10)); !ok || source.Node().Tag != TagTableCell {
		t.Errorf("cell hit = (%v, %v, %v), want (TableCell, _, true)", source, bounds, ok)
	} else if want := cell.Bounds().Add(image.Pt(5, 5)); bounds != want {
		t.Errorf("cell hit bounds = %v, want %v (cell's bounds shifted by its offset)", bounds, want)
	}
	if source, bounds, ok := b.hitTest(image.Pt(2, 10)); !ok || source.Node().Tag != TagTable {
		t.Errorf("frame hit (before first column) = (%v, %v, %v), want fallback (Table, _, true)", source, bounds, ok)
	} else if want := b.Bounds(); bounds != want {
		t.Errorf("frame hit bounds = %v, want %v (the whole table)", bounds, want)
	}

	// A cell that declines (EmptyBox) falls back to the table itself.
	bEmpty := &TableBox{
		columnOffsets: []int{5, 45},
		rowOffsets:    []int{5, 25},
		cells:         [][]Box{{NewEmptyBox(40, 20)}},
		source:        tableSrc,
	}
	if source, _, ok := bEmpty.hitTest(image.Pt(20, 10)); !ok || source.Node().Tag != TagTable {
		t.Errorf("declining-cell hit = (%v, %v), want fallback (Table, true)", source, ok)
	}
}

func TestTextBoxHitTest(t *testing.T) {
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72)}
	face, err := ctx.SelectFace(TextStyle{Size: 16})
	if err != nil {
		t.Fatal(err)
	}
	src := &InlineText{text: "hi", node: &ASTNode{Tag: TagParagraph}}
	b := &TextBox{Text: "hi", Face: face, source: src}

	bounds, advance := b.BoundsAndAdvance()
	source, gotBounds, ok, next := b.hitTest(image.Pt(bounds.Min.X, bounds.Min.Y), 0, 0)
	if !ok {
		t.Fatal("hitTest inside glyph bounds = not ok, want a match")
	}
	if source.Node().Tag != TagParagraph {
		t.Errorf("Tag = %v, want TagParagraph", source.Node().Tag)
	}
	if gotBounds != bounds {
		t.Errorf("bounds = %v, want %v (the glyph's own bounds, at x=y=0)", gotBounds, bounds)
	}
	if next != advance {
		t.Errorf("next = %d, want %d (the advance)", next, advance)
	}

	if _, _, ok, _ := b.hitTest(image.Pt(bounds.Min.X, bounds.Max.Y+100), 0, 0); ok {
		t.Error("hitTest far below the glyph = ok, want a miss")
	}
}

// TestListItemMarkerBoxHitTest checks the negative-offset math directly:
// DrawInline draws the marker at x-advance-space, not x, so hitTest has
// to check the same actual position - a point at the "naive" x should
// not match.
func TestListItemMarkerBoxHitTest(t *testing.T) {
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72)}
	face, err := ctx.SelectFace(TextStyle{Size: 16})
	if err != nil {
		t.Fatal(err)
	}
	markerSrc := &InlineText{text: "•", node: &ASTNode{Tag: TagListItem}}
	markerBox := &TextBox{Text: markerSrc.text, Face: face, source: markerSrc}
	marker := &ListItemMarkerBox{Marker: markerBox}

	markerBounds, advance := markerBox.BoundsAndAdvance()
	space := markerBox.SpaceWidth()
	const x, y = 100, 0
	markerX := x - advance - space

	p := image.Pt(markerX+markerBounds.Min.X, y+markerBounds.Min.Y)
	source, bounds, ok, next := marker.hitTest(p, x, y)
	if !ok {
		t.Fatal("hitTest at the marker's actual drawn position = not ok, want a match")
	}
	if source.Node().Tag != TagListItem {
		t.Errorf("Tag = %v, want TagListItem", source.Node().Tag)
	}
	if want := markerBounds.Add(image.Pt(markerX, y)); bounds != want {
		t.Errorf("bounds = %v, want %v (marker's own bounds at its real, offset position)", bounds, want)
	}
	if next != x-space {
		t.Errorf("next = %d, want %d (x - space, matching DrawInline's own return)", next, x-space)
	}

	if _, _, ok, _ := marker.hitTest(image.Pt(x, y), x, y); ok {
		t.Error("hitTest at x itself (not the marker's real, offset position) = ok, want a miss")
	}
}

// TestLineBoxHitTest checks that x-accumulation across multiple parts
// attributes a click to the right one - using the leftmost/rightmost
// content as landmarks rather than hardcoded pixel offsets, so this
// doesn't depend on exact font metrics.
func TestLineBoxHitTest(t *testing.T) {
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72)}
	face, err := ctx.SelectFace(TextStyle{Size: 16})
	if err != nil {
		t.Fatal(err)
	}
	src1 := &InlineText{text: "aa", node: &ASTNode{Tag: TagEmphasis}}
	src2 := &InlineText{text: "bb", node: &ASTNode{Tag: TagStrong}}
	b1 := &TextBox{Text: "aa", Face: face, source: src1}
	b2 := &TextBox{Text: "bb", Face: face, source: src2}
	line := &LineBox{parts: []InlineBox{b1, b2}}

	bounds := line.Bounds()
	midY := (bounds.Min.Y + bounds.Max.Y) / 2

	p1 := image.Pt(bounds.Min.X, midY)
	source, hitBounds, ok := line.hitTest(p1)
	if !ok || source.Node().Tag != TagEmphasis {
		t.Errorf("hit at left edge = (%v, %v), want (Emphasis, true) - the first word", source, ok)
	}
	if ok && !p1.In(hitBounds) {
		t.Errorf("bounds %v for hit at %v don't contain the point", hitBounds, p1)
	}

	p2 := image.Pt(bounds.Max.X-1, midY)
	source, hitBounds, ok = line.hitTest(p2)
	if !ok || source.Node().Tag != TagStrong {
		t.Errorf("hit at right edge = (%v, %v), want (Strong, true) - the second word", source, ok)
	}
	if ok && !p2.In(hitBounds) {
		t.Errorf("bounds %v for hit at %v don't contain the point", hitBounds, p2)
	}

	if _, _, ok := line.hitTest(image.Pt(bounds.Min.X, bounds.Max.Y+100)); ok {
		t.Error("hit far below the line = ok, want a miss")
	}
}

// TestLineBoxBoundsIndentedText checks that Bounds() doesn't crop away a
// line's leading indentation - a code line's leading spaces have real
// advance but no ink, so BoundsAndAdvance's own bounds.Min.X lands well
// to the right of 0 (unlike ordinary text, where it's ~0 or slightly
// negative). Bounds() must not treat that gap as excess to trim: doing
// so under-reports the line's width, which broke hit-testing through a
// parent StackBox (its containment pre-check rejected real hits on the
// right-hand side of indented code lines - see TestStackBoxHitTestIndentedLine).
func TestLineBoxBoundsIndentedText(t *testing.T) {
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72)}
	face, err := ctx.SelectFace(TextStyle{Size: 16})
	if err != nil {
		t.Fatal(err)
	}
	// A long indent, so the gap this is checking for (~40 space-widths)
	// can't be mistaken for font-metric rounding noise.
	text := "                                        return 42"
	line := &LineBox{parts: []InlineBox{&TextBox{Text: text, Face: face}}}

	raw, _ := line.BoundsAndAdvance()
	// want is bounds.Max.X itself (not bounds.Dx()): since the leading
	// indentation has real advance but no ink, bounds.Min.X lands well
	// past 0, and this line's real x=0 is its own left edge (matching
	// hitTest/drawContents), not the first inked pixel - Max.X already
	// measures from that real origin.
	if got, want := line.Bounds().Dx(), raw.Max.X; got != want {
		t.Errorf("Bounds().Dx() = %d, want %d (bounds.Max.X, the rightmost ink pixel measured from this line's own real left edge - not bounds.Dx(), which only spans the ink and ignores the leading indentation gap)", got, want)
	}
}

// TestStackBoxHitTestIndentedLine is a regression test for a real bug:
// StackBox.hitTest's containment pre-check (hitTestChild, via
// child.Bounds()) rejected hits on the right-hand side of an indented
// code line, because Bounds() used to crop away the line's leading
// indentation gap - under-reporting its width and making a real hit on
// visible text look like it fell outside the line's bounds entirely.
func TestStackBoxHitTestIndentedLine(t *testing.T) {
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72)}
	face, err := ctx.SelectFace(TextStyle{Size: 16})
	if err != nil {
		t.Fatal(err)
	}
	// A long indent: with only a few spaces, the point this test probes
	// can land inside the ink-only span Bounds() used to wrongly report,
	// passing even with the bug present - it has to fall in the part of
	// the line only the leading-indentation gap accounts for.
	text := "                                        return 42"
	src := &InlineText{text: text, node: &ASTNode{Tag: TagCodeBlock}}
	line := &LineBox{parts: []InlineBox{&TextBox{Text: text, Face: face, source: src}}}
	stack := stackOf(line)

	// The point to probe is derived from the raw (unnormalized) bounds,
	// not from line.Bounds() itself - a test point derived from the
	// value under test can't tell a wrong answer from a right one.
	raw, _ := line.BoundsAndAdvance()
	midY := line.Bounds().Dy() / 2
	p := image.Pt(raw.Max.X-1, midY)
	source, _, ok := stack.hitTest(p)
	if !ok || source.Node().Tag != TagCodeBlock {
		t.Errorf("hit near the right edge of an indented line, via the parent StackBox, = (%v, %v), want (CodeBlock, true)", source, ok)
	}
}

func TestStackBoxHitTest(t *testing.T) {
	src := &sourceBlock{node: &ASTNode{Tag: TagThematicBreak}}
	stack := stackOf(
		NewEmptyBox(100, 10),
		&RuleBox{width: 100, thickness: 20, source: src},
		NewEmptyBox(100, 30),
	)

	// In the leading gap.
	if _, _, ok := stack.hitTest(image.Pt(50, 5)); ok {
		t.Error("hit in leading gap = ok, want a miss")
	}
	// In the RuleBox slot - y in [10, 30).
	source, bounds, ok := stack.hitTest(image.Pt(50, 15))
	if !ok || source.Node().Tag != TagThematicBreak {
		t.Errorf("hit in middle slot = (%v, %v), want (ThematicBreak, true)", source, ok)
	}
	if want := image.Rect(0, 0, 100, 20).Add(image.Pt(0, 10)); bounds != want {
		t.Errorf("bounds = %v, want %v (RuleBox's own bounds, shifted by the slot's y offset)", bounds, want)
	}
	// Past the end.
	if _, _, ok := stack.hitTest(image.Pt(50, 1000)); ok {
		t.Error("hit past the end = ok, want a miss")
	}
}

// stackOf builds a StackBox from already-built boxes, as pre-resolved
// slots - for tests that just want a StackBox with known children and
// don't need to exercise lazy building via Block.
func stackOf(boxes ...Box) *StackBox {
	return &StackBox{slots: preResolvedSlots(boxes)}
}

func TestStackBoxNormalizeCursor(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	cases := []struct {
		c    stackCursor
		want stackCursor
	}{
		{c: stackCursor{0, 0}, want: stackCursor{0, 0}},
		{c: stackCursor{0, 9}, want: stackCursor{0, 9}},
		{c: stackCursor{0, 10}, want: stackCursor{1, 0}},
		{c: stackCursor{1, 5}, want: stackCursor{1, 5}},
		{c: stackCursor{1, -7}, want: stackCursor{0, 3}},
		{c: stackCursor{2, -35}, want: stackCursor{0, 0}},
		{c: stackCursor{0, 17}, want: stackCursor{1, 7}},
		{c: stackCursor{1, 20}, want: stackCursor{2, 0}},
		{c: stackCursor{0, 60}, want: stackCursor{2, 30}},
		{c: stackCursor{0, 1000}, want: stackCursor{2, 30}},
		{c: stackCursor{2, 30}, want: stackCursor{2, 30}},
	}

	for _, tc := range cases {
		got := stack.normalizeCursor(tc.c)
		if got != tc.want {
			t.Errorf("normalizeCursor(%+v) = %+v, want %+v", tc.c, got, tc.want)
		}
	}
}

func TestStackBoxNormalizeCursorEmpty(t *testing.T) {
	stack := &StackBox{}
	got := stack.normalizeCursor(stackCursor{0, 5})
	if got != (stackCursor{0, 0}) {
		t.Errorf("normalizeCursor on an empty StackBox = %+v, want {0, 0}", got)
	}
}

// TestStackBoxNormalizeCursorIdempotent checks that normalizeCursor leaves an
// already-canonical cursor - one already satisfying
// 0 <= offset < height(index) - unchanged.
func TestStackBoxNormalizeCursorIdempotent(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	for index := 0; index < 3; index++ {
		h := stack.boxAt(index).Bounds().Dy()
		for offset := 0; offset < h; offset++ {
			c := stackCursor{index: index, offset: float64(offset)}
			got := stack.normalizeCursor(c)
			if got != c {
				t.Errorf("normalizeCursor(%+v) (already canonical) = %+v, want unchanged", c, got)
			}
		}
	}
}

// TestStackBoxMoveCursor checks that moveCursor shifts by dy and
// normalizes the result, matching what an equivalent normalizeCursor call
// would give.
func TestStackBoxMoveCursor(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	cases := []struct {
		c    stackCursor
		dy   float64
		want stackCursor
	}{
		{c: stackCursor{0, 5}, dy: 10, want: stackCursor{1, 5}},
		{c: stackCursor{1, 5}, dy: -12, want: stackCursor{0, 3}},
		{c: stackCursor{2, 30}, dy: 100, want: stackCursor{2, 30}},
	}

	for _, tc := range cases {
		got := stack.moveCursor(tc.c, tc.dy)
		if got != tc.want {
			t.Errorf("moveCursor(%+v, %v) = %+v, want %+v", tc.c, tc.dy, got, tc.want)
		}
	}
}
