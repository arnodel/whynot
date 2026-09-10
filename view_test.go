package whynot

import (
	"image"
	"image/color"
	"math"
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

func (b *fixedHeightBlock) Node() *ASTNode {
	return nil
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

func (b *scaledHeightBlock) Node() *ASTNode {
	return nil
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
	want := drawnRect{5, 10, 100, 50, v.ctx.StyleSheet.BackgroundColor()}
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

// TestNewViewWithStyleSheet checks that WithStyleSheet overrides NewView's
// default StyleSheet (NewDarkStyleSheet).
func TestNewViewWithStyleSheet(t *testing.T) {
	custom := NewDarkStyleSheet()
	v := NewView([]byte("hello"), NewGoFontFaceSelector(72), WithStyleSheet(custom))
	if v.ctx.StyleSheet != StyleSheet(custom) {
		t.Errorf("StyleSheet = %v, want the instance passed via WithStyleSheet", v.ctx.StyleSheet)
	}
}

// TestViewSetStyleSheetRebuildsImmediately checks that a StyleSheet swap
// is visible right away, without waiting for another Layout call -
// GetBox/GetInlineBox bake in resolved style values (colors, margins
// collapsed to gaps, font sizes, ...) when the layout tree is built, so
// without an explicit rebuild inside SetStyleSheet nothing would change
// until something else happened to trigger a resize.
func TestViewSetStyleSheetRebuildsImmediately(t *testing.T) {
	block := Parse([]byte("hello world"))
	small := NewDarkStyleSheet()
	small.ParagraphTextStyle.Size = 10
	big := NewDarkStyleSheet()
	big.ParagraphTextStyle.Size = 40

	v := &View{block: block, ctx: RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: small}}
	v.Layout(200, 1)
	smallHeight := v.box.Bounds().Dy()

	v.SetStyleSheet(big)
	bigHeight := v.box.Bounds().Dy()

	if bigHeight <= smallHeight {
		t.Fatalf("height after SetStyleSheet = %d, want > %d (a 40pt paragraph should be taller than a 10pt one)", bigHeight, smallHeight)
	}
}

// TestViewSetStyleSheetBeforeLayout checks that calling SetStyleSheet
// before Layout has ever run doesn't panic (and doesn't waste a rebuild
// at a meaningless zero width) - the eventual first Layout call picks up
// the StyleSheet on its own.
func TestViewSetStyleSheetBeforeLayout(t *testing.T) {
	custom := NewLightStyleSheet()
	v := newTestView(&fixedHeightBlock{height: 10})

	v.SetStyleSheet(custom)
	if v.box != nil {
		t.Fatalf("box built before Layout was ever called")
	}

	v.Layout(100, 1)
	if v.ctx.StyleSheet != StyleSheet(custom) {
		t.Errorf("StyleSheet after the first Layout = %v, want the instance passed to SetStyleSheet", v.ctx.StyleSheet)
	}
}

// TestViewSetStyleSheetReanchorsScroll checks that a StyleSheet swap
// re-anchors the scroll position by ratio through the current slot, the
// same way a resize does - a new StyleSheet can change content heights
// (a different font size here) just as reflowing at a new width can, so
// the raw pixel offset alone would point at different content.
func TestViewSetStyleSheetReanchorsScroll(t *testing.T) {
	block := Parse([]byte("first\n\nsecond"))
	small := NewDarkStyleSheet()
	small.ParagraphTextStyle.Size = 10
	big := NewDarkStyleSheet()
	big.ParagraphTextStyle.Size = 40

	v := &View{block: block, ctx: RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: small}}
	v.Layout(200, 1)

	// Slot 1 is the margin gap StackBlock.GetBox inserts between the two
	// paragraphs, not content - the second paragraph is slot 2. Anchor
	// halfway through it.
	if len(v.box.slots) != 3 {
		t.Fatalf("got %d slots, want 3 (paragraph, gap, paragraph): %#v", len(v.box.slots), v.box.slots)
	}
	oldHeight := v.box.boxAt(2).Bounds().Dy()
	v.cursor = stackCursor{index: 2, offset: float64(oldHeight) / 2}

	v.SetStyleSheet(big)

	newHeight := v.box.boxAt(2).Bounds().Dy()
	if newHeight <= oldHeight {
		t.Fatalf("height at slot 2 after SetStyleSheet = %d, want > %d", newHeight, oldHeight)
	}
	if v.cursor.index != 2 {
		t.Fatalf("cursor index after SetStyleSheet = %d, want 2", v.cursor.index)
	}
	if gotRatio := v.cursor.offset / float64(newHeight); math.Abs(gotRatio-0.5) > 0.02 {
		t.Errorf("cursor ratio through slot 2 after SetStyleSheet = %.3f, want ~0.5 (preserved through the rebuild)", gotRatio)
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

// TestViewBoxAtEndToEnd checks that BoxAt reaches every kind of content in
// a real document, resolving to the right ASTTag. Exact pixel positions
// aren't predictable across margins/gaps/nesting, so this scans a coarse
// grid over the whole rendered document and just checks that *some* point
// resolves to each expected tag - a topological check, not a geometric
// one.
func TestViewBoxAtEndToEnd(t *testing.T) {
	source := []byte(`# Heading

A paragraph with some text.

` + "```" + `
code line
` + "```" + `

- item one
- item two

> a quoted paragraph

| a | b |
| - | - |
| 1 | 2 |

---
`)

	v := NewView(source, NewGoFontFaceSelector(72))
	const width = 300
	v.Layout(width, 1)

	height := v.box.Bounds().Dy()
	found := map[ASTTag]bool{}
	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			source, _, ok := v.BoxAt(x, y)
			if !ok {
				continue
			}
			found[source.Node().Tag] = true
		}
	}

	for _, tag := range []ASTTag{
		TagHeading1,
		TagParagraph,
		TagCodeBlock,
		TagListItem,
		TagBlockquote,
		TagTable,
		TagTableCell,
		TagThematicBreak,
	} {
		if !found[tag] {
			t.Errorf("no point in the document resolved to tag %v", tag)
		}
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
