package whynot

import (
	"image"
	"math"
	"strings"
	"testing"
	"time"
)

const longDoc = "# Heading\n\nSome text, quite a bit of it actually, more than one line's worth.\n\n"

const (
	testWidth  = 300
	testHeight = 200
)

func findLinkPos(t *testing.T, v *View, w, h int) (x, y int) {
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

func newTestInteraction(t *testing.T, source string) (*Interaction, *View) {
	t.Helper()
	v := NewView([]byte(source), NewGoFontFaceSelector(72))
	v.Layout(testWidth, testHeight, 1, 0)
	return &Interaction{View: v, Bounds: image.Rect(0, 0, testWidth, testHeight)}, v
}

func TestInteractionHoverAndClick(t *testing.T) {
	const doc = "A paragraph with [a link](dest) in it, and then some more text below it too."
	in, v := newTestInteraction(t, doc)
	// Offset bounds, to catch a coordinate-translation bug.
	in.Bounds = image.Rect(50, 30, 50+testWidth, 30+testHeight)
	lx, ly := findLinkPos(t, v, testWidth, testHeight)
	cx, cy := in.Bounds.Min.X+lx, in.Bounds.Min.Y+ly

	var hoverEvents []string
	in.OnLinkHover = func(dest string) { hoverEvents = append(hoverEvents, dest) }
	var clicked string
	var clickCount int
	in.OnLinkClick = func(dest string) { clicked = dest; clickCount++ }

	// Outside bounds entirely - no hover.
	in.HoverAndClick(0, 0, false)
	if len(hoverEvents) != 0 {
		t.Errorf("hover events outside bounds = %v, want none", hoverEvents)
	}

	// Over the link.
	in.HoverAndClick(cx, cy, false)
	if len(hoverEvents) != 1 || hoverEvents[0] != "dest" {
		t.Errorf("hover events after moving onto the link = %v, want [dest]", hoverEvents)
	}

	// Same position again - no new event (edge-triggered).
	in.HoverAndClick(cx, cy, false)
	if len(hoverEvents) != 1 {
		t.Errorf("hover events after a second identical call = %v, want still just 1", hoverEvents)
	}

	// Click it.
	in.HoverAndClick(cx, cy, true)
	if clickCount != 1 || clicked != "dest" {
		t.Errorf("OnLinkClick called %d time(s) with %q, want once with \"dest\"", clickCount, clicked)
	}

	// Move off - hover clears.
	in.HoverAndClick(in.Bounds.Min.X, in.Bounds.Min.Y, false)
	if len(hoverEvents) != 2 || hoverEvents[1] != "" {
		t.Errorf("hover events after moving off the link = %v, want a trailing \"\"", hoverEvents)
	}
}

func TestInteractionAnchorScrolling(t *testing.T) {
	// The link sits at the very top (easy to find without scrolling);
	// the heading it targets is far below.
	doc := "[jump](#target)\n\n" + strings.Repeat(longDoc, 10) + "# Target"
	in, v := newTestInteraction(t, doc)
	in.AnchorScrolling = true

	lx, ly := findLinkPos(t, v, testWidth, testHeight)

	var clicked bool
	in.OnLinkClick = func(string) { clicked = true }

	beforeTop := v.VisibleViewBounds(image.Pt(testWidth, testHeight)).Min.Y
	in.HoverAndClick(lx, ly, true)
	if clicked {
		t.Error("OnLinkClick was called for a #fragment link with AnchorScrolling set")
	}
	afterTop := v.VisibleViewBounds(image.Pt(testWidth, testHeight)).Min.Y
	if afterTop <= beforeTop {
		t.Errorf("VisibleViewBounds top after clicking the anchor link = %d, want more than before (%d)", afterTop, beforeTop)
	}
}

func TestInteractionReset(t *testing.T) {
	const doc = "A paragraph with [a link](dest) in it."
	in, v := newTestInteraction(t, doc)
	lx, ly := findLinkPos(t, v, testWidth, testHeight)

	var hoverEvents []string
	in.OnLinkHover = func(dest string) { hoverEvents = append(hoverEvents, dest) }

	in.HoverAndClick(lx, ly, false)
	if len(hoverEvents) != 1 {
		t.Fatalf("hover events before Reset = %v, want 1", hoverEvents)
	}

	in.Reset()

	// Same position, same destination - without Reset this wouldn't
	// re-fire (edge-triggered on a change from the old hoverDest).
	in.HoverAndClick(lx, ly, false)
	if len(hoverEvents) != 2 || hoverEvents[1] != "dest" {
		t.Errorf("hover events after Reset = %v, want a second \"dest\" (Reset should clear the old baseline)", hoverEvents)
	}
}

func TestInteractionScrollGatedByBounds(t *testing.T) {
	in, v := newTestInteraction(t, strings.Repeat(longDoc, 20))
	in.Bounds = image.Rect(0, 0, testWidth, testHeight)
	viewport := image.Pt(testWidth, testHeight)

	// Scroll deep into the document first (negative moves down, see
	// View.Scroll's own doc comment) so a small positive delta
	// afterwards has room to move without clamping at the top.
	in.Scroll(0, 0, -5000)
	before := v.VisibleViewBounds(viewport).Min.Y

	in.Scroll(-1000, -1000, 37) // outside bounds
	if got := v.VisibleViewBounds(viewport).Min.Y; got != before {
		t.Errorf("VisibleViewBounds top after an out-of-bounds Scroll = %d, want unchanged %d", got, before)
	}

	in.Scroll(0, 0, 37) // inside bounds
	if got := v.VisibleViewBounds(viewport).Min.Y; got != before-37 {
		t.Errorf("VisibleViewBounds top after an in-bounds Scroll(37) = %d, want %d", got, before-37)
	}
}

func TestInteractionMomentum(t *testing.T) {
	in, v := newTestInteraction(t, strings.Repeat(longDoc, 20))
	in.Bounds = image.Rect(0, 0, testWidth, testHeight)
	viewport := image.Pt(testWidth, testHeight)

	// No time passed yet - AccumulateMomentum should record nothing to
	// decay from.
	in.Momentum(time.Now())
	if got := v.VisibleViewBounds(viewport).Min.Y; got != 0 {
		t.Fatalf("VisibleViewBounds top after Momentum with none accumulated = %d, want 0", got)
	}

	now := time.Now()
	in.AccumulateMomentum(-600, now) // dt=0 on the very first call - no-op
	now = now.Add(100 * time.Millisecond)
	in.AccumulateMomentum(-60, now) // -60px over 100ms = -600px/s

	before := v.VisibleViewBounds(viewport).Min.Y
	now = now.Add(100 * time.Millisecond)
	in.Momentum(now)
	if got := v.VisibleViewBounds(viewport).Min.Y; got <= before {
		t.Errorf("VisibleViewBounds top after Momentum = %d, want more than %d (coasting forward)", got, before)
	}

	in.CancelMomentum()
	before = v.VisibleViewBounds(viewport).Min.Y
	now = now.Add(100 * time.Millisecond)
	in.Momentum(now)
	if got := v.VisibleViewBounds(viewport).Min.Y; got != before {
		t.Errorf("VisibleViewBounds top after Momentum following CancelMomentum = %d, want unchanged %d", got, before)
	}
}

func TestInteractionMomentumDecaysToZero(t *testing.T) {
	in, _ := newTestInteraction(t, strings.Repeat(longDoc, 200))
	in.Bounds = image.Rect(0, 0, testWidth, testHeight)

	now := time.Now()
	in.AccumulateMomentum(-100, now)
	now = now.Add(100 * time.Millisecond)
	in.AccumulateMomentum(-6000, now) // a strong flick

	for i := 0; i < 1000 && in.momentum != 0; i++ {
		now = now.Add(500 * time.Millisecond)
		in.Momentum(now)
	}
	if in.momentum != 0 {
		t.Error("momentum never decayed to 0")
	}
}

func TestInteractionMomentumUnscaled(t *testing.T) {
	// Regression pin for the decay formula itself, independent of a real
	// View - matches ebitenrenderer's own former TestDecayMomentum.
	in := &Interaction{View: &View{}}
	in.momentum = 1000

	first := time.Now()
	in.Momentum(first) // dt=0 on first call - no-op
	if in.momentum != 1000 {
		t.Fatalf("momentum after a dt=0 call = %v, want unchanged 1000", in.momentum)
	}

	second := first.Add(time.Second / 60)
	in.Momentum(second)
	// Use the exact dt Momentum itself computes (second.Sub(first)), not
	// an independently-derived one - float64(time.Duration)-based dt
	// arithmetic doesn't round-trip exactly through 1.0/60.
	dt := second.Sub(first).Seconds()
	if want := 1000 * math.Pow(interactionMomentumDecayPerSecond, dt); in.momentum != want {
		t.Errorf("momentum after one tick = %v, want %v", in.momentum, want)
	}
}
