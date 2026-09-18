package whynot

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font"
)

// fixedHeightBlock always lays out to a fixed height regardless of width,
// for tests that want exact, predictable heights without depending on
// real font metrics.
type fixedHeightBlock struct {
	height int
}

func (b *fixedHeightBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
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

func (b *scaledHeightBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	return NewEmptyBox(width, int(float64(width)*b.scale))
}

func (b *scaledHeightBlock) Margins(ctx RenderingContext) Margins {
	return Margins{}
}

func (b *scaledHeightBlock) Node() *ASTNode {
	return nil
}

// noMarginStyleSheet returns NewDarkStyleSheet with its ViewMargins zeroed,
// for tests that check exact slot indices/heights - the default nonzero
// margin would shift every index by the leading margin slot and need
// accounting for in every expected value, none of which is what these
// tests are about.
func noMarginStyleSheet() *DefaultStyleSheet {
	s := NewDarkStyleSheet()
	s.ViewMargin = Margins{}
	return s
}

func newTestView(blocks ...Block) *View {
	return &View{
		block: &StackBlock{blocks: blocks},
		ctx:   RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: noMarginStyleSheet()},
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
	images []image.Image
}

var _ Canvas = (*recordingCanvas)(nil)

func (c *recordingCanvas) Bounds() image.Rectangle                                      { return c.bounds }
func (c *recordingCanvas) DrawText(s string, face font.Face, x, y int, clr color.Color) {}
func (c *recordingCanvas) DrawImage(img image.Image, x, y, width, height int) {
	c.images = append(c.images, img)
}
func (c *recordingCanvas) DrawRect(x, y, w, h int, clr color.Color) {
	c.rects = append(c.rects, drawnRect{x, y, w, h, clr})
}

// TestViewDrawFillsBackground checks that Draw fills dst's whole bounds
// with the StyleSheet's Background color before drawing content - offset
// bounds (not starting at 0,0) to make sure Min.X/Min.Y are actually used,
// not assumed zero.
func TestViewDrawFillsBackground(t *testing.T) {
	v := newTestView(&fixedHeightBlock{height: 10})
	v.Layout(100, 1, 0)

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
// GetBlockLayout/GetInlineLayout bake in resolved style values (colors, margins
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
	v.Layout(200, 1, 0)
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

	v.Layout(100, 1, 0)
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
	small.ViewMargin = Margins{}
	big := NewDarkStyleSheet()
	big.ParagraphTextStyle.Size = 40
	big.ViewMargin = Margins{}

	v := &View{block: block, ctx: RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: small}}
	v.Layout(200, 1, 0)

	// Slot 1 is the margin gap StackBlock.GetBlockLayout inserts between the two
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

// TestViewRebuildInsertsMarginSlots checks that a nonzero ViewMargins adds
// real leading/trailing EmptyBox slots (not just a draw-time offset), and
// that a zero margin (the common case in other tests) adds none.
func TestViewRebuildInsertsMarginSlots(t *testing.T) {
	style := NewDarkStyleSheet()
	style.ViewMargin = Margins{Top: 10, Bottom: 15}
	v := &View{
		block: &StackBlock{blocks: []Block{&fixedHeightBlock{height: 30}}},
		ctx:   RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: style},
	}
	v.Layout(100, 1, 0)

	if len(v.box.slots) != 3 {
		t.Fatalf("got %d slots, want 3 (top margin, content, bottom margin): %#v", len(v.box.slots), v.box.slots)
	}
	if got := v.box.Bounds().Dy(); got != 10+30+15 {
		t.Errorf("total height = %d, want 55 (10 top + 30 content + 15 bottom)", got)
	}

	noMargin := newTestView(&fixedHeightBlock{height: 30})
	noMargin.Layout(100, 1, 0)
	if len(noMargin.box.slots) != 1 {
		t.Errorf("got %d slots with a zero margin, want 1 (no phantom margin slots)", len(noMargin.box.slots))
	}
}

// TestViewScrollClampsIntoBottomMargin checks that scrolling past the end
// of the document clamps inside the bottom margin slot, not at the end of
// the last real content block - the margin is real (if empty) space in
// the tree, so it's reachable by scrolling exactly like any other slot.
func TestViewScrollClampsIntoBottomMargin(t *testing.T) {
	style := NewDarkStyleSheet()
	style.ViewMargin = Margins{Top: 10, Bottom: 15}
	v := &View{
		block: &StackBlock{blocks: []Block{&fixedHeightBlock{height: 30}}},
		ctx:   RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: style},
	}
	v.Layout(100, 1, 0)

	v.Scroll(-1000)
	wantIndex := len(v.box.slots) - 1
	if v.cursor != (stackCursor{wantIndex, 15}) {
		t.Errorf("after scrolling past the end = %+v, want {%d, 15} (clamped inside the bottom margin)", v.cursor, wantIndex)
	}
}

// TestViewHitTestAppliesMargin checks that a point inside the left margin
// gutter misses even at a y within real content's range, and that a
// point on real content resolves with bounds correctly shifted back into
// the same coordinate space the query arrived in.
func TestViewHitTestAppliesMargin(t *testing.T) {
	style := NewDarkStyleSheet()
	style.ViewMargin = Margins{Top: 10, Bottom: 10, Left: 20, Right: 20}
	v := NewView([]byte("hello"), NewGoFontFaceSelector(72), WithStyleSheet(style))
	v.Layout(300, 1, 0)

	if len(v.box.slots) != 3 {
		t.Fatalf("got %d slots, want 3 (top margin, paragraph, bottom margin): %#v", len(v.box.slots), v.box.slots)
	}
	content := v.box.boxAt(1).Bounds()

	// The content's own top-left corner, shifted into the View's
	// coordinate space by the margin, should land on real content.
	p := image.Pt(20+content.Min.X, 10+content.Min.Y)
	hit, offset := v.HitTest(p.X, p.Y)
	if hit == nil {
		t.Fatalf("hit at the content's own top-left %v = nil, want a match", p)
	}
	if !p.In(hit.Bounds().Add(offset)) {
		t.Errorf("bounds %v (offset %v) don't contain %v", hit.Bounds(), offset, p)
	}

	// Same y, but inside the left margin gutter (x well short of 20).
	if hit, _ := v.HitTest(5, p.Y); hit != nil {
		t.Errorf("hit inside the left margin = %v, want a miss", hit)
	}
}

// TestViewDrawAppliesLeftMargin checks that Draw shifts content by the
// StyleSheet's left margin - top/bottom are exercised via the box tree
// itself (see TestViewRebuildInsertsMarginSlots), but left has no slot to
// carry it, so Draw has to apply it directly.
func TestViewDrawAppliesLeftMargin(t *testing.T) {
	style := NewDarkStyleSheet()
	style.ViewMargin = Margins{Left: 20}
	v := NewView([]byte("---"), NewGoFontFaceSelector(72), WithStyleSheet(style))
	v.Layout(300, 1, 0)

	dst := &recordingCanvas{bounds: image.Rect(0, 0, 300, 100)}
	v.Draw(dst, 0, 0)

	if len(dst.rects) < 2 {
		t.Fatalf("got %d DrawRect calls, want at least 2 (background fill + the rule)", len(dst.rects))
	}
	if rule := dst.rects[1]; rule.x != 20 {
		t.Errorf("rule x = %d, want 20 (the left margin)", rule.x)
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
	v.Layout(100, 1, 0)

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
	v.Layout(100, 1, 0)

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
	v.Layout(100, 1, 0) // heights: [100, 200]

	// Anchor halfway through the second block.
	v.cursor = stackCursor{index: 1, offset: 100}

	v.Layout(50, 1, 0) // heights become [50, 100]; same ratio should give offset 50

	if v.cursor != (stackCursor{1, 50}) {
		t.Errorf("cursor after resize = %+v, want {1, 50} (50%% of the new height 100)", v.cursor)
	}
}

// TestViewHitTestEndToEnd checks that HitTest reaches every kind of
// content in a real document, resolving to the right ASTTag. Exact pixel
// positions aren't predictable across margins/gaps/nesting, so this
// scans a coarse grid over the whole rendered document and just checks
// that *some* point resolves to each expected tag - a topological
// check, not a geometric one.
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

	v := NewView(source, NewGoFontFaceSelector(72))
	const width = 300
	v.Layout(width, 1, 0)

	height := v.box.Bounds().Dy()
	found := map[ASTTag]bool{}
	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			hit, _ := v.HitTest(x, y)
			if hit == nil {
				continue
			}
			found[hit.Source().Node().Tag] = true
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

// findTag scans a coarse grid of points for one that resolves to tag,
// returning the first match - for tests that need a real point on
// specific content without hardcoding pixel positions (exact positions
// depend on font metrics/wrapping, which this sidesteps).
func findTag(v *View, tag ASTTag) (x, y int, ok bool) {
	height := v.box.Bounds().Dy()
	for y := 0; y < height; y += 2 {
		for x := 0; x < v.boxWidth; x += 2 {
			if hit, _ := v.HitTest(x, y); hit != nil {
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
// invalidation (see invalidateSlot), not a full rebuild: v.box itself
// stays the same object throughout.
func TestViewHoverHighlightsLink(t *testing.T) {
	style := NewDarkStyleSheet()
	v := NewView([]byte("click [this](url) now"), NewGoFontFaceSelector(72), WithStyleSheet(style))
	v.Layout(300, 1, 0)

	x, y, ok := findTag(v, TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to TagLink")
	}

	beforeBox := v.box
	dest, ok := v.Hover(x, y)
	if v.box != beforeBox {
		t.Error("Hover onto a link rebuilt the whole box, want surgical per-slot invalidation")
	}
	if v.ctx.HighlightNode == nil {
		t.Fatal("HighlightNode = nil after hovering a link, want non-nil")
	}
	if !ok || dest != "url" {
		t.Errorf("Hover(x, y) = %q, %v, want %q, true", dest, ok, "url")
	}

	hit, _ := v.HitTest(x, y)
	if hit == nil {
		t.Fatal("hit at the link's own position = nil after the hover rebuild")
	}
	text, ok := hit.(*TextBox)
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
	v := NewView([]byte("click [this](https://example.com/target) now"), NewGoFontFaceSelector(72))
	v.Layout(300, 1, 0)

	x, y, ok := findTag(v, TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to TagLink")
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
	v := NewView(source, NewGoFontFaceSelector(72), WithStyleSheet(noMarginStyleSheet()))
	v.Layout(300, 1, 0)

	if ok := v.ScrollToAnchor("does-not-exist"); ok {
		t.Error("ScrollToAnchor for an unknown id returned true, want false")
	}

	if ok := v.ScrollToAnchor("second"); !ok {
		t.Fatal(`ScrollToAnchor("second") = false, want true`)
	}

	hit, _ := v.HitTest(0, 0)
	if hit == nil {
		t.Fatal("hit at the top of the viewport after ScrollToAnchor = nil")
	}
	heading := hit.Source().Node().AncestorTag(TagHeading1)
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
	v := NewView(source, NewGoFontFaceSelector(72), WithStyleSheet(noMarginStyleSheet()))
	v.Layout(300, 1, 0)

	v.Scroll(-500)
	want := v.cursor
	pos := v.ScrollPosition()

	v.Scroll(-1000)
	if v.cursor == want {
		t.Fatal("test setup: further scrolling didn't change the cursor")
	}

	v.RestoreScrollPosition(pos)
	if v.cursor != want {
		t.Errorf("cursor after RestoreScrollPosition = %+v, want %+v", v.cursor, want)
	}
}

// TestViewTitle checks that Title finds the document's first heading,
// at any level, skipping non-heading blocks before it, and joins a
// multi-word heading's words back into a single string.
func TestViewTitle(t *testing.T) {
	source := []byte("Some intro text, before any heading.\n\n## A Heading\n\nMore text.\n")
	v := NewView(source, NewGoFontFaceSelector(72))
	title, ok := v.Title()
	if !ok || title != "A Heading" {
		t.Errorf("Title() = %q, %v, want %q, true", title, ok, "A Heading")
	}
}

// TestViewTitleNoHeading checks that Title reports ok=false for a
// document with no heading at all, rather than an empty string being
// mistaken for a real (if blank) title.
func TestViewTitleNoHeading(t *testing.T) {
	v := NewView([]byte("Just a paragraph, no heading anywhere.\n"), NewGoFontFaceSelector(72))
	if title, ok := v.Title(); ok {
		t.Errorf("Title() = %q, true, want ok=false", title)
	}
}

// TestViewHoverNoOpWhenUnchanged checks that Hover only invalidates a
// slot on an actual highlight transition - calling it again at the
// same position must not pay for rebuilding that slot again. v.box
// itself never changes now (see TestViewHoverHighlightsLink), so the
// slot's own memoized box is what has to stay identical instead.
func TestViewHoverNoOpWhenUnchanged(t *testing.T) {
	v := NewView([]byte("click [this](url) now"), NewGoFontFaceSelector(72))
	v.Layout(300, 1, 0)

	x, y, ok := findTag(v, TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to TagLink")
	}

	v.Hover(x, y)
	_, slot := v.linkNodeAt(x, y)
	slotBox := v.box.slots[slot].box
	v.Hover(x, y)
	if v.box.slots[slot].box != slotBox {
		t.Error("Hover at an unchanged position invalidated the slot again, want a no-op")
	}
}

// TestViewHoverClearsWhenMovingAway checks that moving off a link clears
// HighlightNode (and rebuilds to un-highlight it), rather than leaving
// the last-hovered link highlighted indefinitely.
func TestViewHoverClearsWhenMovingAway(t *testing.T) {
	v := NewView([]byte("click [this](url) now"), NewGoFontFaceSelector(72))
	v.Layout(300, 1, 0)

	x, y, ok := findTag(v, TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to TagLink")
	}
	v.Hover(x, y)
	if v.ctx.HighlightNode == nil {
		t.Fatal("HighlightNode = nil after hovering the link, want non-nil")
	}

	v.Hover(0, 0) // the top-left corner: inside the view margin, never the link
	if v.ctx.HighlightNode != nil {
		t.Error("HighlightNode still set after hovering away from the link")
	}
}

// findTwoLinks scans v for two points landing on two different TagLink
// nodes, for tests that need to hover between distinct links.
func findTwoLinks(t *testing.T, v *View) (x1, y1, x2, y2 int) {
	t.Helper()
	height := v.box.Bounds().Dy()
	var firstNode *ASTNode
	found := 0
	for y := 0; y < height && found < 2; y += 2 {
		for x := 0; x < v.boxWidth && found < 2; x += 2 {
			hit, _ := v.HitTest(x, y)
			if hit == nil {
				continue
			}
			n := hit.Source().Node()
			if n == nil || n.Tag != TagLink {
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
// two slots' memoized boxes - v.box itself is untouched (no full
// rebuild), and so is every other already-resolved slot.
func TestViewHoverSurgicalInvalidation(t *testing.T) {
	v := NewView([]byte(twoLinkDoc), NewGoFontFaceSelector(72))
	v.Layout(300, 1, 0)

	x1, y1, x2, y2 := findTwoLinks(t, v)
	_, slot1 := v.linkNodeAt(x1, y1)
	_, slot2 := v.linkNodeAt(x2, y2)
	if slot1 == slot2 {
		t.Fatal("both links resolved to the same slot, test needs links in different slots")
	}

	before := make([]BlockLayout, len(v.box.slots))
	for i := range v.box.slots {
		before[i] = v.box.boxAt(i)
	}
	beforeBox := v.box

	v.Hover(x1, y1)
	v.Hover(x2, y2)

	if v.box != beforeBox {
		t.Fatal("Hover rebuilt the whole box, want surgical per-slot invalidation")
	}
	for i := range v.box.slots {
		got := v.box.boxAt(i)
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
	style := NewDarkStyleSheet()
	v := NewView([]byte(twoLinkDoc), NewGoFontFaceSelector(72), WithStyleSheet(style))
	v.Layout(300, 1, 0)

	x1, y1, x2, y2 := findTwoLinks(t, v)

	v.Hover(x1, y1)
	if v.ctx.HighlightNode == nil {
		t.Fatal("HighlightNode = nil after hovering link one")
	}

	v.Layout(320, 1, 0) // an unrelated resize, while link one is highlighted

	// The next frame's Hover call, mouse unmoved - what cmd/whynot does
	// every tick - refreshes highlightSlot before anything needs it.
	v.Hover(x1, y1)

	// Move to the other link - link one must actually un-highlight, not
	// get stuck, despite the rebuild in between.
	v.Hover(x2, y2)

	hit, _ := v.HitTest(x1, y1)
	text, ok := hit.(*TextBox)
	if !ok {
		t.Fatalf("hit at link one's position = %T, want *TextBox", hit)
	}
	if text.Color == style.HighlightColor() {
		t.Error("link one is still highlighted after hovering link two, despite a resize in between")
	}
}

// BenchmarkViewHover measures the cost of a hover transition - surgical
// per-slot invalidation (see invalidateSlot), restyling only the slot
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
	v := NewView(source, NewGoFontFaceSelector(72))
	v.Layout(1024, 1, 0)

	x, y, ok := findTag(v, TagLink)
	if !ok {
		b.Fatal("no point in the document resolved to TagLink")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			v.Hover(x, y)
		} else {
			v.Hover(0, 0)
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
	ctx := RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: NewDarkStyleSheet()}
	const width = 1024

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		v := &View{block: block, ctx: ctx}
		v.Layout(width, 1, 0)
		if len(v.box.slots) > 0 {
			v.cursor.index = len(v.box.slots) - 1
			v.cursor.offset = 0
		}
		b.StartTimer()

		v.Layout(width+1, 1, 0)
	}
}

// TestViewDocumentBounds checks that DocumentBounds' height is the sum
// of every top-level slot's real (resolved) height, width the
// last-known Layout width.
func TestViewDocumentBounds(t *testing.T) {
	v := &View{
		boxWidth: 300,
		box: &StackBox{slots: []stackSlot{
			{box: NewEmptyBox(300, 10)},
			{box: NewEmptyBox(300, 20)},
			{box: NewEmptyBox(300, 30)},
		}},
	}
	if got, want := v.DocumentBounds(), image.Rect(0, 0, 300, 60); got != want {
		t.Errorf("DocumentBounds() = %v, want %v", got, want)
	}
}

func TestViewDocumentBoundsNilBox(t *testing.T) {
	v := &View{}
	if got := v.DocumentBounds(); got != (image.Rectangle{}) {
		t.Errorf("DocumentBounds() with no box laid out = %v, want the zero Rectangle", got)
	}
}

// TestViewVisibleViewBounds checks that VisibleViewBounds covers
// exactly the pixel range DrawFrom itself would draw into a viewport
// this size, in the same real-pixel-height units DocumentBounds uses.
func TestViewVisibleViewBounds(t *testing.T) {
	v := &View{
		boxWidth: 300,
		box: &StackBox{slots: []stackSlot{
			{box: NewEmptyBox(300, 50)},
			{box: NewEmptyBox(300, 50)},
			{box: NewEmptyBox(300, 50)},
			{box: NewEmptyBox(300, 50)},
		}},
		cursor: stackCursor{index: 1, offset: 10},
	}
	// Slot 0 is 50px, so the view's top is at 50+10 = 60px. Starting
	// 10px into slot 1 (40px of it left), an 80px-tall viewport covers
	// the rest of slot 1 (40px) and all of slot 2 (its own top at
	// 50+50=100px is within 60+80=140px) - slot 3's top (150px) is past
	// the viewport's bottom, so it's excluded.
	got := v.VisibleViewBounds(image.Pt(300, 80))
	if want := (image.Rect(0, 60, 300, 150)); got != want {
		t.Errorf("VisibleViewBounds() = %v, want %v", got, want)
	}
}

// TestViewVisibleViewBoundsClampsAtDocumentEnd checks that a viewport
// taller than the remaining document doesn't walk past the last slot.
func TestViewVisibleViewBoundsClampsAtDocumentEnd(t *testing.T) {
	v := &View{
		boxWidth: 300,
		box: &StackBox{slots: []stackSlot{
			{box: NewEmptyBox(300, 50)},
			{box: NewEmptyBox(300, 50)},
		}},
		cursor: stackCursor{index: 1, offset: 0},
	}
	got := v.VisibleViewBounds(image.Pt(300, 1000))
	if want := (image.Rect(0, 50, 300, 100)); got != want {
		t.Errorf("VisibleViewBounds() = %v, want %v (clamped to the last slot)", got, want)
	}
}

// TestViewVisibleViewBoundsResolvesRealHeights checks that the forward
// walk computing the bottom edge resolves each slot for real (the same
// way DrawFrom itself would), not from heightEstimate's extrapolated
// average - using the average here was the actual bug behind the
// thumb's size visibly jumping while scrolling: the average is a
// moving target as more of the document gets visited, a real height
// isn't.
func TestViewVisibleViewBoundsResolvesRealHeights(t *testing.T) {
	v := &View{
		boxWidth: 300,
		box: &StackBox{slots: []stackSlot{
			{box: NewEmptyBox(300, 10)},                         // resolved; average would be 10
			{block: &fixedHeightBlock{height: 500}, width: 300}, // NOT resolved yet - real height 500, far from that average
		}},
	}
	got := v.VisibleViewBounds(image.Pt(300, 1000))
	if want := (image.Rect(0, 0, 300, 510)); got != want {
		t.Errorf("VisibleViewBounds() = %v, want %v (slot 1 resolved for real, not estimated from the average)", got, want)
	}
	if v.box.slots[1].box == nil {
		t.Error("VisibleViewBounds didn't actually resolve slot 1 - want it forced, the way DrawFrom would")
	}
}

func TestViewVisibleViewBoundsNilBox(t *testing.T) {
	v := &View{}
	if got := v.VisibleViewBounds(image.Pt(300, 100)); got != (image.Rectangle{}) {
		t.Errorf("VisibleViewBounds() with no box laid out = %v, want the zero Rectangle", got)
	}
}

// TestViewHeightEstimateExtrapolates checks that an unresolved slot's
// height is extrapolated from the average of what's already resolved,
// and that actually resolving it afterward replaces the extrapolation
// with its real height, even when that's far from the average.
func TestViewHeightEstimateExtrapolates(t *testing.T) {
	v := &View{
		boxWidth: 300,
		box: &StackBox{slots: []stackSlot{
			{box: NewEmptyBox(300, 100)},
			{box: NewEmptyBox(300, 300)},
			{}, // unresolved
		}},
	}
	// avg of the two resolved slots (100, 300) is 200, extrapolated for
	// the third -> total 100 + 300 + 200 = 600.
	if got, want := v.DocumentBounds(), image.Rect(0, 0, 300, 600); got != want {
		t.Errorf("DocumentBounds() = %v, want %v (extrapolated)", got, want)
	}

	// Resolve slot 2 to a real height well below the average - the
	// estimate must track the real value, not the stale extrapolation.
	v.box.slots[2].box = NewEmptyBox(300, 50)
	if got, want := v.DocumentBounds(), image.Rect(0, 0, 300, 450); got != want {
		t.Errorf("DocumentBounds() after resolving slot 2 = %v, want %v", got, want)
	}
}

// TestViewHeightEstimatePersistsAcrossHoverInvalidation checks that
// invalidating a slot (Hover's surgical invalidation, or any other)
// doesn't regress its contribution to the estimate back to "unknown" -
// the last real height it had stays in slotHeights and keeps being
// used until the slot is naturally re-resolved.
func TestViewHeightEstimatePersistsAcrossHoverInvalidation(t *testing.T) {
	source := []byte("first paragraph\n\n[a link](url)\n\nthird paragraph")
	v := NewView(source, NewGoFontFaceSelector(72))
	v.Layout(300, 1, 0)
	for i := range v.box.slots {
		v.box.boxAt(i) // resolve every slot once
	}
	before := v.DocumentBounds()

	x, y, ok := findTag(v, TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to TagLink")
	}
	_, slot := v.linkNodeAt(x, y)

	v.Hover(x, y)
	v.Hover(-1, -1)

	if v.box.slots[slot].box != nil {
		t.Fatal("test setup: slot wasn't actually invalidated by Hover")
	}
	if got := v.slotHeights[slot]; got < 0 {
		t.Fatalf("slotHeights[%d] = %v after invalidation, want the last-known real height preserved", slot, got)
	}
	if got := v.DocumentBounds(); got != before {
		t.Errorf("DocumentBounds() after hover invalidation = %v, want unchanged %v", got, before)
	}
}

// TestViewHeightEstimateResetsOnResize checks that a real rebuild (a
// resize here) discards stale per-slot estimates rather than mixing
// pre-resize heights into the post-resize total - unlike Hover, a
// resize genuinely can change every slot's height.
func TestViewHeightEstimateResetsOnResize(t *testing.T) {
	v := &View{
		block: &StackBlock{blocks: []Block{
			&scaledHeightBlock{scale: 1}, // height == width given
			&scaledHeightBlock{scale: 1},
		}},
		ctx: RenderingContext{FaceSelector: NewGoFontFaceSelector(72), StyleSheet: noMarginStyleSheet()},
	}
	v.Layout(100, 1, 0)
	v.box.boxAt(1)         // resolve slot 1 too, not just the cursor's own slot 0
	_ = v.DocumentBounds() // populate slotHeights from both slots before the resize

	// Resize - slot 1's real height is now 200px, but nothing has asked
	// boxAt(1) again yet at the new width, so its stale 100px estimate
	// (captured into slotHeights just above) would leak into the total
	// if slotHeights weren't reset.
	v.Layout(200, 1, 0)

	// Slot 0 is resolved fresh (200px, real - rebuild's own cursor
	// re-anchoring does this); slot 1 stays unresolved, so its
	// contribution is extrapolated from what's known (200), not the
	// stale 100 from before the resize.
	if got, want := v.DocumentBounds(), image.Rect(0, 0, 200, 400); got != want {
		t.Errorf("DocumentBounds() after resize = %v, want %v (stale slot 1 estimate discarded)", got, want)
	}
}

// TestViewBoundsStableAcrossHoverRebuilds is the regression test for
// the bug the parked scrollbar attempt originally hit: at the time,
// Hover triggered a full rebuild on every highlight change, which
// could reset a naive height estimate back to "just the current slot."
// Hover is surgical now (see invalidateSlot) and never touches
// slotHeights, so this passes not because DocumentBounds/
// VisibleViewBounds are insulated from Hover's effects, but because
// there's genuinely nothing for a hover-only change to invalidate -
// the persisted per-slot estimates for whatever Hover nils out (the
// highlighted link's own slot, at most) stay exactly as accurate as
// before, since a highlight never changes a slot's real height.
func TestViewBoundsStableAcrossHoverRebuilds(t *testing.T) {
	source := []byte("first paragraph\n\n[a link](url)\n\nthird paragraph\n\nfourth paragraph\n\nfifth paragraph")
	v := NewView(source, NewGoFontFaceSelector(72))
	v.Layout(300, 1, 0)
	v.Scroll(20) // resolve a couple of slots, the way real scrolling would

	x, y, ok := findTag(v, TagLink)
	if !ok {
		t.Fatal("no point in the document resolved to TagLink")
	}

	wantDoc := v.DocumentBounds()
	wantVisible := v.VisibleViewBounds(image.Pt(300, 200))

	for i := 0; i < 4; i++ {
		v.Hover(x, y)   // HighlightNode: nil -> the link (rebuilds)
		v.Hover(-1, -1) // HighlightNode: the link -> nil (rebuilds again)
		if got := v.DocumentBounds(); got != wantDoc {
			t.Fatalf("DocumentBounds changed after hover rebuild #%d: got %v, want %v", i, got, wantDoc)
		}
		if got := v.VisibleViewBounds(image.Pt(300, 200)); got != wantVisible {
			t.Fatalf("VisibleViewBounds changed after hover rebuild #%d: got %v, want %v", i, got, wantVisible)
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
	source := &countingImageSource{
		resolved: "b.png",
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
	cache := NewImageCache(source)

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, result := cache.Load("b.png")
		if result.Status == ImagePending && result.Bounds != (image.Rectangle{}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bounds never revealed")
		}
		time.Sleep(time.Millisecond)
	}

	settledA := NewEmptyBox(100, 10)
	pendingImg := &ImageBox{bounds: image.Rect(0, 0, 20, 20), pending: []string{"b.png"}}
	pendingSlot := &LineBox{parts: []InlineLayout{pendingImg}}
	settledC := NewEmptyBox(100, 10)

	view := &View{
		ctx:      RenderingContext{ImageCache: cache, FaceSelector: NewGoFontFaceSelector(72)},
		boxWidth: 100,
		boxScale: 1,
		box: &StackBox{slots: []stackSlot{
			{box: settledA},
			{box: pendingSlot},
			{box: settledC},
		}},
	}
	// The bounds-reveal above already happened - mark it seen without
	// going through Layout/rebuild (this hand-built View has no real
	// block to rebuild from). This test is about what happens on the
	// *next* change, once bounds are already known.
	_, view.imageCacheMark = cache.ChangedSince(0)

	close(release)
	waitForSettled(t, cache, "b.png")

	view.Layout(100, 1, 0) // same width/scale -> invalidateChangedImages
	if view.box.slots[0].box != settledA {
		t.Error("unrelated settled slot 0 was touched")
	}
	if view.box.slots[1].box != nil {
		t.Error("slot 1 (pending on b.png) was not invalidated")
	}
	if view.box.slots[2].box != settledC {
		t.Error("unrelated settled slot 2 was touched")
	}
}

// TestViewInvalidateChangedImagesRebuildsFullyWhenBoundsRevealed checks
// that an image with no known size until its fetch fully completes -
// unlike the placeholder-rect case above - triggers a full rebuild
// (every slot, not just the image's own) once its bounds are revealed,
// since a "(loading image…)" text placeholder's height is arbitrary
// and the real image's may differ, unlike a correctly-pre-sized
// placeholder rect.
func TestViewInvalidateChangedImagesRebuildsFullyWhenBoundsRevealed(t *testing.T) {
	full := onePixelPNG(t)
	release := make(chan struct{})
	source := &countingImageSource{
		resolved: "img.png",
		open: func() (io.ReadCloser, error) {
			<-release
			return io.NopCloser(bytes.NewReader(full)), nil
		},
	}

	doc := "first paragraph here\n\n![alt](img.png)\n\nthird paragraph here"
	view := NewView([]byte(doc), NewGoFontFaceSelector(72), WithImageSource(source))
	view.Layout(300, 1, 0)
	view.box.Bounds() // force every slot to resolve once, including the image's

	firstSlotBefore := view.box.slots[0].box
	cursorBefore := view.cursor

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

	view.Layout(300, 1, 0) // same width/scale -> invalidateChangedImages
	view.box.Bounds()

	if view.box.slots[0].box == firstSlotBefore {
		t.Error("unrelated slot 0 was not rebuilt - want a full rebuild once bounds were revealed")
	}
	if view.cursor != cursorBefore {
		t.Errorf("cursor = %+v, want unchanged %+v (an unscrolled view's ratio-based reanchor should land back at the same position)", view.cursor, cursorBefore)
	}
}
