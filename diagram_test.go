package whynot

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"io"
	"testing"
	"time"
)

// bareStyleSheet implements StyleSheet with zero-value/no-opinion
// answers everywhere - deliberately does *not* implement
// DiagramStyleSheet, so diagramPadding's own fallback branch has
// something real to exercise (a StyleSheet written before
// DiagramStyleSheet existed, or one that simply doesn't care).
type bareStyleSheet struct{}

func (bareStyleSheet) Margins(*ASTNode) Margins                       { return Margins{} }
func (bareStyleSheet) TextStyle(*ASTNode) PartialTextStyle            { return PartialTextStyle{} }
func (bareStyleSheet) Color(*ASTNode) color.Color                     { return nil }
func (bareStyleSheet) BorderColor(*ASTNode) color.Color               { return nil }
func (bareStyleSheet) BackgroundColor() color.Color                   { return nil }
func (bareStyleSheet) ViewMargins() Margins                           { return Margins{} }
func (bareStyleSheet) HighlightColor() color.Color                    { return nil }
func (bareStyleSheet) StrikeThickness(*ASTNode) float64               { return 0 }
func (bareStyleSheet) ThematicBreakThickness(*ASTNode) float64        { return 0 }
func (bareStyleSheet) BlockquoteGeometry(*ASTNode) BlockquoteGeometry { return BlockquoteGeometry{} }
func (bareStyleSheet) TableGeometry(*ASTNode) TableGeometry           { return TableGeometry{} }
func (bareStyleSheet) LineHeight(*ASTNode) float64                    { return 1 }

var _ StyleSheet = bareStyleSheet{}

// overriddenPaddingStyleSheet demonstrates the actual extension
// mechanism DiagramStyleSheet's own doc comment describes: embed
// *DefaultStyleSheet (so every other method, and DiagramPadding's own
// default, come along for free) and override just the one method a
// caller wants to customize.
type overriddenPaddingStyleSheet struct {
	*DefaultStyleSheet
}

func (s overriddenPaddingStyleSheet) DiagramPadding(*ASTNode) float64 { return 99 }

func TestDiagramPadding(t *testing.T) {
	if got := diagramPadding(bareStyleSheet{}, nil); got != fallbackDiagramPadding {
		t.Errorf("diagramPadding with a StyleSheet not implementing DiagramStyleSheet = %v, want fallbackDiagramPadding (%v)", got, fallbackDiagramPadding)
	}
	if got := diagramPadding(NewDarkStyleSheet(), nil); got != 12 {
		t.Errorf("diagramPadding(NewDarkStyleSheet(), nil) = %v, want 12", got)
	}
	overridden := overriddenPaddingStyleSheet{NewDarkStyleSheet()}
	if got := diagramPadding(overridden, nil); got != 99 {
		t.Errorf("diagramPadding with an embed-and-override StyleSheet = %v, want 99 (the override)", got)
	}
}

// waitForDiagramSettled polls block.GetBlockLayout(ctx, width) until its
// PendingImages() is empty (the diagram resolved) or the test's
// deadline passes - fetchAndDecode always runs on its own goroutine, so
// a test can't assume the very first GetBlockLayout call already
// reflects a settled result.
func waitForDiagramSettled(t *testing.T, block Block, ctx RenderingContext, width int) BlockLayout {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		box := block.GetBlockLayout(ctx, width)
		if len(box.PendingImages()) == 0 {
			return box
		}
		if time.Now().After(deadline) {
			t.Fatal("diagram still pending after 2s")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestDiagramBlockPendingShowsFallbackAndReportsPending checks that
// while the AsyncImage hasn't resolved yet, GetBlockLayout draws
// exactly what fallback's own layout would (the raw/highlighted code, in
// practice) rather than a generic placeholder, while still reporting
// the diagram's own key via PendingImages - so
// View.invalidateChangedImages knows to revisit this slot once it
// settles even though the fallback itself knows nothing about it.
func TestDiagramBlockPendingShowsFallbackAndReportsPending(t *testing.T) {
	release := make(chan struct{})
	img := AsyncImage{
		Key: "diagram-key",
		Fetch: func() (io.ReadCloser, error) {
			<-release
			return io.NopCloser(bytes.NewReader(onePixelPNG(t))), nil
		},
	}
	defer close(release)

	fallback := &fixedHeightBlock{height: 42}
	block := NewDiagramBlock(nil, img, fallback)
	ctx := RenderingContext{ImageCache: NewImageCache(FileImageSource{})}

	box := block.GetBlockLayout(ctx, 300)
	wantBounds := fallback.GetBlockLayout(ctx, 300).Bounds()
	if box.Bounds() != wantBounds {
		t.Errorf("Bounds() while pending = %v, want fallback's own %v", box.Bounds(), wantBounds)
	}
	if pending := box.PendingImages(); len(pending) != 1 || pending[0] != "diagram-key" {
		t.Errorf("PendingImages() while pending = %v, want [\"diagram-key\"]", pending)
	}
	wantHit, wantOffset := fallback.GetBlockLayout(ctx, 300).HitTest(image.Pt(0, 0))
	if hit, offset := box.HitTest(image.Pt(0, 0)); hit != wantHit || offset != wantOffset {
		t.Errorf("HitTest while pending = %v, %v, want it to delegate to the fallback's own %v, %v", hit, offset, wantHit, wantOffset)
	}
}

// TestDiagramBlockFailedShowsFallbackAndReportsPending checks that a
// failed fetch is treated exactly like a pending one from
// GetBlockLayout's own contract - the fallback is shown, and the key is
// still reported so a later retry (see ImageCache's own retry delay)
// still gets picked up by View.invalidateChangedImages.
func TestDiagramBlockFailedShowsFallbackAndReportsPending(t *testing.T) {
	img := AsyncImage{
		Key:   "diagram-key",
		Fetch: func() (io.ReadCloser, error) { return nil, errors.New("boom") },
	}
	fallback := &fixedHeightBlock{height: 42}
	block := NewDiagramBlock(nil, img, fallback)
	ctx := RenderingContext{ImageCache: NewImageCache(FileImageSource{})}

	box := waitForDiagramFailed(t, block, img, ctx, 300)
	wantBounds := fallback.GetBlockLayout(ctx, 300).Bounds()
	if box.Bounds() != wantBounds {
		t.Errorf("Bounds() after a failed fetch = %v, want fallback's own %v", box.Bounds(), wantBounds)
	}
	if pending := box.PendingImages(); len(pending) != 1 || pending[0] != "diagram-key" {
		t.Errorf("PendingImages() after a failed fetch = %v, want [\"diagram-key\"] (so a later retry is still picked up)", pending)
	}
}

// waitForDiagramFailed is waitForDiagramSettled's counterpart for a
// fetch that's expected to fail rather than settle - a failed
// diagramBlock still reports its key as pending (see
// TestDiagramBlockFailedShowsFallbackAndReportsPending), so it can't be
// distinguished from "still fetching" by PendingImages() alone; poll
// the cache directly (by the same key/AsyncImage the block itself uses)
// until it reports ImageFailed instead.
func waitForDiagramFailed(t *testing.T, block Block, img AsyncImage, ctx RenderingContext, width int) BlockLayout {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		box := block.GetBlockLayout(ctx, width)
		if result := ctx.ImageCache.LoadImage(img); result.Status == ImageFailed {
			return box
		}
		if time.Now().After(deadline) {
			t.Fatal("diagram still not failed after 2s")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestDiagramBlockReadyDrawsImageAndClearsPending checks that once the
// AsyncImage resolves, GetBlockLayout switches to actually drawing the
// decoded image (fit to width, like any other image) instead of the
// fallback, and PendingImages() is empty - nothing left to wait on.
func TestDiagramBlockReadyDrawsImageAndClearsPending(t *testing.T) {
	pixel := onePixelPNG(t)
	img := AsyncImage{
		Key:   "diagram-key",
		Fetch: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(pixel)), nil },
	}
	fallback := &fixedHeightBlock{height: 42}
	block := NewDiagramBlock(nil, img, fallback)
	ctx := RenderingContext{ImageCache: NewImageCache(FileImageSource{}), StyleSheet: noMarginStyleSheet(), Scale: 1}

	box := waitForDiagramSettled(t, block, ctx, 300)
	if box.Bounds() == (fallback.GetBlockLayout(ctx, 300).Bounds()) {
		t.Error("Bounds() once ready still matches the fallback's - want the decoded image's own size")
	}
	if hit, _ := box.HitTest(image.Pt(0, 0)); hit != nil {
		t.Error("HitTest once ready = non-nil, want it to decline (a diagram isn't interactive)")
	}

	canvas := &recordingCanvas{bounds: image.Rect(0, 0, 300, 300)}
	DrawBlockLayout(box, canvas, 0, 0, 0)
	if len(canvas.images) != 1 {
		t.Fatalf("DrawBlockLayout drew %d image(s), want 1", len(canvas.images))
	}
	// A white backdrop (drawn before the image) plus a 4-sided frame
	// (drawn after) - see imageLayout's own doc comment for why: a
	// diagram's own colors assume a light backdrop, and Kroki's output
	// is transparent where nothing is drawn.
	if len(canvas.rects) < 5 {
		t.Fatalf("DrawBlockLayout drew %d rect(s), want at least 5 (a white backdrop + 4-sided frame)", len(canvas.rects))
	}
	if canvas.rects[0].color != color.White {
		t.Errorf("first rect drawn = %v, want a white backdrop drawn before the image", canvas.rects[0].color)
	}

	// The image itself must sit inset from the outer (white+frame)
	// bounds by frameThickness+DiagramPadding on every side - the
	// "matting" a bare frame around the diagram doesn't give it on its
	// own (see DiagramStyleSheet).
	wantInset := int(ctx.ScaledThematicBreakThickness(nil)) + int(diagramPadding(ctx.StyleSheet, nil)*ctx.Scale)
	wantImageRect := image.Rect(wantInset, wantInset, box.Bounds().Dx()-wantInset, box.Bounds().Dy()-wantInset)
	if canvas.imageRects[0] != wantImageRect {
		t.Errorf("image drawn at %v, want %v (inset %d on every side)", canvas.imageRects[0], wantImageRect, wantInset)
	}
}
