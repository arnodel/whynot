package ebitenrenderer

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/arnodel/whynot"
)

// longDoc is long enough, laid out at testPanelWidth, to exceed
// testPanelHeight and so always show a scrollbar.
const longDoc = "# Heading\n\nSome text, quite a bit of it actually, more than one line's worth.\n\n"

const (
	testPanelWidth  = 300
	testPanelHeight = 200
)

func newTestPanel(t *testing.T, source string, opts ...PanelOption) *Panel {
	t.Helper()
	v := whynot.NewView([]byte(source), whynot.NewGoFontFaceSelector(72))
	return NewPanel(v, New(), image.Rect(0, 0, testPanelWidth, testPanelHeight), opts...)
}

func TestScrollbarThumbRectShortDocument(t *testing.T) {
	p := newTestPanel(t, "just one short line", WithScrollbar())
	if _, ok := p.scrollbarThumbRect(); ok {
		t.Error("scrollbarThumbRect ok = true for a document that fits, want false")
	}
}

func TestScrollbarThumbRectLongDocument(t *testing.T) {
	p := newTestPanel(t, strings.Repeat(longDoc, 20), WithScrollbar())

	r, ok := p.scrollbarThumbRect()
	if !ok {
		t.Fatal("scrollbarThumbRect ok = false for a document taller than the viewport")
	}
	if r.Min.X < p.bounds.Min.X || r.Max.X > p.bounds.Max.X {
		t.Errorf("thumb rect x-range %v outside bounds %v", r, p.bounds)
	}
	if r.Min.Y < p.bounds.Min.Y || r.Max.Y > p.bounds.Max.Y {
		t.Errorf("thumb rect y-range %v outside bounds %v", r, p.bounds)
	}
	if r.Dx() <= 0 {
		t.Errorf("thumb width = %d, want positive", r.Dx())
	}

	p.view.ScrollToRatio(0)
	top, _ := p.scrollbarThumbRect()
	if top.Min.Y != p.bounds.Min.Y {
		t.Errorf("thumb top at ratio 0 = %d, want bounds.Min.Y (%d)", top.Min.Y, p.bounds.Min.Y)
	}

	p.view.ScrollToRatio(1)
	bottom, _ := p.scrollbarThumbRect()
	if bottom.Max.Y != p.bounds.Max.Y {
		t.Errorf("thumb bottom at ratio 1 = %d, want bounds.Max.Y (%d)", bottom.Max.Y, p.bounds.Max.Y)
	}
}

func TestUpdateScrollbarDragPressOutsideThumb(t *testing.T) {
	p := newTestPanel(t, strings.Repeat(longDoc, 20), WithScrollbar())
	r, ok := p.scrollbarThumbRect()
	if !ok {
		t.Fatal("test setup: expected a thumb")
	}
	// Well outside the thumb (thumb is at the right edge).
	consumed := p.updateScrollbarDrag(r.Min.X-50, r.Min.Y, true, true)
	if consumed {
		t.Error("updateScrollbarDrag consumed a press outside the thumb")
	}
	if p.draggingScrollbar {
		t.Error("draggingScrollbar = true after a press outside the thumb")
	}
}

func TestUpdateScrollbarDragPressAndDrag(t *testing.T) {
	p := newTestPanel(t, strings.Repeat(longDoc, 20), WithScrollbar())
	r, ok := p.scrollbarThumbRect()
	if !ok {
		t.Fatal("test setup: expected a thumb")
	}
	grabX, grabY := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	if !p.updateScrollbarDrag(grabX, grabY, true, true) {
		t.Fatal("updateScrollbarDrag didn't consume a press on the thumb")
	}
	if !p.draggingScrollbar {
		t.Error("draggingScrollbar = false after pressing the thumb")
	}
	if !p.scrollbarState.pressed {
		t.Error("scrollbarState.pressed = false while dragging")
	}

	beforeTop := p.view.VisibleViewBounds(image.Pt(testPanelWidth, testPanelHeight)).Min.Y
	// Drag most of the way down the track.
	dragY := p.bounds.Min.Y + int(float64(p.bounds.Dy())*0.8)
	if !p.updateScrollbarDrag(grabX, dragY, true, false) {
		t.Fatal("updateScrollbarDrag didn't consume a continued drag")
	}
	afterTop := p.view.VisibleViewBounds(image.Pt(testPanelWidth, testPanelHeight)).Min.Y
	if afterTop <= beforeTop {
		t.Errorf("scroll position after dragging down = %d, want more than before (%d)", afterTop, beforeTop)
	}

	// Cursor now outside the thumb rect, but mouse still down - the
	// drag should keep being tracked (see updateScrollbarDrag's own
	// doc comment: only *starting* a drag requires hovering).
	if !p.updateScrollbarDrag(grabX-1000, dragY, true, false) {
		t.Error("updateScrollbarDrag stopped tracking an in-progress drag once the cursor left the thumb")
	}
	if !p.draggingScrollbar {
		t.Error("draggingScrollbar = false mid-drag after the cursor left the thumb rect")
	}

	// Release.
	if p.updateScrollbarDrag(grabX, dragY, false, false) {
		t.Error("updateScrollbarDrag consumed input after mouse release")
	}
	if p.draggingScrollbar {
		t.Error("draggingScrollbar = true after release")
	}
}

// findLinkPos scans a grid of panel-local coordinates for one that
// LinkAt resolves to a link - the only way to locate a link from
// outside package whynot, which has no test-only fixture helpers
// exported.
func findLinkPos(t *testing.T, v *whynot.View, w, h int) (x, y int) {
	t.Helper()
	for y := 0; y < h; y += 4 {
		for x := 0; x < w; x += 4 {
			if _, ok := v.LinkAt(x, y); ok {
				return x, y
			}
		}
	}
	t.Fatal("no link found in the rendered viewport")
	return 0, 0
}

func TestUpdateHoverAndClick(t *testing.T) {
	const doc = "A paragraph with [a link](dest) in it, and then some more text below it too."
	// Deliberately offset from the origin to catch a coordinate-
	// translation bug.
	bounds := image.Rect(50, 30, 50+testPanelWidth, 30+testPanelHeight)
	v := whynot.NewView([]byte(doc), whynot.NewGoFontFaceSelector(72))
	p := NewPanel(v, New(), bounds)

	lx, ly := findLinkPos(t, v, bounds.Dx(), bounds.Dy())
	cx, cy := bounds.Min.X+lx, bounds.Min.Y+ly

	var hoverEvents []string
	p.OnLinkHover = func(dest string) { hoverEvents = append(hoverEvents, dest) }
	var clicked string
	var clickCount int
	p.OnLinkClick = func(dest string) { clicked = dest; clickCount++ }

	// Outside bounds entirely - no hover.
	p.update(0, 0, 0, false, false)
	if len(hoverEvents) != 0 {
		t.Errorf("hover events outside bounds = %v, want none", hoverEvents)
	}

	// Over the link.
	p.update(cx, cy, 0, false, false)
	if len(hoverEvents) != 1 || hoverEvents[0] != "dest" {
		t.Errorf("hover events after moving onto the link = %v, want [dest]", hoverEvents)
	}

	// Same position again - no new event (edge-triggered).
	p.update(cx, cy, 0, false, false)
	if len(hoverEvents) != 1 {
		t.Errorf("hover events after a second identical call = %v, want still just 1", hoverEvents)
	}

	// Click it.
	p.update(cx, cy, 0, true, true)
	if clickCount != 1 || clicked != "dest" {
		t.Errorf("OnLinkClick called %d time(s) with %q, want once with \"dest\"", clickCount, clicked)
	}

	// Move off - hover clears.
	p.update(bounds.Min.X, bounds.Min.Y, 0, false, false)
	if len(hoverEvents) != 2 || hoverEvents[1] != "" {
		t.Errorf("hover events after moving off the link = %v, want a trailing \"\"", hoverEvents)
	}
}

func TestUpdateAnchorScrolling(t *testing.T) {
	// The link sits at the very top (easy to find without scrolling);
	// the heading it targets is far below - avoids the fragile dance
	// of scrolling to find a link right at the document's end (where
	// ScrollToRatio(1)'s cursor clamps to the *end* of the last slot,
	// leaving nothing from it actually visible).
	doc := "[jump](#target)\n\n" + strings.Repeat(longDoc, 10) + "# Target"
	v := whynot.NewView([]byte(doc), whynot.NewGoFontFaceSelector(72))
	p := NewPanel(v, New(), image.Rect(0, 0, testPanelWidth, testPanelHeight), WithAnchorScrolling())

	lx, ly := findLinkPos(t, v, testPanelWidth, testPanelHeight)

	var clicked bool
	p.OnLinkClick = func(string) { clicked = true }

	beforeTop := v.VisibleViewBounds(image.Pt(testPanelWidth, testPanelHeight)).Min.Y
	p.update(lx, ly, 0, true, true)
	if clicked {
		t.Error("OnLinkClick was called for a #fragment link with WithAnchorScrolling set")
	}
	afterTop := v.VisibleViewBounds(image.Pt(testPanelWidth, testPanelHeight)).Min.Y
	if afterTop <= beforeTop {
		t.Errorf("VisibleViewBounds top after clicking the anchor link = %d, want more than before (%d)", afterTop, beforeTop)
	}
}

// TestUpdateScrollDeltaPassesThroughUnscaled checks that update passes
// scrollDelta straight to View.Scroll with no extra scaling.
func TestUpdateScrollDeltaPassesThroughUnscaled(t *testing.T) {
	p := newTestPanel(t, strings.Repeat(longDoc, 20))
	viewport := image.Pt(testPanelWidth, testPanelHeight)

	// Scroll deep into the document first (negative moves down, see
	// ScrollDown) so a small delta afterwards has room to move.
	p.update(0, 0, -5000, false, false)
	before := p.view.VisibleViewBounds(viewport).Min.Y

	const delta = 37.0
	p.update(0, 0, delta, false, false)
	after := p.view.VisibleViewBounds(viewport).Min.Y

	if got, want := float64(before-after), delta; got != want {
		t.Errorf("VisibleViewBounds top moved by %v, want exactly %v (scrollDelta unscaled)", got, want)
	}
}

func TestDecayMomentum(t *testing.T) {
	p := newTestPanel(t, "just one short line")

	if delta := p.decayMomentum(); delta != 0 {
		t.Errorf("decayMomentum with no momentum = %v, want 0", delta)
	}

	p.momentum = 10
	if delta := p.decayMomentum(); delta != 10 {
		t.Errorf("first decayMomentum delta = %v, want 10 (unchanged this tick)", delta)
	}
	if p.momentum != 10*momentumFriction {
		t.Errorf("momentum after one tick = %v, want %v", p.momentum, 10*momentumFriction)
	}

	for i := 0; i < 1000 && p.momentum != 0; i++ {
		p.decayMomentum()
	}
	if p.momentum != 0 {
		t.Error("momentum never decayed to 0")
	}
}

// scrollbarStyle is a minimal whynot.ScrollbarStyleSheet for tests,
// wrapping a StyleSheet with one fixed, distinctive color regardless
// of hover/pressed state.
type fixedScrollbarStyleSheet struct {
	whynot.StyleSheet
	c color.Color
}

func (s fixedScrollbarStyleSheet) ScrollbarColor(hover, pressed bool) color.Color { return s.c }

func TestSetView(t *testing.T) {
	styled := fixedScrollbarStyleSheet{StyleSheet: whynot.NewDarkStyleSheet(), c: color.RGBA{1, 2, 3, 4}}

	p := newTestPanel(t, strings.Repeat(longDoc, 20), WithScrollbar(), WithStyleSheet(styled))
	if got := p.scrollbarColor(); got != styled.c {
		t.Fatalf("scrollbarColor before SetView = %v, want %v", got, styled.c)
	}

	// Get it into a mid-drag state so SetView's reset is meaningful to check.
	r, _ := p.scrollbarThumbRect()
	p.updateScrollbarDrag(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, true, true)
	if !p.draggingScrollbar {
		t.Fatal("test setup: expected a drag in progress")
	}
	p.hoverDest = "leftover"

	v2 := whynot.NewView([]byte(strings.Repeat(longDoc, 20)), whynot.NewGoFontFaceSelector(72))
	p.SetView(v2)

	if p.View() != v2 {
		t.Error("View() after SetView doesn't return the new View")
	}
	if p.draggingScrollbar {
		t.Error("draggingScrollbar still true after SetView")
	}
	if p.scrollbarState != (buttonState{}) {
		t.Errorf("scrollbarState after SetView = %+v, want zero value", p.scrollbarState)
	}
	if p.hoverDest != "" {
		t.Errorf("hoverDest after SetView = %q, want \"\"", p.hoverDest)
	}
	// The remembered StyleSheet should carry over to the new View -
	// scrollbarColor is the only externally-observable proof available
	// (View has no public getter for its own current StyleSheet).
	if got := p.scrollbarColor(); got != styled.c {
		t.Errorf("scrollbarColor after SetView = %v, want %v (StyleSheet not re-applied)", got, styled.c)
	}
}

func TestScrollDownUpPageDownUp(t *testing.T) {
	p := newTestPanel(t, strings.Repeat(longDoc, 40))
	viewport := image.Pt(testPanelWidth, testPanelHeight)

	top := func() float64 { return float64(p.view.VisibleViewBounds(viewport).Min.Y) }

	before := top()
	p.ScrollDown()
	if d := top() - before; d <= 0 {
		t.Errorf("ScrollDown didn't move forward: delta = %v", d)
	}

	before = top()
	p.ScrollUp()
	if d := top() - before; d >= 0 {
		t.Errorf("ScrollUp didn't move backward: delta = %v", d)
	}

	before = top()
	p.PageDown()
	wantPage := float64(p.bounds.Dy()) * (1 - pageOverlapFrac)
	if d := top() - before; d < wantPage*0.5 {
		// A loose lower bound - real document layout means it won't be
		// exact, but it should be roughly page-sized, not line-sized.
		t.Errorf("PageDown delta = %v, want roughly %v", d, wantPage)
	}

	before = top()
	p.PageUp()
	if d := top() - before; d > -wantPage*0.5 {
		t.Errorf("PageUp delta = %v, want roughly %v", d, -wantPage)
	}
}
