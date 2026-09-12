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

	source, bounds := b.hitTest(image.Pt(50, 1))
	if source == nil {
		t.Fatal("hitTest = nil source, want a match")
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
	if source, _ := b.hitTest(image.Pt(50, 5)); source != nil {
		t.Error("hitTest on EmptyBox = a match, want always a miss (pure spacing)")
	}
}

func TestContainerBoxHitTest(t *testing.T) {
	src := &sourceBlock{node: &ASTNode{Tag: TagThematicBreak}}
	inner := &RuleBox{width: 50, thickness: 2, source: src}
	b := NewContainerBox(inner, 100, 10, 20, 3) // inner offset by (20, 3)

	// Inside inner's shifted footprint.
	source, bounds := b.hitTest(image.Pt(30, 4))
	if source == nil || source.Node().Tag != TagThematicBreak {
		t.Fatalf("hit inside inner = (%v, _), want (ThematicBreak, _)", source)
	}
	if want := inner.Bounds().Add(image.Pt(20, 3)); bounds != want {
		t.Errorf("bounds = %v, want %v (inner's bounds shifted by innerPos)", bounds, want)
	}
	// Outside inner's footprint but within the container - pure padding,
	// no source of its own.
	if source, _ := b.hitTest(image.Pt(5, 4)); source != nil {
		t.Error("hit in the padding around inner = a match, want a miss")
	}
}

func TestBlockquoteBoxHitTest(t *testing.T) {
	quoteSrc := &sourceBlock{node: &ASTNode{Tag: TagBlockquote}}
	contentSrc := &sourceBlock{node: &ASTNode{Tag: TagParagraph}}
	content := &RuleBox{width: 80, thickness: 10, source: contentSrc}
	b := &BlockquoteBox{width: 100, indent: 16, barWidth: 3, inner: content, source: quoteSrc}

	if source, bounds := b.hitTest(image.Pt(1, 1)); source == nil || source.Node().Tag != TagBlockquote {
		t.Errorf("bar hit = (%v, %v), want (Blockquote, _)", source, bounds)
	} else if want := b.Bounds(); bounds != want {
		t.Errorf("bar hit bounds = %v, want %v (the whole quote)", bounds, want)
	}
	if source, bounds := b.hitTest(image.Pt(50, 1)); source == nil || source.Node().Tag != TagParagraph {
		t.Errorf("content hit = (%v, %v), want (Paragraph, _)", source, bounds)
	} else if want := content.Bounds().Add(image.Pt(16, 0)); bounds != want {
		t.Errorf("content hit bounds = %v, want %v (content's bounds shifted by indent)", bounds, want)
	}

	// A gap inside the content area (EmptyBox always declines) falls
	// back to the blockquote itself rather than reporting no match.
	empty := &BlockquoteBox{width: 100, indent: 16, barWidth: 3, inner: NewEmptyBox(80, 10), source: quoteSrc}
	if source, _ := empty.hitTest(image.Pt(50, 1)); source == nil || source.Node().Tag != TagBlockquote {
		t.Errorf("gap-in-content hit = %v, want fallback Blockquote", source)
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

	if source, bounds := b.hitTest(image.Pt(20, 10)); source == nil || source.Node().Tag != TagTableCell {
		t.Errorf("cell hit = (%v, %v), want (TableCell, _)", source, bounds)
	} else if want := cell.Bounds().Add(image.Pt(5, 5)); bounds != want {
		t.Errorf("cell hit bounds = %v, want %v (cell's bounds shifted by its offset)", bounds, want)
	}
	if source, bounds := b.hitTest(image.Pt(2, 10)); source == nil || source.Node().Tag != TagTable {
		t.Errorf("frame hit (before first column) = (%v, %v), want fallback (Table, _)", source, bounds)
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
	if source, _ := bEmpty.hitTest(image.Pt(20, 10)); source == nil || source.Node().Tag != TagTable {
		t.Errorf("declining-cell hit = %v, want fallback Table", source)
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
	source, gotBounds, next := b.hitTest(image.Pt(bounds.Min.X, bounds.Min.Y), 0, 0)
	if source == nil {
		t.Fatal("hitTest inside glyph bounds = nil source, want a match")
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

	if source, _, _ := b.hitTest(image.Pt(bounds.Min.X, bounds.Max.Y+100), 0, 0); source != nil {
		t.Error("hitTest far below the glyph = a match, want a miss")
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
	source, bounds, next := marker.hitTest(p, x, y)
	if source == nil {
		t.Fatal("hitTest at the marker's actual drawn position = nil source, want a match")
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

	if source, _, _ := marker.hitTest(image.Pt(x, y), x, y); source != nil {
		t.Error("hitTest at x itself (not the marker's real, offset position) = a match, want a miss")
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
	source, hitBounds := line.hitTest(p1)
	if source == nil || source.Node().Tag != TagEmphasis {
		t.Errorf("hit at left edge = %v, want Emphasis - the first word", source)
	}
	if source != nil && !p1.In(hitBounds) {
		t.Errorf("bounds %v for hit at %v don't contain the point", hitBounds, p1)
	}

	p2 := image.Pt(bounds.Max.X-1, midY)
	source, hitBounds = line.hitTest(p2)
	if source == nil || source.Node().Tag != TagStrong {
		t.Errorf("hit at right edge = %v, want Strong - the second word", source)
	}
	if source != nil && !p2.In(hitBounds) {
		t.Errorf("bounds %v for hit at %v don't contain the point", hitBounds, p2)
	}

	if source, _ := line.hitTest(image.Pt(bounds.Min.X, bounds.Max.Y+100)); source != nil {
		t.Error("hit far below the line = a match, want a miss")
	}
}

// TestLineBoxBoundsIndentedText checks that Bounds() doesn't crop away a
// line's leading indentation - see the comment on LineBox.Bounds.
func TestLineBoxBoundsIndentedText(t *testing.T) {
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72)}
	face, err := ctx.SelectFace(TextStyle{Size: 16})
	if err != nil {
		t.Fatal(err)
	}
	// Long enough that the gap can't be mistaken for rounding noise.
	text := "                                        return 42"
	line := &LineBox{parts: []InlineBox{&TextBox{Text: text, Face: face}}}

	raw, _ := line.BoundsAndAdvance()
	if got, want := line.Bounds().Dx(), raw.Max.X; got != want {
		t.Errorf("Bounds().Dx() = %d, want %d (raw.Max.X, not the ink-only span)", got, want)
	}
}

// TestStackBoxHitTestIndentedLine is a regression test: StackBox.hitTest's
// containment check used to reject hits on the right-hand side of an
// indented code line, because Bounds() under-reported its width - see
// the comment on LineBox.Bounds.
func TestStackBoxHitTestIndentedLine(t *testing.T) {
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72)}
	face, err := ctx.SelectFace(TextStyle{Size: 16})
	if err != nil {
		t.Fatal(err)
	}
	text := "                                        return 42"
	src := &InlineText{text: text, node: &ASTNode{Tag: TagCodeBlock}}
	line := &LineBox{parts: []InlineBox{&TextBox{Text: text, Face: face, source: src}}}
	stack := stackOf(line)

	// Derived from the raw bounds, not line.Bounds() itself - a point
	// derived from the value under test can't catch a wrong answer.
	raw, _ := line.BoundsAndAdvance()
	midY := line.Bounds().Dy() / 2
	p := image.Pt(raw.Max.X-1, midY)
	source, _ := stack.hitTest(p)
	if source == nil || source.Node().Tag != TagCodeBlock {
		t.Errorf("hit near the right edge of an indented line = %v, want CodeBlock", source)
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
	if source, _ := stack.hitTest(image.Pt(50, 5)); source != nil {
		t.Error("hit in leading gap = a match, want a miss")
	}
	// In the RuleBox slot - y in [10, 30).
	source, bounds := stack.hitTest(image.Pt(50, 15))
	if source == nil || source.Node().Tag != TagThematicBreak {
		t.Errorf("hit in middle slot = %v, want ThematicBreak", source)
	}
	if want := image.Rect(0, 0, 100, 20).Add(image.Pt(0, 10)); bounds != want {
		t.Errorf("bounds = %v, want %v (RuleBox's own bounds, shifted by the slot's y offset)", bounds, want)
	}
	// Past the end.
	if source, _ := stack.hitTest(image.Pt(50, 1000)); source != nil {
		t.Error("hit past the end = a match, want a miss")
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
