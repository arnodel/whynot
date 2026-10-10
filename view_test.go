package whynot

import (
	"bytes"
	"image"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/canvastest"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/imagecache"
	"github.com/arnodel/whynot/internal/markdown"
	"github.com/arnodel/whynot/internal/styling"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// fixedHeightBlock always lays out to a fixed height regardless of width,
// for tests that want exact, predictable heights without depending on
// real font metrics.
type fixedHeightBlock struct {
	height int
}

func (b *fixedHeightBlock) GetBlockLayout(ctx engine.Context, width int) engine.BlockLayout {
	return engine.NewEmptyBox(width, b.height)
}

func (b *fixedHeightBlock) Margins(ctx engine.Context) styling.Margins {
	return styling.Margins{}
}

func (b *fixedHeightBlock) Node() *ast.Node {
	return nil
}

// scaledHeightBlock lays out to a height of width*scale, so a resize
// genuinely changes its height - unlike fixedHeightBlock, which is for
// tests that exercise Layout's ratio-based re-anchoring.
type scaledHeightBlock struct {
	scale float64
}

func (b *scaledHeightBlock) GetBlockLayout(ctx engine.Context, width int) engine.BlockLayout {
	return engine.NewEmptyBox(width, int(float64(width)*b.scale))
}

func (b *scaledHeightBlock) Margins(ctx engine.Context) styling.Margins {
	return styling.Margins{}
}

func (b *scaledHeightBlock) Node() *ast.Node {
	return nil
}

// hitAt is View.hitTest without the slot: the hit itself and where its
// Bounds() go, for tests that check what was hit, not just where.
func hitAt(v *View, x, y int) (engine.Hit, image.Point) {
	hit, offset, _ := v.hitTest(x, y)
	return hit, offset
}

// testDocument wraps blocks as a complete Document, bypassing Parse.
func testDocument(blocks ...engine.Block) *Document {
	pieces := make([]markdown.Piece, len(blocks))
	for i, b := range blocks {
		pieces[i] = markdown.Piece{Block: b}
	}
	return &Document{finished: pieces, version: 1, complete: true}
}

func newTestView(blocks ...engine.Block) *View {
	return &View{
		doc: testDocument(blocks...),
		ctx: engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.NoViewMargin()},
	}
}

// TestViewDrawFillsBackground checks that Draw fills the View's bounds,
// clipped to the canvas, with the StyleSheet's Background color before
// drawing content - offset bounds (not starting at 0,0) to make sure
// Min.X/Min.Y are actually used, not assumed zero.
func TestViewDrawFillsBackground(t *testing.T) {
	v := newTestView(&fixedHeightBlock{height: 10})
	v.SetBounds(image.Rect(5, 10, 105, 1000))

	dst := &canvastest.Recorder{Area: image.Rect(0, 0, 200, 60)}
	v.Draw(dst, 0)

	if len(dst.Rects) == 0 {
		t.Fatal("Draw issued no DrawRect calls, want at least a background fill")
	}
	got := dst.Rects[0]
	want := canvastest.Rect{X: 5, Y: 10, W: 100, H: 50, Color: v.ctx.Styles.BackgroundColor()}
	if got != want {
		t.Errorf("background fill = %+v, want %+v", got, want)
	}
}

// TestViewDrawFillsBackgroundBeforeLayout checks that the background fill
// doesn't depend on Layout having been called yet - Draw shouldn't panic
// or skip the fill just because v.stack.box is still nil.
func TestViewDrawFillsBackgroundBeforeLayout(t *testing.T) {
	v := newTestView(&fixedHeightBlock{height: 10})

	dst := &canvastest.Recorder{Area: image.Rect(0, 0, 100, 50)}
	v.Draw(dst, v.ctx.Time)

	if len(dst.Rects) != 1 {
		t.Fatalf("Draw issued %d DrawRect calls, want exactly 1 (the background fill)", len(dst.Rects))
	}
}

// TestNewViewDefaults checks NewView's defaults: the shared Go font
// selector and simpletheme's dark theme.
func TestNewViewDefaults(t *testing.T) {
	v := NewView(Parse([]byte("hello")))
	if v.ctx.FaceSelector != defaultFaceSelector {
		t.Errorf("FaceSelector = %v, want the shared default", v.ctx.FaceSelector)
	}
	if v.ctx.Styles != simpletheme.DarkStyleSheet.Styles() {
		t.Errorf("StyleSheet = %v, want simpletheme.DarkStyleSheet", v.ctx.Styles)
	}
}

// TestNewViewWithStyleSheet checks that WithStyleSheet overrides NewView's
// default StyleSheet.
func TestNewViewWithStyleSheet(t *testing.T) {
	custom := stylingtest.Basic()
	v := NewView(Parse([]byte("hello")), WithStyleSheet(custom))
	if v.ctx.Styles != custom.Styles() {
		t.Errorf("StyleSheet = %v, want the instance passed via WithStyleSheet", v.ctx.Styles)
	}
}

// TestViewSetStyleSheetRebuildsImmediately checks that a StyleSheet swap
// is visible right away, without waiting for another Layout call -
// GetBlockLayout/GetInlineLayout bake in resolved style values (colors, margins
// collapsed to gaps, font sizes, ...) when the layout tree is built, so
// without an explicit rebuild inside SetStyleSheet nothing would change
// until something else happened to trigger a resize.
func TestViewSetStyleSheetRebuildsImmediately(t *testing.T) {
	block := Parse([]byte("hello world"))
	small := stylingtest.Basic()
	small.ParagraphTextStyle.Size = 10
	big := stylingtest.Basic()
	big.ParagraphTextStyle.Size = 40

	v := &View{doc: block, ctx: engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: small}}
	layoutView(v, 200, 1000, 1, 0)
	smallHeight := v.stack.box.Bounds().Dy()

	v.SetStyleSheet(big)
	bigHeight := v.stack.box.Bounds().Dy()

	if bigHeight <= smallHeight {
		t.Fatalf("height after SetStyleSheet = %d, want > %d (a 40pt paragraph should be taller than a 10pt one)", bigHeight, smallHeight)
	}
}

// TestViewSetStyleSheetBeforeLayout checks that calling SetStyleSheet
// before Layout has ever run doesn't panic (and doesn't waste a rebuild
// at a meaningless zero width) - the eventual first Layout call picks up
// the StyleSheet on its own.
func TestViewSetStyleSheetBeforeLayout(t *testing.T) {
	custom := stylingtest.Basic()
	v := newTestView(&fixedHeightBlock{height: 10})

	v.SetStyleSheet(custom)
	if v.stack.box != nil {
		t.Fatalf("box built before Layout was ever called")
	}

	layoutView(v, 100, 1000, 1, 0)
	if v.ctx.Styles != custom.Styles() {
		t.Errorf("StyleSheet after the first Layout = %v, want the instance passed to SetStyleSheet", v.ctx.Styles)
	}
}

// TestViewSetStyleSheetReanchorsScroll checks that a StyleSheet swap
// re-anchors the scroll position by ratio through the current slot, the
// same way a resize does - a new StyleSheet can change content heights
// (a different font size here) just as reflowing at a new width can, so
// the raw pixel offset alone would point at different content.
func TestViewSetStyleSheetReanchorsScroll(t *testing.T) {
	block := Parse([]byte("first\n\nsecond"))
	small := stylingtest.Basic()
	small.ParagraphTextStyle.Size = 10
	small.ViewMargin = styling.Margins{}
	big := stylingtest.Basic()
	big.ParagraphTextStyle.Size = 40
	big.ViewMargin = styling.Margins{}

	v := &View{doc: block, ctx: engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: small}}
	layoutView(v, 200, 1000, 1, 0)

	// Slot 1 is the margin gap StackBlock.GetBlockLayout inserts between the two
	// paragraphs, not content - the second paragraph is slot 2. Anchor
	// halfway through it.
	if len(v.stack.box.Slots) != 3 {
		t.Fatalf("got %d slots, want 3 (paragraph, gap, paragraph): %#v", len(v.stack.box.Slots), v.stack.box.Slots)
	}
	oldHeight := v.stack.box.BoxAt(2).Bounds().Dy()
	v.stack.cursor = engine.StackCursor{Index: 2, Offset: float64(oldHeight) / 2}

	v.SetStyleSheet(big)

	newHeight := v.stack.box.BoxAt(2).Bounds().Dy()
	if newHeight <= oldHeight {
		t.Fatalf("height at slot 2 after SetStyleSheet = %d, want > %d", newHeight, oldHeight)
	}
	if v.stack.cursor.Index != 2 {
		t.Fatalf("cursor index after SetStyleSheet = %d, want 2", v.stack.cursor.Index)
	}
	if gotRatio := v.stack.cursor.Offset / float64(newHeight); math.Abs(gotRatio-0.5) > 0.02 {
		t.Errorf("cursor ratio through slot 2 after SetStyleSheet = %.3f, want ~0.5 (preserved through the rebuild)", gotRatio)
	}
}

// TestViewRebuildInsertsMarginSlots checks that a nonzero ViewMargins adds
// real leading/trailing EmptyBox slots (not just a draw-time offset), and
// that a zero margin (the common case in other tests) adds none.
func TestViewRebuildInsertsMarginSlots(t *testing.T) {
	style := stylingtest.Basic()
	style.ViewMargin = styling.Margins{Top: 10, Bottom: 15}
	v := &View{
		doc: testDocument(&fixedHeightBlock{height: 30}),
		ctx: engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: style},
	}
	layoutView(v, 100, 1000, 1, 0)

	if len(v.stack.box.Slots) != 3 {
		t.Fatalf("got %d slots, want 3 (top margin, content, bottom margin): %#v", len(v.stack.box.Slots), v.stack.box.Slots)
	}
	if got := v.stack.box.Bounds().Dy(); got != 10+30+15 {
		t.Errorf("total height = %d, want 55 (10 top + 30 content + 15 bottom)", got)
	}

	noMargin := newTestView(&fixedHeightBlock{height: 30})
	layoutView(noMargin, 100, 1000, 1, 0)
	if len(noMargin.stack.box.Slots) != 1 {
		t.Errorf("got %d slots with a zero margin, want 1 (no phantom margin slots)", len(noMargin.stack.box.Slots))
	}
}

// TestViewScrollClampsIntoBottomMargin checks that scrolling past the end
// of the document clamps inside the bottom margin slot, not at the end of
// the last real content block - the margin is real (if empty) space in
// the tree, so it's reachable by scrolling exactly like any other slot.
func TestViewScrollClampsIntoBottomMargin(t *testing.T) {
	style := stylingtest.Basic()
	style.ViewMargin = styling.Margins{Top: 10, Bottom: 15}
	v := &View{
		doc: testDocument(&fixedHeightBlock{height: 30}),
		ctx: engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: style},
	}
	layoutView(v, 100, 1000, 1, 0)

	v.ScrollBy(1000)
	wantIndex := len(v.stack.box.Slots) - 1
	if v.stack.cursor != (engine.StackCursor{Index: wantIndex, Offset: 15}) {
		t.Errorf("after scrolling past the end = %+v, want {%d, 15} (clamped inside the bottom margin)", v.stack.cursor, wantIndex)
	}
}

// TestViewHitTestAppliesMargin checks that a point inside the left margin
// gutter misses even at a y within real content's range, and that a
// point on real content resolves with bounds correctly shifted back into
// the same coordinate space the query arrived in.
func TestViewHitTestAppliesMargin(t *testing.T) {
	style := stylingtest.Basic()
	style.ViewMargin = styling.Margins{Top: 10, Bottom: 10, Left: 20, Right: 20}
	v := NewView(Parse([]byte("hello")), WithStyleSheet(style))
	layoutView(v, 300, 1000, 1, 0)

	if len(v.stack.box.Slots) != 3 {
		t.Fatalf("got %d slots, want 3 (top margin, paragraph, bottom margin): %#v", len(v.stack.box.Slots), v.stack.box.Slots)
	}
	content := v.stack.box.BoxAt(1).Bounds()

	// The content's own top-left corner, shifted into the View's
	// coordinate space by the margin, should land on real content.
	p := image.Pt(20+content.Min.X, 10+content.Min.Y)
	hit, offset := hitAt(v, p.X, p.Y)
	if hit == nil {
		t.Fatalf("hit at the content's own top-left %v = nil, want a match", p)
	}
	if !p.In(hit.Bounds().Add(offset)) {
		t.Errorf("bounds %v (offset %v) don't contain %v", hit.Bounds(), offset, p)
	}

	// Same y, but inside the left margin gutter (x well short of 20).
	if hit, _ := hitAt(v, 5, p.Y); hit != nil {
		t.Errorf("hit inside the left margin = %v, want a miss", hit)
	}
}

// TestViewDrawAppliesLeftMargin checks that Draw shifts content by the
// StyleSheet's left margin - top/bottom are exercised via the box tree
// itself (see TestViewRebuildInsertsMarginSlots), but left has no slot to
// carry it, so Draw has to apply it directly.
func TestViewDrawAppliesLeftMargin(t *testing.T) {
	style := stylingtest.Basic()
	style.ViewMargin = styling.Margins{Left: 20}
	v := NewView(Parse([]byte("---")), WithStyleSheet(style))
	layoutView(v, 300, 1000, 1, 0)

	dst := &canvastest.Recorder{Area: image.Rect(0, 0, 300, 100)}
	v.Draw(dst, v.ctx.Time)

	if len(dst.Rects) < 2 {
		t.Fatalf("got %d DrawRect calls, want at least 2 (background fill + the rule)", len(dst.Rects))
	}
	if rule := dst.Rects[1]; rule.X != 20 {
		t.Errorf("rule x = %d, want 20 (the left margin)", rule.X)
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
	layoutView(v, 100, 1000, 1, 0)

	if v.stack.cursor != (engine.StackCursor{Index: 0, Offset: 0}) {
		t.Fatalf("initial position = %+v, want {0, 0}", v.stack.cursor)
	}

	v.ScrollBy(15)
	if v.stack.cursor != (engine.StackCursor{Index: 1, Offset: 5}) {
		t.Errorf("after Scroll(-15) = %+v, want {1, 5}", v.stack.cursor)
	}

	v.ScrollBy(-15)
	if v.stack.cursor != (engine.StackCursor{Index: 0, Offset: 0}) {
		t.Errorf("after Scroll(15) = %+v, want {0, 0}", v.stack.cursor)
	}

	// Scrolling further than the document is long clamps to the end
	// rather than going out of range.
	v.ScrollBy(1000)
	if v.stack.cursor != (engine.StackCursor{Index: 2, Offset: 30}) {
		t.Errorf("after Scroll(-1000) = %+v, want {2, 30} (clamped to the end)", v.stack.cursor)
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
	layoutView(v, 100, 1000, 1, 0)

	v.ScrollBy(7.5)
	v.ScrollBy(7.5)
	if v.stack.cursor != (engine.StackCursor{Index: 1, Offset: 5}) {
		t.Errorf("after Scroll(-7.5) twice = %+v, want {1, 5}", v.stack.cursor)
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
	layoutView(v, 100, 1000, 1, 0) // heights: [100, 200]

	// Anchor halfway through the second block.
	v.stack.cursor = engine.StackCursor{Index: 1, Offset: 100}

	layoutView(v, 50, 1000, 1, 0) // heights become [50, 100]; same ratio should give offset 50

	if v.stack.cursor != (engine.StackCursor{Index: 1, Offset: 50}) {
		t.Errorf("cursor after resize = %+v, want {1, 50} (50%% of the new height 100)", v.stack.cursor)
	}
}

// TestViewHitTestEndToEnd checks that HitTest reaches every kind of
// content in a real document, resolving to the right ast.Tag. Exact pixel
// positions aren't predictable across margins/gaps/nesting, so this
// scans a coarse grid over the whole rendered document and just checks
// that *some* point resolves to each expected tag - a topological
// check, not a geometric one.
// TestViewHitTestRectangle checks the public HitTest: the rectangle of
// the content under the point, in Draw's coordinates, or ok=false off
// any content.
func TestViewHitTestRectangle(t *testing.T) {
	style := stylingtest.Basic()
	style.ViewMargin = styling.Margins{Top: 10, Bottom: 10, Left: 20, Right: 20}
	v := NewView(Parse([]byte("hello")), WithStyleSheet(style))
	layoutView(v, 300, 1000, 1, 0)

	content := v.stack.box.BoxAt(1).Bounds()
	p := image.Pt(20+content.Min.X, 10+content.Min.Y)
	hit, offset := hitAt(v, p.X, p.Y)
	if hit == nil {
		t.Fatalf("no hit at %v", p)
	}
	r, ok := v.HitTest(p.X, p.Y)
	if want := hit.Bounds().Add(offset); !ok || r != want {
		t.Errorf("HitTest(%v) = %v, %v; want %v, true", p, r, ok, want)
	}
	if r, ok := v.HitTest(5, p.Y); ok {
		t.Errorf("HitTest in the left margin = %v, true; want a miss", r)
	}
}

func TestViewHitTestEndToEnd(t *testing.T) {
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

	v := NewView(Parse(source), WithStyleSheet(stylingtest.Basic()))
	const width = 300
	layoutView(v, width, 1000, 1, 0)

	height := v.stack.box.Bounds().Dy()
	found := map[ast.Tag]bool{}
	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			hit, _ := hitAt(v, x, y)
			if hit == nil {
				continue
			}
			found[hit.Source().Node().Tag] = true
		}
	}

	for _, tag := range []ast.Tag{
		ast.TagHeading1,
		ast.TagParagraph,
		ast.TagCodeBlock,
		ast.TagListItem,
		ast.TagBlockquote,
		ast.TagTable,
		ast.TagTableCell,
		ast.TagThematicBreak,
	} {
		if !found[tag] {
			t.Errorf("no point in the document resolved to tag %v", tag)
		}
	}
}

// findTag scans a coarse grid of points for one that resolves to tag,
// returning the first match - for tests that need a real point on
// specific content without hardcoding pixel positions (exact positions
// depend on font metrics/wrapping, which this sidesteps).
func findTag(v *View, tag ast.Tag) (x, y int, ok bool) {
	height := v.stack.box.Bounds().Dy()
	for y := 0; y < height; y += 2 {
		for x := 0; x < v.width; x += 2 {
			if hit, _ := hitAt(v, x, y); hit != nil {
				if n := hit.Source().Node(); n != nil && n.Tag == tag {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

// TestViewHoverHighlightsLink checks that hovering a link recolors its
// text to the StyleSheet's HighlightColor - via surgical per-slot
// invalidation (see StackBox.invalidate), not a full rebuild: v.stack.box itself
// stays the same object throughout.
func TestViewHoverHighlightsLink(t *testing.T) {
	style := stylingtest.Basic()
	v := NewView(Parse([]byte("click [this](url) now")), WithStyleSheet(style))
	layoutView(v, 300, 1000, 1, 0)

	x, y, ok := findTag(v, ast.TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to ast.TagLink")
	}

	beforeBox := v.stack.box
	dest, ok := v.hover(x, y)
	if v.stack.box != beforeBox {
		t.Error("Hover onto a link rebuilt the whole box, want surgical per-slot invalidation")
	}
	if v.ctx.HighlightNode == nil {
		t.Fatal("HighlightNode = nil after hovering a link, want non-nil")
	}
	if !ok || dest != "url" {
		t.Errorf("hover(x, y) = %q, %v, want %q, true", dest, ok, "url")
	}

	hit, _ := hitAt(v, x, y)
	if hit == nil {
		t.Fatal("hit at the link's own position = nil after the hover rebuild")
	}
	text, ok := hit.(*engine.TextBox)
	if !ok {
		t.Fatalf("hit = %T, want *TextBox", hit)
	}
	if want := style.HighlightColor(); text.Color != want {
		t.Errorf("hovered link's Color = %v, want %v (HighlightColor)", text.Color, want)
	}
}

// TestViewLinkAt checks that LinkAt resolves a link's own destination,
// and reports ok=false off a link.
func TestViewLinkAt(t *testing.T) {
	v := NewView(Parse([]byte("click [this](https://example.com/target) now")), WithStyleSheet(stylingtest.Basic()))
	layoutView(v, 300, 1000, 1, 0)

	x, y, ok := findTag(v, ast.TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to ast.TagLink")
	}
	dest, ok := v.LinkAt(x, y)
	if !ok {
		t.Fatal("LinkAt on a link position returned ok=false")
	}
	if want := "https://example.com/target"; dest != want {
		t.Errorf("LinkAt destination = %q, want %q", dest, want)
	}

	if _, ok := v.LinkAt(0, 0); ok {
		t.Error("LinkAt off any link returned ok=true, want false")
	}
}

// TestViewScrollToAnchor checks that ScrollToAnchor moves the scroll
// position so the named heading ends up right at the top of the
// viewport, and reports false for an id that doesn't exist. The list
// between the two headings isn't incidental: a top-level list's own
// Node() is nil (see StackBlock.Node), which once crashed
// ScrollToAnchor's scan on any document where a list preceded the
// target heading.
func TestViewScrollToAnchor(t *testing.T) {
	source := []byte("# First\n\n- one\n- two\n\n# Second\n\nMore text.\n")
	v := NewView(Parse(source), WithStyleSheet(stylingtest.NoViewMargin()))
	layoutView(v, 300, 1000, 1, 0)

	if ok := v.ScrollToAnchor("does-not-exist"); ok {
		t.Error("ScrollToAnchor for an unknown id returned true, want false")
	}

	if ok := v.ScrollToAnchor("second"); !ok {
		t.Fatal(`ScrollToAnchor("second") = false, want true`)
	}

	hit, _ := hitAt(v, 0, 0)
	if hit == nil {
		t.Fatal("hit at the top of the viewport after ScrollToAnchor = nil")
	}
	heading := hit.Source().Node().AncestorTag(ast.TagHeading1)
	if heading == nil || heading.ID != "second" {
		t.Errorf("top of viewport after ScrollToAnchor(\"second\") isn't the Second heading (heading = %v)", heading)
	}
}

// TestViewScrollPositionRoundTrip checks that a ScrollPosition captured
// via ScrollPosition and later given to RestoreScrollPosition puts the
// cursor back exactly where it was, even after further scrolling in
// between - the mechanism cmd/whynot's Back relies on to undo an
// in-page anchor jump without keeping a second View around.
func TestViewScrollPositionRoundTrip(t *testing.T) {
	source := []byte(strings.Repeat("# Heading\n\nSome text.\n\n", 20))
	v := NewView(Parse(source), WithStyleSheet(stylingtest.NoViewMargin()))
	layoutView(v, 300, 1000, 1, 0)

	v.ScrollBy(500)
	want := v.stack.cursor
	pos := v.ScrollPosition()

	v.ScrollBy(1000)
	if v.stack.cursor == want {
		t.Fatal("test setup: further scrolling didn't change the cursor")
	}

	v.RestoreScrollPosition(pos)
	if v.stack.cursor != want {
		t.Errorf("cursor after RestoreScrollPosition = %+v, want %+v", v.stack.cursor, want)
	}
}

// TestViewScrollToRatio checks that ScrollToRatio lands at the
// expected slot/offset for a few ratios through a document of known
// per-slot heights (100, 100, 100, 100 - total 400).
func TestViewScrollToRatio(t *testing.T) {
	v := &View{
		width: 300,
		stack: documentStack{box: &engine.StackBox{Context: &engine.Context{}, Slots: []engine.StackSlot{
			{Box: engine.NewEmptyBox(300, 100)},
			{Box: engine.NewEmptyBox(300, 100)},
			{Box: engine.NewEmptyBox(300, 100)},
			{Box: engine.NewEmptyBox(300, 100)},
		}}},
	}

	cases := []struct {
		ratio float64
		want  engine.StackCursor
	}{
		{0, engine.StackCursor{Index: 0, Offset: 0}},
		{0.25, engine.StackCursor{Index: 1, Offset: 0}},
		{0.5, engine.StackCursor{Index: 2, Offset: 0}},
		{0.6, engine.StackCursor{Index: 2, Offset: 40}},
		{1, engine.StackCursor{Index: 3, Offset: 100}},
	}
	for _, c := range cases {
		v.ScrollToRatio(c.ratio)
		if v.stack.cursor != c.want {
			t.Errorf("ScrollToRatio(%v) cursor = %+v, want %+v", c.ratio, v.stack.cursor, c.want)
		}
	}
}

// TestViewScrollToRatioClampsOutOfRange checks that a ratio outside
// [0, 1] - e.g. a scrollbar drag past the track's own ends - clamps
// rather than landing outside the document.
func TestViewScrollToRatioClampsOutOfRange(t *testing.T) {
	v := &View{
		width: 300,
		stack: documentStack{box: &engine.StackBox{Context: &engine.Context{}, Slots: []engine.StackSlot{
			{Box: engine.NewEmptyBox(300, 100)},
			{Box: engine.NewEmptyBox(300, 100)},
		}}},
	}

	v.ScrollToRatio(-1)
	if want := (engine.StackCursor{Index: 0, Offset: 0}); v.stack.cursor != want {
		t.Errorf("ScrollToRatio(-1) cursor = %+v, want %+v", v.stack.cursor, want)
	}

	v.ScrollToRatio(2)
	if want := (engine.StackCursor{Index: 1, Offset: 100}); v.stack.cursor != want {
		t.Errorf("ScrollToRatio(2) cursor = %+v, want %+v", v.stack.cursor, want)
	}
}

// TestViewScrollToRatioNilBox checks that ScrollToRatio is a no-op
// before Layout has ever run, rather than panicking.
func TestViewScrollToRatioNilBox(t *testing.T) {
	v := &View{}
	v.ScrollToRatio(0.5) // must not panic
}

// TestViewScrollToRatioConverges checks the property drag-to-scroll
// relies on. While heights are still estimates, ScrollToRatio can land a
// little off its target, since laying out the slot it lands in refines
// the estimate. But a scrollbar drag calls it again every frame with
// the pointer's ratio, and each call refines the estimate further, so
// the position converges on the target rather than drifting.
//
// Only SetBounds and ScrollToRatio lay anything out here: a frame's
// look-ahead layout (View.Draw's) depends on a time budget, which would
// make the estimates, and the test, depend on the machine's speed.
func TestViewScrollToRatioConverges(t *testing.T) {
	source := []byte(strings.Repeat("# Heading\n\nSome text, quite a bit of it actually.\n\n", 30))
	v := NewView(Parse(source), WithStyleSheet(stylingtest.NoViewMargin()))
	v.SetBounds(image.Rect(0, 0, 300, 200))

	for _, ratio := range []float64{0.1, 0.9, 0.3, 0.7, 0.5} {
		v.ScrollToRatio(ratio)
		first, _ := v.VisibleRange()
		v.ScrollToRatio(ratio) // the next frame of a drag
		second, _ := v.VisibleRange()
		if d := math.Abs(second - ratio); d > 0.001 || d > math.Abs(first-ratio) {
			t.Errorf("ScrollToRatio(%v), twice: VisibleRange starts at %v then %v, want the second within 0.001 and no further than the first", ratio, first, second)
		}
	}
}

// TestViewCurrentHeadingID checks that CurrentHeadingID reports
// whichever heading the current scroll position has scrolled past, and
// ok=false before the first one.
func TestViewCurrentHeadingID(t *testing.T) {
	source := []byte("Intro text, before any heading.\n\n# First\n\nMore text.\n\n# Second\n\nMore text.\n")
	v := NewView(Parse(source), WithStyleSheet(stylingtest.NoViewMargin()))
	layoutView(v, 300, 1000, 1, 0)

	if _, ok := v.CurrentHeadingID(); ok {
		t.Error("CurrentHeadingID() before any heading = ok, want false")
	}

	if ok := v.ScrollToAnchor("first"); !ok {
		t.Fatal(`ScrollToAnchor("first") = false, want true`)
	}
	if id, ok := v.CurrentHeadingID(); !ok || id != "first" {
		t.Errorf("CurrentHeadingID() = %q, %v, want %q, true", id, ok, "first")
	}

	if ok := v.ScrollToAnchor("second"); !ok {
		t.Fatal(`ScrollToAnchor("second") = false, want true`)
	}
	if id, ok := v.CurrentHeadingID(); !ok || id != "second" {
		t.Errorf("CurrentHeadingID() = %q, %v, want %q, true", id, ok, "second")
	}
}

// TestViewHoverNoOpWhenUnchanged checks that Hover only invalidates a
// slot on an actual highlight transition - calling it again at the
// same position must not pay for rebuilding that slot again. v.stack.box
// itself never changes now (see TestViewHoverHighlightsLink), so the
// slot's own memoized box is what has to stay identical instead.
func TestViewHoverNoOpWhenUnchanged(t *testing.T) {
	v := NewView(Parse([]byte("click [this](url) now")), WithStyleSheet(stylingtest.Basic()))
	layoutView(v, 300, 1000, 1, 0)

	x, y, ok := findTag(v, ast.TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to ast.TagLink")
	}

	v.hover(x, y)
	_, slot := v.linkNodeAt(x, y)
	slotBox := v.stack.box.Slots[slot].Box
	v.hover(x, y)
	if v.stack.box.Slots[slot].Box != slotBox {
		t.Error("Hover at an unchanged position invalidated the slot again, want a no-op")
	}
}

// TestViewHoverClearsWhenMovingAway checks that moving off a link clears
// HighlightNode (and rebuilds to un-highlight it), rather than leaving
// the last-hovered link highlighted indefinitely.
func TestViewHoverClearsWhenMovingAway(t *testing.T) {
	v := NewView(Parse([]byte("click [this](url) now")), WithStyleSheet(stylingtest.Basic()))
	layoutView(v, 300, 1000, 1, 0)

	x, y, ok := findTag(v, ast.TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to ast.TagLink")
	}
	v.hover(x, y)
	if v.ctx.HighlightNode == nil {
		t.Fatal("HighlightNode = nil after hovering the link, want non-nil")
	}

	v.hover(0, 0) // the top-left corner: inside the view margin, never the link
	if v.ctx.HighlightNode != nil {
		t.Error("HighlightNode still set after hovering away from the link")
	}
}

// findTwoLinks scans v for two points landing on two different ast.TagLink
// nodes, for tests that need to hover between distinct links.
func findTwoLinks(t *testing.T, v *View) (x1, y1, x2, y2 int) {
	t.Helper()
	height := v.stack.box.Bounds().Dy()
	var firstNode *ast.Node
	found := 0
	for y := 0; y < height && found < 2; y += 2 {
		for x := 0; x < v.width && found < 2; x += 2 {
			hit, _ := hitAt(v, x, y)
			if hit == nil {
				continue
			}
			n := hit.Source().Node()
			if n == nil || n.Tag != ast.TagLink {
				continue
			}
			if found == 0 {
				firstNode = n
				x1, y1 = x, y
				found = 1
			} else if n != firstNode {
				x2, y2 = x, y
				found = 2
			}
		}
	}
	if found < 2 {
		t.Fatalf("found %d distinct links, want 2", found)
	}
	return x1, y1, x2, y2
}

// twoLinkDoc is source for tests that need two links in two different
// top-level slots, with an unrelated slot on either side and between
// them to prove surgical invalidation leaves everything else alone.
const twoLinkDoc = "first paragraph\n\n[link one](url1)\n\nsecond paragraph\n\n[link two](url2)\n\nthird paragraph"

// TestViewHoverSurgicalInvalidation checks that hovering from one link
// to a different one, in a different slot, invalidates exactly those
// two slots' memoized boxes - v.stack.box itself is untouched (no full
// rebuild), and so is every other already-resolved slot.
func TestViewHoverSurgicalInvalidation(t *testing.T) {
	v := NewView(Parse([]byte(twoLinkDoc)), WithStyleSheet(stylingtest.Basic()))
	layoutView(v, 300, 1000, 1, 0)

	x1, y1, x2, y2 := findTwoLinks(t, v)
	_, slot1 := v.linkNodeAt(x1, y1)
	_, slot2 := v.linkNodeAt(x2, y2)
	if slot1 == slot2 {
		t.Fatal("both links resolved to the same slot, test needs links in different slots")
	}

	before := make([]engine.BlockLayout, len(v.stack.box.Slots))
	for i := range v.stack.box.Slots {
		before[i] = v.stack.box.BoxAt(i)
	}
	beforeBox := v.stack.box

	v.hover(x1, y1)
	v.hover(x2, y2)

	if v.stack.box != beforeBox {
		t.Fatal("Hover rebuilt the whole box, want surgical per-slot invalidation")
	}
	for i := range v.stack.box.Slots {
		got := v.stack.box.BoxAt(i)
		switch i {
		case slot1, slot2:
			if got == before[i] {
				t.Errorf("slot %d (hovered) was not invalidated", i)
			}
		default:
			if got != before[i] {
				t.Errorf("slot %d (never hovered) was invalidated, want untouched", i)
			}
		}
	}
}

// TestViewHoverSurvivesRebuildInBetween checks that a link correctly
// un-highlights even after an unrelated full rebuild (a resize) happens
// while it's highlighted - simulating a resize between two frames, with
// the ordinary per-frame Hover call cmd/whynot always makes in between
// (the mechanism highlightSlot's own doc comment relies on to stay
// fresh across a rebuild it can't itself observe).
func TestViewHoverSurvivesRebuildInBetween(t *testing.T) {
	style := stylingtest.Basic()
	v := NewView(Parse([]byte(twoLinkDoc)), WithStyleSheet(style))
	layoutView(v, 300, 1000, 1, 0)

	x1, y1, x2, y2 := findTwoLinks(t, v)

	v.hover(x1, y1)
	if v.ctx.HighlightNode == nil {
		t.Fatal("HighlightNode = nil after hovering link one")
	}

	layoutView(v, 320, 1000, 1, 0) // an unrelated resize, while link one is highlighted

	// The next frame's Hover call, mouse unmoved - what cmd/whynot does
	// every tick - refreshes highlightSlot before anything needs it.
	v.hover(x1, y1)

	// Move to the other link - link one must actually un-highlight, not
	// get stuck, despite the rebuild in between.
	v.hover(x2, y2)

	hit, _ := hitAt(v, x1, y1)
	text, ok := hit.(*engine.TextBox)
	if !ok {
		t.Fatalf("hit at link one's position = %T, want *TextBox", hit)
	}
	if text.Color == style.HighlightColor() {
		t.Error("link one is still highlighted after hovering link two, despite a resize in between")
	}
}

// BenchmarkViewHover measures the cost of a hover transition - surgical
// per-slot invalidation (see StackBox.invalidate), restyling only the slot
// being left and the slot being entered rather than rebuilding the
// whole document. Alternates between the link and a point off it so
// every call is an actual transition, never short-circuited as a no-op.
// ~14µs on testdata/test.md as of this writing, versus ~154µs for the
// full-rebuild approach this replaced, measured the same way.
func BenchmarkViewHover(b *testing.B) {
	source, err := os.ReadFile("testdata/test.md")
	if err != nil {
		b.Fatal(err)
	}
	v := NewView(Parse(source), WithStyleSheet(stylingtest.Basic()))
	layoutView(v, 1024, 1000, 1, 0)

	x, y, ok := findTag(v, ast.TagLink)
	if !ok {
		b.Fatal("no point in the document resolved to ast.TagLink")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			v.hover(x, y)
		} else {
			v.hover(0, 0)
		}
	}
}

// BenchmarkViewLayoutResizeDeep measures a resize while already anchored
// at the very last slot of a large document - the scenario that motivated
// making StackBlock.GetBlockLayout lazy in the first place. It positions the
// anchor directly rather than via Scroll, since walking there via Scroll
// would itself resolve everything in between - not the cost this
// benchmark is isolating.
func BenchmarkViewLayoutResizeDeep(b *testing.B) {
	source, err := os.ReadFile("testdata/test-large.md")
	if err != nil {
		b.Fatal(err)
	}
	block := Parse(source)
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic(), ImageCache: imagecache.NewCache()}
	const width = 1024

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		v := &View{doc: block, ctx: ctx}
		layoutView(v, width, 1000, 1, 0)
		if len(v.stack.box.Slots) > 0 {
			v.stack.cursor.Index = len(v.stack.box.Slots) - 1
			v.stack.cursor.Offset = 0
		}
		b.StartTimer()

		layoutView(v, width+1, 1000, 1, 0)
	}
}

// TestViewStackBounds checks that the stack's height is the sum of every
// top-level slot's real (resolved) height, and its width the laid-out
// width.
func TestViewStackBounds(t *testing.T) {
	v := &View{
		width: 300,
		stack: documentStack{box: &engine.StackBox{Context: &engine.Context{}, Slots: []engine.StackSlot{
			{Box: engine.NewEmptyBox(300, 10)},
			{Box: engine.NewEmptyBox(300, 20)},
			{Box: engine.NewEmptyBox(300, 30)},
		}}},
	}
	if got, want := stackBounds(v), image.Rect(0, 0, 300, 60); got != want {
		t.Errorf("stackBounds = %v, want %v", got, want)
	}
}

func TestViewStackBoundsNilBox(t *testing.T) {
	v := &View{}
	if got := stackBounds(v); got != (image.Rectangle{}) {
		t.Errorf("stackBounds with no box laid out = %v, want the zero Rectangle", got)
	}
}

// TestViewStackVisibleBounds checks that the visible part's height is
// exactly the viewport's own height (not wherever the last drawn slot
// happens to end - see documentStack.visibleRange), clamped to the
// document's total.
func TestViewStackVisibleBounds(t *testing.T) {
	v := &View{
		width: 300,
		stack: documentStack{box: &engine.StackBox{Context: &engine.Context{}, Slots: []engine.StackSlot{
			{Box: engine.NewEmptyBox(300, 50)},
			{Box: engine.NewEmptyBox(300, 50)},
			{Box: engine.NewEmptyBox(300, 50)},
			{Box: engine.NewEmptyBox(300, 50)},
		}}, cursor: engine.StackCursor{Index: 1, Offset: 10}},
	}
	// Slot 0 is 50px, so the view's top is at 50+10 = 60px. An 80px-tall
	// viewport bottoms out at exactly 60+80 = 140px - not 150px (slot
	// 2's own end), even though DrawFrom would still draw all of slot 2
	// (its top at 100px is within the viewport) and only excludes slot
	// 3 (top at 150px).
	got := stackVisibleBounds(v, image.Pt(300, 80))
	if want := (image.Rect(0, 60, 300, 140)); got != want {
		t.Errorf("stackVisibleBounds = %v, want %v", got, want)
	}
}

// TestViewStackVisibleBoundsClampsAtDocumentEnd checks that a viewport
// taller than the remaining document doesn't walk past the last slot.
func TestViewStackVisibleBoundsClampsAtDocumentEnd(t *testing.T) {
	v := &View{
		width: 300,
		stack: documentStack{box: &engine.StackBox{Context: &engine.Context{}, Slots: []engine.StackSlot{
			{Box: engine.NewEmptyBox(300, 50)},
			{Box: engine.NewEmptyBox(300, 50)},
		}}, cursor: engine.StackCursor{Index: 1, Offset: 0}},
	}
	got := stackVisibleBounds(v, image.Pt(300, 1000))
	if want := (image.Rect(0, 50, 300, 100)); got != want {
		t.Errorf("stackVisibleBounds = %v, want %v (clamped to the last slot)", got, want)
	}
}

// TestViewStackVisibleBoundsResolvesRealHeights checks that the forward
// walk computing the bottom edge resolves each slot for real (the same
// way DrawFrom itself would), not from documentStack's height estimate's extrapolated
// average - using the average here was the actual bug behind the
// thumb's size visibly jumping while scrolling: the average is a
// moving target as more of the document gets visited, a real height
// isn't.
func TestViewStackVisibleBoundsResolvesRealHeights(t *testing.T) {
	v := &View{
		width: 300,
		stack: documentStack{box: &engine.StackBox{Context: &engine.Context{}, Slots: []engine.StackSlot{
			{Box: engine.NewEmptyBox(300, 10)},                  // resolved; average would be 10
			{Block: &fixedHeightBlock{height: 500}, Width: 300}, // NOT resolved yet - real height 500, far from that average
		}}},
	}
	got := stackVisibleBounds(v, image.Pt(300, 1000))
	if want := (image.Rect(0, 0, 300, 510)); got != want {
		t.Errorf("stackVisibleBounds = %v, want %v (slot 1 resolved for real, not estimated from the average)", got, want)
	}
	if v.stack.box.Slots[1].Box == nil {
		t.Error("stackVisibleBounds didn't actually resolve slot 1 - want it forced, the way DrawFrom would")
	}
}

func TestViewStackVisibleBoundsNilBox(t *testing.T) {
	v := &View{}
	if got := stackVisibleBounds(v, image.Pt(300, 100)); got != (image.Rectangle{}) {
		t.Errorf("stackVisibleBounds with no box laid out = %v, want the zero Rectangle", got)
	}
}

// TestViewHeightEstimateExtrapolates checks that an unresolved slot's
// height is extrapolated from the average of what's already resolved,
// and that actually resolving it afterward replaces the extrapolation
// with its real height, even when that's far from the average.
func TestViewHeightEstimateExtrapolates(t *testing.T) {
	v := &View{
		width: 300,
		stack: documentStack{box: &engine.StackBox{Context: &engine.Context{}, Slots: []engine.StackSlot{
			{Box: engine.NewEmptyBox(300, 100)},
			{Box: engine.NewEmptyBox(300, 300)},
			{}, // unresolved
		}}},
	}
	// avg of the two resolved slots (100, 300) is 200, extrapolated for
	// the third -> total 100 + 300 + 200 = 600.
	if got, want := stackBounds(v), image.Rect(0, 0, 300, 600); got != want {
		t.Errorf("stackBounds = %v, want %v (extrapolated)", got, want)
	}

	// Resolve slot 2 to a real height well below the average - the
	// estimate must track the real value, not the stale extrapolation.
	v.stack.box.Slots[2].Box = engine.NewEmptyBox(300, 50)
	if got, want := stackBounds(v), image.Rect(0, 0, 300, 450); got != want {
		t.Errorf("stackBounds after resolving slot 2 = %v, want %v", got, want)
	}
}

// TestViewHeightEstimatePersistsAcrossHoverInvalidation checks that
// invalidating a slot (Hover's surgical invalidation, or any other)
// doesn't regress its contribution to the estimate back to "unknown" -
// the last real height it had stays in stack.heights and keeps being
// used until the slot is naturally re-resolved.
func TestViewHeightEstimatePersistsAcrossHoverInvalidation(t *testing.T) {
	source := []byte("first paragraph\n\n[a link](url)\n\nthird paragraph")
	v := NewView(Parse(source), WithStyleSheet(stylingtest.Basic()))
	layoutView(v, 300, 1000, 1, 0)
	for i := range v.stack.box.Slots {
		v.stack.box.BoxAt(i) // resolve every slot once
	}
	before := stackBounds(v)

	x, y, ok := findTag(v, ast.TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to ast.TagLink")
	}
	_, slot := v.linkNodeAt(x, y)

	v.hover(x, y)
	v.hover(-1, -1)

	if v.stack.box.Slots[slot].Box != nil {
		t.Fatal("test setup: slot wasn't actually invalidated by Hover")
	}
	if got := v.stack.heights[slot]; got < 0 {
		t.Fatalf("stack.heights[%d] = %v after invalidation, want the last-known real height preserved", slot, got)
	}
	if got := stackBounds(v); got != before {
		t.Errorf("stackBounds after hover invalidation = %v, want unchanged %v", got, before)
	}
}

// TestViewHeightEstimateSeedsFromStaleValueAcrossResize checks that a
// resize keeps a slot's pre-resize height as a seed estimate rather
// than discarding it to the document-wide average - refined back to
// exact once the slot is actually re-resolved at the new width.
func TestViewHeightEstimateSeedsFromStaleValueAcrossResize(t *testing.T) {
	v := &View{
		doc: testDocument(
			&scaledHeightBlock{scale: 1}, // height == width given
			&scaledHeightBlock{scale: 1},
		),
		ctx: engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.NoViewMargin()},
	}
	layoutView(v, 100, 1000, 1, 0)
	v.stack.box.BoxAt(1) // resolve slot 1 too, not just the cursor's own slot 0
	_ = stackBounds(v)   // populate stack.heights from both slots before the resize

	// Resize - slot 1's real height is now 200px, but nothing has asked
	// boxAt(1) again yet at the new width. rebuild() directly, not
	// Layout(200, ...): Layout now also runs documentStack.preLayout, which
	// would eagerly resolve slot 1 anyway (it's a two-slot document,
	// trivially within preLayoutHeightRadius) - this test is about
	// rebuild's own seed-preservation behavior for a slot nothing has
	// asked for yet, not about whether pre-layout got to it first.
	v.width = 200
	v.rebuild()

	// Slot 0 is resolved fresh (200px, real - rebuild's own cursor
	// re-anchoring does this); slot 1 stays at its stale pre-resize
	// estimate (100) until actually re-resolved.
	if got, want := stackBounds(v), image.Rect(0, 0, 200, 300); got != want {
		t.Errorf("stackBounds after resize = %v, want %v (slot 1's stale estimate kept as a seed)", got, want)
	}

	// Once slot 1 is actually re-resolved at the new width, its real
	// (200px) height replaces the stale seed.
	v.stack.box.BoxAt(1)
	if got, want := stackBounds(v), image.Rect(0, 0, 200, 400); got != want {
		t.Errorf("stackBounds after resolving slot 1 = %v, want %v (stale seed replaced by the real height)", got, want)
	}
}

// TestViewBoundsStableAcrossHoverRebuilds is the regression test for
// the bug the parked scrollbar attempt originally hit: at the time,
// Hover triggered a full rebuild on every highlight change, which
// could reset a naive height estimate back to "just the current slot."
// Hover is surgical now (see StackBox.invalidate) and never touches
// stack.heights, so this passes not because the stack's bounds are
// insulated from Hover's effects, but because
// there's genuinely nothing for a hover-only change to invalidate -
// the persisted per-slot estimates for whatever Hover nils out (the
// highlighted link's own slot, at most) stay exactly as accurate as
// before, since a highlight never changes a slot's real height.
func TestViewBoundsStableAcrossHoverRebuilds(t *testing.T) {
	source := []byte("first paragraph\n\n[a link](url)\n\nthird paragraph\n\nfourth paragraph\n\nfifth paragraph")
	v := NewView(Parse(source), WithStyleSheet(stylingtest.Basic()))
	layoutView(v, 300, 1000, 1, 0)
	v.ScrollBy(-20) // resolve a couple of slots, the way real scrolling would

	x, y, ok := findTag(v, ast.TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to ast.TagLink")
	}

	wantDoc := stackBounds(v)
	wantVisible := stackVisibleBounds(v, image.Pt(300, 200))

	for i := 0; i < 4; i++ {
		v.hover(x, y)   // HighlightNode: nil -> the link (rebuilds)
		v.hover(-1, -1) // HighlightNode: the link -> nil (rebuilds again)
		if got := stackBounds(v); got != wantDoc {
			t.Fatalf("stackBounds changed after hover rebuild #%d: got %v, want %v", i, got, wantDoc)
		}
		if got := stackVisibleBounds(v, image.Pt(300, 200)); got != wantVisible {
			t.Fatalf("stackVisibleBounds changed after hover rebuild #%d: got %v, want %v", i, got, wantVisible)
		}
	}
}

// TestViewInvalidateChangedImagesTargetsOnlyAffectedSlot checks that
// once an image's bounds are already known (the placeholder-rect
// path), its later becoming ready invalidates only the one slot
// waiting on it - unrelated already-resolved slots are left with the
// exact same memoized box, not rebuilt.
func TestViewInvalidateChangedImagesTargetsOnlyAffectedSlot(t *testing.T) {
	full := onePixelPNG(t)
	release := make(chan struct{})
	source := &testImage{
		key: "b.png",
		open: func() (io.ReadCloser, error) {
			// Split after the IHDR chunk (33 bytes: 8-byte signature +
			// 4+4+13+4 for the chunk itself) so DecodeConfig can reveal
			// bounds without needing the blocked remainder - same
			// technique as TestImageCacheHeaderPeekRevealsBoundsEarly.
			head, rest := full[:33], full[33:]
			r := io.MultiReader(bytes.NewReader(head), blockingReader{release: release, rest: bytes.NewReader(rest)})
			return io.NopCloser(r), nil
		},
	}
	cache := imagecache.NewCache()

	deadline := time.Now().Add(2 * time.Second)
	for {
		result := cache.Load(source)
		if result.Status == imagecache.Pending && result.Bounds != (image.Rectangle{}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bounds never revealed")
		}
		time.Sleep(time.Millisecond)
	}

	settledA := engine.NewEmptyBox(100, 10)
	pendingImg := &engine.ImageBox{Rect: image.Rect(0, 0, 20, 20), Pending: []string{"b.png"}}
	pendingSlot := engine.NewLineBox([]engine.InlineLayout{pendingImg}, false)
	settledC := engine.NewEmptyBox(100, 10)

	view := &View{
		ctx:   engine.Context{Scale: 1, ImageCache: cache, FaceSelector: fonts.NewGoSelector()},
		width: 100,
		scale: 1,
		stack: documentStack{box: &engine.StackBox{Context: &engine.Context{}, Slots: []engine.StackSlot{
			{Box: settledA},
			{Box: pendingSlot},
			{Box: settledC},
		}}},
	}
	// The bounds-reveal above already happened - mark it seen without
	// going through Layout/rebuild (this hand-built View has no real
	// block to rebuild from). This test is about what happens on the
	// *next* change, once bounds are already known.
	_, view.imageCacheMark = cache.ChangedSince(0)

	close(release)
	waitForSettled(t, cache, source)

	// invalidateChangedImages directly, not Layout: this hand-built View
	// has no real block for any slot (see above), so documentStack.preLayout -
	// which Layout also runs, and which would immediately try to
	// re-resolve slot 1 once invalidateChangedImages nils it out, per
	// its own already-passed-slot fix if slot 1 were before the cursor,
	// or simply because it's within reach of the zero-value cursor here
	// - would panic calling GetBlockLayout on a nil Block. This test is
	// about invalidateChangedImages's own narrow contract in isolation.
	view.invalidateChangedImages()
	if view.stack.box.Slots[0].Box != settledA {
		t.Error("unrelated settled slot 0 was touched")
	}
	if view.stack.box.Slots[1].Box != nil {
		t.Error("slot 1 (pending on b.png) was not invalidated")
	}
	if view.stack.box.Slots[2].Box != settledC {
		t.Error("unrelated settled slot 2 was touched")
	}
}

// TestViewInvalidateChangedImagesSurgicalWhenBoundsRevealed checks
// that an image's bounds being revealed only invalidates its own slot,
// leaving every other slot's memoized box and height estimate
// untouched - not a full rebuild, even though this is the one
// transition that can change a slot's height (see
// imagecache.Change.BoundsRevealed).
func TestViewInvalidateChangedImagesSurgicalWhenBoundsRevealed(t *testing.T) {
	full := onePixelPNG(t)
	release := make(chan struct{})
	source := &testImage{
		key: "img.png",
		open: func() (io.ReadCloser, error) {
			<-release
			return io.NopCloser(bytes.NewReader(full)), nil
		},
	}

	doc := "first paragraph here\n\n![alt](img.png)\n\nthird paragraph here"
	view := NewView(Parse([]byte(doc), withTestImage(source)), WithStyleSheet(stylingtest.Basic()))
	layoutView(view, 300, 1000, 1, 0)
	view.stack.box.Bounds() // force every slot to resolve once, including the image's

	firstSlotBefore := view.stack.box.Slots[0].Box
	thirdSlotBefore := view.stack.box.Slots[2].Box
	cursorBefore := view.stack.cursor

	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if changes, _ := view.ctx.ImageCache.ChangedSince(0); len(changes) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("image never settled")
		}
		time.Sleep(time.Millisecond)
	}

	layoutView(view, 300, 1000, 1, 0) // same width/scale -> invalidateChangedImages
	view.stack.box.Bounds()

	if view.stack.box.Slots[0].Box != firstSlotBefore {
		t.Error("unrelated slot 0 was touched - want only the image's own slot invalidated")
	}
	if view.stack.box.Slots[2].Box != thirdSlotBefore {
		t.Error("unrelated slot 2 was touched - want only the image's own slot invalidated")
	}
	if view.stack.cursor != cursorBefore {
		t.Errorf("cursor = %+v, want unchanged %+v (the cursor isn't anchored in the changed slot, so nothing needs reanchoring)", view.stack.cursor, cursorBefore)
	}
}

// TestViewInvalidateChangedImagesReanchorsCursorOnItsOwnSlot checks
// that when the cursor is anchored in the slot whose bounds are being
// revealed, its offset is rescaled by ratio to the slot's new height
// rather than left pointing at a stale pixel.
func TestViewInvalidateChangedImagesReanchorsCursorOnItsOwnSlot(t *testing.T) {
	full := onePixelPNG(t)
	release := make(chan struct{})
	source := &testImage{
		key: "img.png",
		open: func() (io.ReadCloser, error) {
			<-release
			return io.NopCloser(bytes.NewReader(full)), nil
		},
	}

	doc := "first paragraph here\n\n![alt](img.png)\n\nthird paragraph here"
	view := NewView(Parse([]byte(doc), withTestImage(source)), WithStyleSheet(stylingtest.Basic()))
	layoutView(view, 300, 1000, 1, 0)
	// Slots: 0 = leading view margin, 1 = "first paragraph here", 2 =
	// inter-block gap, 3 = the image's own paragraph, 4 = gap, 5 =
	// "third paragraph here", 6 = trailing view margin
	// (StackBlock.StackLayout inserts a margin-gap slot between each pair
	// of top-level blocks - see TestViewSetStyleSheetReanchorsScroll).
	const imageSlot = 3
	oldHeight := view.stack.box.BoxAt(imageSlot).Bounds().Dy() // the placeholder's height
	firstParaBefore := view.stack.box.Slots[1].Box

	// Scroll the cursor to be halfway down the image's own placeholder.
	view.stack.cursor = engine.StackCursor{Index: imageSlot, Offset: float64(oldHeight) / 2}

	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if changes, _ := view.ctx.ImageCache.ChangedSince(0); len(changes) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("image never settled")
		}
		time.Sleep(time.Millisecond)
	}

	layoutView(view, 300, 1000, 1, 0) // same width/scale -> invalidateChangedImages

	if view.stack.box.Slots[1].Box != firstParaBefore {
		t.Error("unrelated slot 1 was touched")
	}
	if view.stack.cursor.Index != imageSlot {
		t.Fatalf("cursor.index = %d, want still %d (the image's own slot)", view.stack.cursor.Index, imageSlot)
	}
	newHeight := view.stack.box.BoxAt(imageSlot).Bounds().Dy()
	wantOffset := float64(newHeight) / 2
	if got := view.stack.cursor.Offset; got < wantOffset-0.001 || got > wantOffset+0.001 {
		t.Errorf("cursor.offset = %v, want %v (half of the new height %d, same ratio as before)", got, wantOffset, newHeight)
	}
}

// TestViewPreLayoutNearbyResolvesBothDirectionsBeyondViewport checks
// documentStack.preLayout's actual reach: slots within viewportHeight+
// preLayoutHeightRadius forward, and preLayoutHeightRadius backward,
// of the cursor get resolved without ever being drawn or queried
// directly - and slots further out, in either direction, are left
// lazy. 20 slots x 1000px each, cursor at 10, viewport 2000px: forward
// reach is 2 "free" (already-visible) slots plus 3 genuinely-ahead
// ones (10-14), backward reach is 3 slots (7-9) - see the height math
// in documentStack.preLayoutDirection's own doc comment.
func TestViewPreLayoutNearbyResolvesBothDirectionsBeyondViewport(t *testing.T) {
	const n, slotHeight, viewportHeight = 20, 1000, 2000
	blocks := make([]engine.Block, n)
	for i := range blocks {
		blocks[i] = &fixedHeightBlock{height: slotHeight}
	}
	v := newTestView(blocks...)
	v.stack.cursor = engine.StackCursor{Index: 10}
	layoutView(v, 300, viewportHeight, 1, 0)

	for i := 7; i <= 14; i++ {
		if v.stack.box.Slots[i].Box == nil {
			t.Errorf("slot %d (within reach) not resolved", i)
		}
	}
	for _, i := range []int{0, 6, 15, n - 1} {
		if v.stack.box.Slots[i].Box != nil {
			t.Errorf("slot %d (outside reach) was resolved, want left lazy", i)
		}
	}
}

// slowBlock always lays out to a fixed height, like fixedHeightBlock,
// but sleeps first - for tests exercising preLayoutTimeBudget, which
// only matters when resolving a slot genuinely costs something.
type slowBlock struct {
	height int
	delay  time.Duration
}

func (b *slowBlock) GetBlockLayout(ctx engine.Context, width int) engine.BlockLayout {
	time.Sleep(b.delay)
	return engine.NewEmptyBox(width, b.height)
}

func (b *slowBlock) Margins(ctx engine.Context) styling.Margins { return styling.Margins{} }
func (b *slowBlock) Node() *ast.Node                            { return nil }

// TestViewPreLayoutNearbyRespectsTimeBudget checks that documentStack.preLayout
// stops resolving once preLayoutTimeBudget is spent, rather than
// working through everything within preLayoutHeightRadius regardless
// of cost, and that it picks up where it left off on the next Layout
// call. Inherently timing-sensitive (delay/budget are real wall-clock
// values) - 1ms/slot against a 2ms budget leaves a wide enough margin
// that only severe scheduling contention would make this flaky.
func TestViewPreLayoutNearbyRespectsTimeBudget(t *testing.T) {
	const n = 50
	blocks := make([]engine.Block, n)
	for i := range blocks {
		blocks[i] = &slowBlock{height: 10, delay: time.Millisecond}
	}
	v := newTestView(blocks...)
	layoutView(v, 300, 0, 1, 0)

	countResolved := func() int {
		n := 0
		for i := range v.stack.box.Slots {
			if v.stack.box.Slots[i].Box != nil {
				n++
			}
		}
		return n
	}

	afterFirst := countResolved()
	if afterFirst == 0 {
		t.Fatal("first Layout call resolved nothing")
	}
	if afterFirst >= n {
		t.Fatalf("first Layout call resolved all %d slots - budget didn't cut it off", n)
	}

	layoutView(v, 300, 0, 1, 0) // same width/scale -> documentStack.preLayout continues
	afterSecond := countResolved()
	if afterSecond <= afterFirst {
		t.Errorf("second Layout call resolved %d slots, want more than the first call's %d (progress should continue)", afterSecond, afterFirst)
	}
}

// TestViewPrefetchImageSourcesStartsLoadWithoutLayout checks that
// prefetchImageSources reaches an image slot well beyond
// preLayoutHeightRadius (so documentStack.preLayout itself can't have resolved
// it) and starts loading it - via imagecache.Cache.Load, observed here as
// a fetch, which runs on its own goroutine - without laying that slot
// out.
func TestViewPrefetchImageSourcesStartsLoadWithoutLayout(t *testing.T) {
	source := &testImage{key: "b.png", data: onePixelPNG(t)}
	cache := imagecache.NewCache()

	const fillerHeight = 500
	const imageSlotIndex = 10
	if imageSlotIndex*fillerHeight <= preLayoutHeightRadius {
		t.Fatalf("test setup: image slot at %dpx isn't beyond preLayoutHeightRadius (%d) - strengthen the fixture", imageSlotIndex*fillerHeight, preLayoutHeightRadius)
	}

	blocks := make([]engine.Block, 0, imageSlotIndex+3)
	for i := 0; i < imageSlotIndex; i++ {
		blocks = append(blocks, &fixedHeightBlock{height: fillerHeight})
	}
	imageBlock := &engine.TextBlock{Parts: []engine.Inline{&engine.InlineImage{Src: "b.png", Image: source}}}
	blocks = append(blocks, imageBlock)
	for i := 0; i < 3; i++ {
		blocks = append(blocks, &fixedHeightBlock{height: fillerHeight})
	}

	v := newTestView(blocks...)
	v.soleImages = map[engine.Block]fetch.Source{imageBlock: source}
	v.ctx.ImageCache = cache
	layoutView(v, 300, 0, 1, 0)

	deadline := time.Now().Add(2 * time.Second)
	for source.fetchCalls.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("prefetchImageSources never started loading the far-ahead image")
		}
		time.Sleep(time.Millisecond)
	}
	if v.stack.box.Slots[imageSlotIndex].Box != nil {
		t.Error("image slot got fully resolved - want prefetchImageSources to only kick off the load, not lay anything out")
	}
}

// TestViewInvalidateChangedImagesReResolvesAlreadyPassedSlot checks
// invalidateChangedImages's own guarantee in isolation from
// documentStack.preLayout's broader one: a slot well beyond preLayoutHeightRadius
// behind the cursor - so documentStack.preLayout's own backward walk can't
// reach it either - still gets re-resolved immediately once its
// pending image settles, rather than freezing the height estimate
// at a stale placeholder value forever.
func TestViewInvalidateChangedImagesReResolvesAlreadyPassedSlot(t *testing.T) {
	full, err := os.ReadFile("testdata/cat.jpeg") // 400x600 - much taller than the "(loading image…)" placeholder text
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	source := &testImage{
		key: "cat.jpeg",
		open: func() (io.ReadCloser, error) {
			<-release
			return io.NopCloser(bytes.NewReader(full)), nil
		},
	}
	cache := imagecache.NewCache()

	const fillerHeight = 1000
	const imageSlotIndex = 3
	const cursorSlotIndex = 15 // 12 slots (12000px) behind - well beyond preLayoutHeightRadius (3000)
	if (cursorSlotIndex-imageSlotIndex)*fillerHeight <= preLayoutHeightRadius {
		t.Fatalf("test setup: image slot is only %dpx behind the cursor, within preLayoutHeightRadius (%d) - strengthen the fixture", (cursorSlotIndex-imageSlotIndex)*fillerHeight, preLayoutHeightRadius)
	}

	slots := make([]engine.StackSlot, cursorSlotIndex+1)
	for i := range slots {
		slots[i] = engine.StackSlot{Block: &fixedHeightBlock{height: fillerHeight}, Width: 100}
	}
	// A real TextBlock/InlineImage, like a real document would have -
	// not a hand-built pending box - so re-resolving it after
	// invalidation goes through the exact same GetBlockLayout path
	// production code does.
	slots[imageSlotIndex] = engine.StackSlot{Block: &engine.TextBlock{Parts: []engine.Inline{&engine.InlineImage{Src: "cat.jpeg", Image: source}}}, Width: 100}

	ctx := engine.Context{Scale: 1, ImageCache: cache, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.NoViewMargin()}
	view := &View{
		doc:   &Document{},
		ctx:   ctx,
		width: 100,
		scale: 1,
		stack: documentStack{box: &engine.StackBox{Slots: slots, Width: 100}, cursor: engine.StackCursor{Index: cursorSlotIndex}},
	}
	view.stack.box.Context = &view.ctx // as rebuild wires it

	placeholderHeight := view.stack.box.BoxAt(imageSlotIndex).Bounds().Dy() // still pending, no bounds known yet - "(loading image…)" text height
	view.stack.box.BoxAt(cursorSlotIndex)                                   // resolve the cursor's own slot too, seeding a real estimate
	docBefore := stackBounds(view)

	close(release)
	waitForSettled(t, cache, source)

	layoutView(view, 100, 0, 1, 0)

	if view.stack.box.Slots[imageSlotIndex].Box == nil {
		t.Fatal("already-passed slot left invalidated/nil - nothing will ever re-resolve it now")
	}
	if got := view.stack.box.Slots[imageSlotIndex].Box.Bounds().Dy(); got == placeholderHeight {
		t.Errorf("slot %d's height is still the stale placeholder %d after the real image settled", imageSlotIndex, placeholderHeight)
	}
	if docAfter := stackBounds(view); docAfter == docBefore {
		t.Error("stackBounds unchanged after an already-passed slot's image resolved - want it to grow to reflect the real height")
	}
}

// TestViewScrollingReachesImageAlreadyResolved is the end-to-end proof
// this whole fix exists for: scrolling an ordinary distance toward a
// standalone image, the same way real usage does (Scroll+Layout each
// tick), the image is already imagecache.Ready by the time the cursor
// actually reaches its slot - prefetchImageSources started loading it
// long before the cursor got there, instead of the fetch only starting
// (with zero head start) the instant the cursor arrives.
func TestViewScrollingReachesImageAlreadyResolved(t *testing.T) {
	doc := "# doc\n\n" +
		strings.Repeat("Filler paragraph with a bit of text in it to take up some space.\n\n", 20) +
		"![cat](testdata/cat.jpeg)\n\n" +
		strings.Repeat("More filler text after the image.\n\n", 5)
	root, err := os.OpenRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	v := NewView(Parse([]byte(doc), WithImageRegistry(fetch.NewRegistry(fetch.FileResolver{Root: root}))), WithStyleSheet(stylingtest.Basic()))
	const viewportHeight = 200
	layoutView(v, 300, viewportHeight, 1, 0)

	imgSlot := -1
	for i := range v.stack.box.Slots {
		if _, ok := v.soleImages[v.stack.box.Slots[i].Block]; ok {
			imgSlot = i
			break
		}
	}
	if imgSlot < 0 {
		t.Fatal("test setup: couldn't find the image's own slot")
	}

	// Wait for prefetchImageSources's own fetch (kicked off by the
	// Layout call above) to settle before scrolling starts - same as
	// real usage gets for free while the reader is still reading
	// earlier content, just deterministic here instead of a fixed sleep.
	waitForSettled(t, v.ctx.ImageCache, v.soleImages[v.stack.box.Slots[imgSlot].Block])

	for v.stack.cursor.Index < imgSlot {
		v.ScrollBy(50)
		layoutView(v, 300, viewportHeight, 1, 0)
	}

	if pending := v.stack.box.Slots[imgSlot].Box.PendingImages(); len(pending) != 0 {
		t.Errorf("image slot still pending on %v once the cursor reached it - prefetch didn't get there first", pending)
	}
}

// TestViewScrollToAnchorNoFragmentRules checks ScrollToAnchor finds
// headings only: interpreting "" or "top" the way browsers do is for
// whoever handles links (see Panel).
func TestViewScrollToAnchorNoFragmentRules(t *testing.T) {
	v := NewView(Parse([]byte(strings.Repeat(longDoc, 10))), WithStyleSheet(stylingtest.Basic()))
	layoutView(v, testWidth, testHeight, 1, 0)
	v.ScrollBy(500)
	for _, id := range []string{"", "top"} {
		if v.ScrollToAnchor(id) {
			t.Errorf("ScrollToAnchor(%q) = true with no such heading, want false", id)
		}
	}
	if start, _ := v.VisibleRange(); start == 0 {
		t.Error("ScrollToAnchor moved the View to the top")
	}
}

// TestViewScrollToEnd checks that ScrollToEnd puts the bottom of a long
// document exactly at the bottom of the View, counting as a move, leaves
// a document shorter than the View at its top, and does nothing to an
// empty one.
func TestViewScrollToEnd(t *testing.T) {
	// bottom returns where the end of v's document is, from the top of v.
	bottom := func(v *View) float64 {
		s := &v.stack
		y := -s.cursor.Offset
		for i := s.cursor.Index; i < s.len(); i++ {
			y += float64(s.box.BoxAt(i).Bounds().Dy())
		}
		return y
	}

	long := NewView(Parse([]byte(strings.Repeat("Some text, quite a bit of it actually.\n\n", 30))))
	long.SetBounds(image.Rect(0, 0, 300, 200))
	moves := long.moves
	long.ScrollToEnd()
	if got := bottom(long); got != 200 {
		t.Errorf("after ScrollToEnd, the document ends %v below the View's top, want 200", got)
	}
	if long.moves == moves {
		t.Error("ScrollToEnd didn't count as a move")
	}

	short := NewView(Parse([]byte("Just a line.\n")))
	short.SetBounds(image.Rect(0, 0, 300, 200))
	short.ScrollToEnd()
	if short.stack.cursor != (engine.StackCursor{}) {
		t.Errorf("a short document scrolled to %+v, want its top", short.stack.cursor)
	}

	empty := NewView(Parse(nil))
	empty.SetBounds(image.Rect(0, 0, 300, 200))
	empty.ScrollToEnd()
}

// TestViewDocumentBounds checks that DocumentBounds is exact, starting
// above the View by what's scrolled past and ending below it by what's
// left, that its limit keeps it from laying out more than needed, and
// that a document ending above the View's bottom ends there.
func TestViewDocumentBounds(t *testing.T) {
	v := NewView(Parse([]byte(strings.Repeat("Some text, quite a bit of it actually.\n\n", 200))))
	v.SetBounds(image.Rect(10, 20, 310, 220))
	resolved := func() int {
		n := 0
		for _, slot := range v.stack.box.Slots {
			if slot.Box != nil && slot.Block != nil {
				n++
			}
		}
		return n
	}

	before := resolved()
	if r := v.DocumentBounds(50); r.Min != image.Pt(10, 20) || r.Max.Y <= 220+50 || r.Dx() != 300 {
		t.Errorf("at the start, DocumentBounds(50) = %v, want it to start at the View's top, and reach more than 50 below it", r)
	}
	// What's in view, and 50 pixels more: not the 200 paragraphs.
	if resolved()-before > 10 {
		t.Errorf("DocumentBounds(50) laid out %d more blocks, want only those near the View", resolved()-before)
	}

	total := 0
	for i := range v.stack.box.Slots {
		total += v.stack.box.BoxAt(i).Bounds().Dy()
	}
	v.ScrollBy(1234)
	if r, want := v.DocumentBounds(math.MaxInt), image.Rect(10, 20-1234, 310, 20-1234+total); r != want {
		t.Errorf("DocumentBounds = %v, want %v", r, want)
	}
	if r := v.DocumentBounds(100); 20-r.Min.Y < 100 || 20-r.Min.Y > 1234 {
		t.Errorf("DocumentBounds(100) starts %d above the View, want at least 100, and no more than all of it", 20-r.Min.Y)
	}

	short := NewView(Parse([]byte("Just a line.\n")))
	short.SetBounds(image.Rect(0, 0, 300, 200))
	if r := short.DocumentBounds(math.MaxInt); r.Min.Y != 0 || r.Max.Y >= 200 {
		t.Errorf("a short document: DocumentBounds = %v, want it to end above the View's bottom", r)
	}
}

// TestViewScrollWithin checks that ScrollWithin moves exactly by dy when it
// can, and otherwise stops at the document's start or very end, returning
// what's left, without counting as a move when it can't move at all.
func TestViewScrollWithin(t *testing.T) {
	v := NewView(Parse([]byte(strings.Repeat("Some text, quite a bit of it actually.\n\n", 30))))
	v.SetBounds(image.Rect(0, 0, 300, 200))
	above := func() int { return v.Bounds().Min.Y - v.DocumentBounds(math.MaxInt).Min.Y }
	total := v.DocumentBounds(math.MaxInt).Dy()

	if rest := v.ScrollWithin(300); rest != 0 || above() != 300 {
		t.Errorf("ScrollWithin(300) left %v, at %v, want all of it moved", rest, above())
	}
	if rest := v.ScrollWithin(-500); rest != -200 || above() != 0 {
		t.Errorf("ScrollWithin(-500) from 300 left %v, want -200, at the start", rest)
	}
	moves := v.moves
	if rest := v.ScrollWithin(-10); rest != -10 || v.moves != moves {
		t.Errorf("at the start, ScrollWithin(-10) left %v and moved %v, want -10 left, no move", rest, v.moves != moves)
	}
	if rest := v.ScrollWithin(float64(total) + 50); rest != 50 || above() != total {
		t.Errorf("ScrollWithin past the end left %v, at %v, want 50 left, at the very end, %v", rest, above(), total)
	}
}

// TestViewHeightChangeIsCheap checks the promise SetBounds makes: a change
// of height alone doesn't lay the document out again, and a change of
// width does.
func TestViewHeightChangeIsCheap(t *testing.T) {
	v := NewView(Parse([]byte("# Title\n\nSome text.\n")))
	v.SetBounds(image.Rect(0, 0, 300, 200))
	box := v.stack.box
	v.SetBounds(image.Rect(0, 50, 300, 90))
	if v.stack.box != box {
		t.Error("a change of height laid the document out again")
	}
	v.SetBounds(image.Rect(0, 50, 250, 90))
	if v.stack.box == box {
		t.Error("a change of width didn't lay the document out again")
	}
}
