package whynot

import (
	"image"
	"os"
	"testing"
)

// fixedHeightBlock always lays out to a fixed height regardless of width,
// for tests that want exact, predictable heights without depending on
// real font metrics.
type fixedHeightBlock struct {
	height int
}

func (b *fixedHeightBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	return image.Rect(0, 0, width, b.height)
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

func (b *scaledHeightBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	return image.Rect(0, 0, width, int(float64(width)*b.scale))
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

// TestViewScroll checks Scroll's sign convention against what the
// previous offsetY-based implementation did (offsetY += dy, content drawn
// at localY + offsetY): negative dy moves the anchor forward through the
// document (later content becomes visible, i.e. "scrolling down");
// positive moves back toward the start.
func TestViewScroll(t *testing.T) {
	v := newTestView(
		&fixedHeightBlock{height: 10},
		&fixedHeightBlock{height: 20},
		&fixedHeightBlock{height: 30},
	)
	v.Layout(100, 1)

	if v.index != 0 || v.offset != 0 {
		t.Fatalf("initial position = (%d, %d), want (0, 0)", v.index, v.offset)
	}

	v.Scroll(-15)
	if v.index != 1 || v.offset != 5 {
		t.Errorf("after Scroll(-15) = (%d, %d), want (1, 5)", v.index, v.offset)
	}

	v.Scroll(15)
	if v.index != 0 || v.offset != 0 {
		t.Errorf("after Scroll(15) = (%d, %d), want (0, 0)", v.index, v.offset)
	}

	// Scrolling further than the document is long clamps to the end
	// rather than going out of range.
	v.Scroll(-1000)
	if v.index != 2 || v.offset != 30 {
		t.Errorf("after Scroll(-1000) = (%d, %d), want (2, 30) (clamped to the end)", v.index, v.offset)
	}
}

// TestViewScrollSubPixel checks that repeated fractional Scroll deltas
// accumulate correctly instead of being rounded away every call - two
// calls of -7.5 should move the anchor by 15, the same as one call of
// -15, not by 14 (2 * int(-7.5) truncated each time).
func TestViewScrollSubPixel(t *testing.T) {
	v := newTestView(
		&fixedHeightBlock{height: 10},
		&fixedHeightBlock{height: 20},
		&fixedHeightBlock{height: 30},
	)
	v.Layout(100, 1)

	v.Scroll(-7.5)
	v.Scroll(-7.5)
	if v.index != 1 || v.offset != 5 {
		t.Errorf("after Scroll(-7.5) twice = (%d, %d), want (1, 5)", v.index, v.offset)
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
	v.index, v.offset = 1, 100

	v.Layout(50, 1) // heights become [50, 100]; same ratio should give offset 50

	if v.index != 1 {
		t.Fatalf("index after resize = %d, want 1", v.index)
	}
	if v.offset != 50 {
		t.Errorf("offset after resize = %d, want 50 (50%% of the new height 100)", v.offset)
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
		if stack, ok := v.box.(*StackBox); ok && len(stack.slots) > 0 {
			v.index = len(stack.slots) - 1
			v.offset = 0
		}
		b.StartTimer()

		v.Layout(width+1, 1)
	}
}
