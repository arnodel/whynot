package whynot

import (
	"os"
	"testing"
)

// fixedHeightBlock always lays out to a fixed height regardless of width,
// for tests that want exact, predictable heights without depending on
// real font metrics.
type fixedHeightBlock struct {
	height int
}

func (b *fixedHeightBlock) GetBox(ctx RenderingContext, width int) Box {
	return NewEmptyBox(width, b.height)
}

func (b *fixedHeightBlock) Margins() Margins {
	return Margins{}
}

// scaledHeightBlock lays out to a height of width*scale, so a resize
// genuinely changes its height - unlike fixedHeightBlock, which is for
// tests that exercise Layout's ratio-based re-anchoring.
type scaledHeightBlock struct {
	scale float64
}

func (b *scaledHeightBlock) GetBox(ctx RenderingContext, width int) Box {
	return NewEmptyBox(width, int(float64(width)*b.scale))
}

func (b *scaledHeightBlock) Margins() Margins {
	return Margins{}
}

func newTestView(blocks ...Block) *View {
	return &View{
		block: &StackBlock{blocks: blocks},
		ctx:   RenderingContext{FaceSelector: NewGoFontFaceSelector(72)},
	}
}

// TestViewScroll checks Scroll's sign convention: negative dy moves the
// cursor forward through the document (later content becomes visible,
// i.e. "scrolling down"); positive moves back toward the start.
func TestViewScroll(t *testing.T) {
	v := newTestView(
		&fixedHeightBlock{height: 10},
		&fixedHeightBlock{height: 20},
		&fixedHeightBlock{height: 30},
	)
	v.Layout(100, 1)

	if v.cursor != (stackCursor{0, 0}) {
		t.Fatalf("initial position = %+v, want {0, 0}", v.cursor)
	}

	v.Scroll(-15)
	if v.cursor != (stackCursor{1, 5}) {
		t.Errorf("after Scroll(-15) = %+v, want {1, 5}", v.cursor)
	}

	v.Scroll(15)
	if v.cursor != (stackCursor{0, 0}) {
		t.Errorf("after Scroll(15) = %+v, want {0, 0}", v.cursor)
	}

	// Scrolling further than the document is long clamps to the end
	// rather than going out of range.
	v.Scroll(-1000)
	if v.cursor != (stackCursor{2, 30}) {
		t.Errorf("after Scroll(-1000) = %+v, want {2, 30} (clamped to the end)", v.cursor)
	}
}

// TestViewScrollSubPixel checks that repeated fractional Scroll deltas
// accumulate correctly instead of being rounded away every call - two
// calls of -7.5 should move the cursor by 15, the same as one call of -15,
// not by 14 (2 * int(-7.5) truncated each time) - now that offset is
// float64, there's no separate accumulator to get this right or wrong,
// but it's worth still checking directly.
func TestViewScrollSubPixel(t *testing.T) {
	v := newTestView(
		&fixedHeightBlock{height: 10},
		&fixedHeightBlock{height: 20},
		&fixedHeightBlock{height: 30},
	)
	v.Layout(100, 1)

	v.Scroll(-7.5)
	v.Scroll(-7.5)
	if v.cursor != (stackCursor{1, 5}) {
		t.Errorf("after Scroll(-7.5) twice = %+v, want {1, 5}", v.cursor)
	}
}

// TestViewLayoutReanchor checks that a rebuild re-derives the scroll
// position from the same (index, ratio-through-that-slot) rather than the
// same raw pixel offset, using blocks whose height actually changes with
// width.
func TestViewLayoutReanchor(t *testing.T) {
	v := newTestView(
		&scaledHeightBlock{scale: 1}, // height == width
		&scaledHeightBlock{scale: 2}, // height == 2*width
	)
	v.Layout(100, 1) // heights: [100, 200]

	// Anchor halfway through the second block.
	v.cursor = stackCursor{index: 1, offset: 100}

	v.Layout(50, 1) // heights become [50, 100]; same ratio should give offset 50

	if v.cursor != (stackCursor{1, 50}) {
		t.Errorf("cursor after resize = %+v, want {1, 50} (50%% of the new height 100)", v.cursor)
	}
}

// BenchmarkViewLayoutResizeDeep measures a resize while already anchored
// at the very last slot of a large document - the scenario that motivated
// making StackBlock.GetBox lazy in the first place. It positions the
// anchor directly rather than via Scroll, since walking there via Scroll
// would itself resolve everything in between - not the cost this
// benchmark is isolating.
func BenchmarkViewLayoutResizeDeep(b *testing.B) {
	source, err := os.ReadFile("testdata/test-large.md")
	if err != nil {
		b.Fatal(err)
	}
	block := Parse(source)
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72)}
	const width = 1024

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		v := &View{block: block, ctx: ctx}
		v.Layout(width, 1)
		if len(v.box.slots) > 0 {
			v.cursor.index = len(v.box.slots) - 1
			v.cursor.offset = 0
		}
		b.StartTimer()

		v.Layout(width+1, 1)
	}
}
