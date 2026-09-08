package whynot

import (
	"image"
	"image/color"
	"os"
	"testing"

	"golang.org/x/image/font"
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

func (b *fixedHeightBlock) Margins(ctx RenderingContext) Margins {
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

func (b *scaledHeightBlock) Margins(ctx RenderingContext) Margins {
	return Margins{}
}

func newTestView(blocks ...Block) *View {
	return &View{
		block: &StackBlock{blocks: blocks},
		ctx:   RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: NewDarkStyleSheet()},
	}
}

// drawnRect records one Canvas.DrawRect call.
type drawnRect struct {
	x, y, w, h int
	color      color.Color
}

// recordingCanvas is a minimal Canvas fake that only records DrawRect
// calls - enough to check View.Draw's background fill without a real
// rendering backend.
type recordingCanvas struct {
	bounds image.Rectangle
	rects  []drawnRect
}

var _ Canvas = (*recordingCanvas)(nil)

func (c *recordingCanvas) Bounds() image.Rectangle                                      { return c.bounds }
func (c *recordingCanvas) DrawText(s string, face font.Face, x, y int, clr color.Color) {}
func (c *recordingCanvas) DrawImage(src string, x, y int)                               {}
func (c *recordingCanvas) DrawRect(x, y, w, h int, clr color.Color) {
	c.rects = append(c.rects, drawnRect{x, y, w, h, clr})
}

// TestViewDrawFillsBackground checks that Draw fills dst's whole bounds
// with the StyleSheet's Background color before drawing content - offset
// bounds (not starting at 0,0) to make sure Min.X/Min.Y are actually used,
// not assumed zero.
func TestViewDrawFillsBackground(t *testing.T) {
	v := newTestView(&fixedHeightBlock{height: 10})
	v.Layout(100, 1)

	dst := &recordingCanvas{bounds: image.Rect(5, 10, 105, 60)}
	v.Draw(dst, 0, 0)

	if len(dst.rects) == 0 {
		t.Fatal("Draw issued no DrawRect calls, want at least a background fill")
	}
	got := dst.rects[0]
	want := drawnRect{5, 10, 100, 50, v.ctx.StyleSheet.Background()}
	if got != want {
		t.Errorf("background fill = %+v, want %+v", got, want)
	}
}

// TestViewDrawFillsBackgroundBeforeLayout checks that the background fill
// doesn't depend on Layout having been called yet - Draw shouldn't panic
// or skip the fill just because v.box is still nil.
func TestViewDrawFillsBackgroundBeforeLayout(t *testing.T) {
	v := newTestView(&fixedHeightBlock{height: 10})

	dst := &recordingCanvas{bounds: image.Rect(0, 0, 100, 50)}
	v.Draw(dst, 0, 0)

	if len(dst.rects) != 1 {
		t.Fatalf("Draw issued %d DrawRect calls, want exactly 1 (the background fill)", len(dst.rects))
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
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: NewDarkStyleSheet()}
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
