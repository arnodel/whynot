package whynot

import (
	"image"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
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

func newTestController(t *testing.T, source string) (*Controller, *View) {
	t.Helper()
	v := NewView(Parse([]byte(source)), fonts.NewGoSelector(), stylingtest.Basic())
	v.Layout(testWidth, testHeight, 1, 0)
	return NewController(v, image.Rect(0, 0, testWidth, testHeight)), v
}

// frame runs one frame of events at time 0.
func frame(c *Controller, events ...input.Event) {
	c.Frame(events, 0)
}

func press(x, y int) input.Event {
	return input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary, Down: true}
}

func release(x, y int) input.Event {
	return input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary}
}

func TestControllerHoverAndClick(t *testing.T) {
	const doc = "A paragraph with [a link](dest) in it, and then some more text below it too."
	c, v := newTestController(t, doc)
	// Offset bounds, to catch a coordinate-translation bug.
	c.SetBounds(image.Rect(50, 30, 50+testWidth, 30+testHeight))
	lx, ly := findLinkPos(t, v, testWidth, testHeight)
	cx, cy := c.Bounds().Min.X+lx, c.Bounds().Min.Y+ly

	var hoverEvents []string
	c.OnLinkHover = func(dest string) { hoverEvents = append(hoverEvents, dest) }
	var clicked string
	var clickCount int
	c.OnLinkClick = func(dest string) { clicked = dest; clickCount++ }

	// Outside bounds entirely - no hover.
	frame(c, input.PointerMove{X: 0, Y: 0})
	if len(hoverEvents) != 0 {
		t.Errorf("hover events outside bounds = %v, want none", hoverEvents)
	}

	// Over the link.
	frame(c, input.PointerMove{X: cx, Y: cy})
	if len(hoverEvents) != 1 || hoverEvents[0] != "dest" {
		t.Errorf("hover events after moving onto the link = %v, want [dest]", hoverEvents)
	}

	// A frame without input - no new event (edge-triggered).
	frame(c)
	if len(hoverEvents) != 1 {
		t.Errorf("hover events after a frame without input = %v, want still just 1", hoverEvents)
	}

	// Click it.
	frame(c, press(cx, cy))
	frame(c, release(cx, cy))
	if clickCount != 1 || clicked != "dest" {
		t.Errorf("OnLinkClick called %d time(s) with %q, want once with \"dest\"", clickCount, clicked)
	}

	// Move off - hover clears.
	frame(c, input.PointerMove{X: c.Bounds().Min.X, Y: c.Bounds().Min.Y})
	if len(hoverEvents) != 2 || hoverEvents[1] != "" {
		t.Errorf("hover events after moving off the link = %v, want a trailing \"\"", hoverEvents)
	}

	// Leave - nothing hovered, already cleared: no new event.
	frame(c, input.PointerLeave{})
	if len(hoverEvents) != 2 {
		t.Errorf("hover events after leaving = %v, want still 2", hoverEvents)
	}
}

// TestControllerLeaveClearsHover checks the pointer leaving clears the
// hovered link.
func TestControllerLeaveClearsHover(t *testing.T) {
	c, v := newTestController(t, "A paragraph with [a link](dest) in it.")
	lx, ly := findLinkPos(t, v, testWidth, testHeight)
	var hovered string
	c.OnLinkHover = func(dest string) { hovered = dest }

	frame(c, input.PointerMove{X: lx, Y: ly})
	if hovered != "dest" {
		t.Fatalf("hovered = %q, want dest", hovered)
	}
	frame(c, input.PointerLeave{})
	if hovered != "" {
		t.Errorf("hovered after leaving = %q, want none", hovered)
	}
}

// TestControllerSecondaryButtonDoesNotClick checks only the primary
// button follows links.
func TestControllerSecondaryButtonDoesNotClick(t *testing.T) {
	c, v := newTestController(t, "A paragraph with [a link](dest) in it.")
	lx, ly := findLinkPos(t, v, testWidth, testHeight)
	clicked := false
	c.OnLinkClick = func(string) { clicked = true }

	frame(c, input.PointerButton{X: lx, Y: ly, Button: input.ButtonSecondary, Down: true})
	if clicked {
		t.Error("a secondary-button press followed the link")
	}
}

// TestControllerTapFollowsLink checks a touch on a link follows it, and
// leaves nothing hovered once lifted.
func TestControllerTapFollowsLink(t *testing.T) {
	c, v := newTestController(t, "A paragraph with [a link](dest) in it.")
	lx, ly := findLinkPos(t, v, testWidth, testHeight)
	var clicked, hovered string
	c.OnLinkClick = func(dest string) { clicked = dest }
	c.OnLinkHover = func(dest string) { hovered = dest }

	frame(c, input.TouchStart{ID: 1, X: lx, Y: ly})
	if clicked != "dest" {
		t.Errorf("clicked = %q after a tap on the link, want dest", clicked)
	}
	frame(c, input.TouchEnd{ID: 1})
	if hovered != "" {
		t.Errorf("hovered = %q after the touch ended, want none", hovered)
	}
}

func TestControllerAnchorScrolling(t *testing.T) {
	// The link sits at the very top (easy to find without scrolling);
	// the heading it targets is far below.
	doc := "[jump](#target)\n\n" + strings.Repeat(longDoc, 10) + "# Target"
	c, v := newTestController(t, doc)
	c.AnchorScrolling = true

	lx, ly := findLinkPos(t, v, testWidth, testHeight)

	var clicked bool
	c.OnLinkClick = func(string) { clicked = true }

	beforeTop := v.VisibleViewBounds(image.Pt(testWidth, testHeight)).Min.Y
	frame(c, press(lx, ly))
	if clicked {
		t.Error("OnLinkClick was called for a #fragment link with AnchorScrolling set")
	}
	afterTop := v.VisibleViewBounds(image.Pt(testWidth, testHeight)).Min.Y
	if afterTop <= beforeTop {
		t.Errorf("VisibleViewBounds top after clicking the anchor link = %d, want more than before (%d)", afterTop, beforeTop)
	}
}

// TestControllerSetViewForgetsHover checks that switching View clears the
// hovered link, so the same link in the new View is reported again.
func TestControllerSetViewForgetsHover(t *testing.T) {
	const doc = "A paragraph with [a link](dest) in it."
	c, v := newTestController(t, doc)
	lx, ly := findLinkPos(t, v, testWidth, testHeight)

	var hoverEvents []string
	c.OnLinkHover = func(dest string) { hoverEvents = append(hoverEvents, dest) }

	frame(c, input.PointerMove{X: lx, Y: ly})
	if len(hoverEvents) != 1 {
		t.Fatalf("hover events before SetView = %v, want 1", hoverEvents)
	}

	c.SetView(v)
	frame(c)
	if len(hoverEvents) != 2 || hoverEvents[1] != "dest" {
		t.Errorf("hover events after SetView = %v, want a second \"dest\"", hoverEvents)
	}
}

func TestControllerWheelGatedByBounds(t *testing.T) {
	c, v := newTestController(t, strings.Repeat(longDoc, 20))
	viewport := image.Pt(testWidth, testHeight)

	// Scroll deep into the document first, so scrolling back afterwards
	// has room to move without clamping at the top.
	frame(c, input.Wheel{X: 0, Y: 0, DY: 5000})
	before := v.VisibleViewBounds(viewport).Min.Y
	if before == 0 {
		t.Fatal("test setup: wheel didn't scroll down")
	}

	frame(c, input.Wheel{X: -1000, Y: -1000, DY: -37}) // outside bounds
	if got := v.VisibleViewBounds(viewport).Min.Y; got != before {
		t.Errorf("VisibleViewBounds top after an out-of-bounds wheel = %d, want unchanged %d", got, before)
	}

	frame(c, input.Wheel{X: 0, Y: 0, DY: -37}) // inside bounds
	if got := v.VisibleViewBounds(viewport).Min.Y; got != before-37 {
		t.Errorf("VisibleViewBounds top after an in-bounds wheel of -37 = %d, want %d", got, before-37)
	}
}

// fling runs a vertical touch drag of dy over d, then lifts the finger,
// returning the time it lifted.
func fling(c *Controller, dy int, d time.Duration) time.Duration {
	c.Frame([]input.Event{input.TouchStart{ID: 1, X: 10, Y: 10}}, 0)
	c.Frame([]input.Event{input.TouchMove{ID: 1, X: 10, Y: 10 + dy}}, d)
	c.Frame([]input.Event{input.TouchEnd{ID: 1}}, d)
	return d
}

func TestControllerMomentum(t *testing.T) {
	c, v := newTestController(t, strings.Repeat(longDoc, 20))
	viewport := image.Pt(testWidth, testHeight)

	// No fling yet - nothing to coast.
	c.Frame(nil, 0)
	if got := v.VisibleViewBounds(viewport).Min.Y; got != 0 {
		t.Fatalf("VisibleViewBounds top after a frame with no fling = %d, want 0", got)
	}

	now := fling(c, -60, 100*time.Millisecond) // -60px over 100ms = -600px/s

	before := v.VisibleViewBounds(viewport).Min.Y
	now += 100 * time.Millisecond
	c.Frame(nil, now)
	if got := v.VisibleViewBounds(viewport).Min.Y; got <= before {
		t.Errorf("VisibleViewBounds top after coasting = %d, want more than %d (coasting forward)", got, before)
	}

	// The app moving the View stops the fling.
	v.ScrollBy(10)
	before = v.VisibleViewBounds(viewport).Min.Y
	now += 100 * time.Millisecond
	c.Frame(nil, now)
	if got := v.VisibleViewBounds(viewport).Min.Y; got != before {
		t.Errorf("VisibleViewBounds top after coasting following View.ScrollBy = %d, want unchanged %d", got, before)
	}
}

// TestControllerSetViewStopsFling checks a fling doesn't carry over to a
// new View.
func TestControllerSetViewStopsFling(t *testing.T) {
	c, _ := newTestController(t, strings.Repeat(longDoc, 20))
	now := fling(c, -60, 100*time.Millisecond)

	v2 := NewView(Parse([]byte(strings.Repeat(longDoc, 20))), fonts.NewGoSelector(), stylingtest.Basic())
	v2.Layout(testWidth, testHeight, 1, now)
	c.SetView(v2)
	c.Frame(nil, now+100*time.Millisecond)
	if got := v2.VisibleViewBounds(image.Pt(testWidth, testHeight)).Min.Y; got != 0 {
		t.Errorf("new View's top after a frame = %d, want 0 (no fling carried over)", got)
	}
}

// TestControllerWheelStopsFling checks a wheel scroll stops a fling.
func TestControllerWheelStopsFling(t *testing.T) {
	c, _ := newTestController(t, strings.Repeat(longDoc, 20))
	now := fling(c, -60, 100*time.Millisecond)
	if !c.Animating() {
		t.Fatal("test setup: no fling")
	}
	c.Frame([]input.Event{input.Wheel{X: 10, Y: 10, DY: 1}}, now)
	if c.Animating() {
		t.Error("still coasting after a wheel scroll")
	}
}

func TestControllerMomentumDecaysToZero(t *testing.T) {
	c, _ := newTestController(t, strings.Repeat(longDoc, 200))
	now := fling(c, -6000, 100*time.Millisecond) // a strong flick

	for i := 0; i < 1000 && c.momentum != 0; i++ {
		now += 500 * time.Millisecond
		c.Frame(nil, now)
	}
	if c.momentum != 0 {
		t.Error("momentum never decayed to 0")
	}
}

func TestControllerMomentumUnscaled(t *testing.T) {
	// Regression pin for the decay formula itself, independent of a real
	// View.
	c := NewController(&View{}, image.Rect(0, 0, testWidth, testHeight))
	c.momentum = 1000

	c.Frame(nil, 0) // dt=0 on the first frame - no-op
	if c.momentum != 1000 {
		t.Fatalf("momentum after a dt=0 frame = %v, want unchanged 1000", c.momentum)
	}

	second := time.Second / 60
	c.Frame(nil, second)
	if want := 1000 * math.Pow(controllerMomentumDecayPerSecond, second.Seconds()); c.momentum != want {
		t.Errorf("momentum after one frame = %v, want %v", c.momentum, want)
	}
}

// TestControllerMomentumBelowMinimumNeverScrolls checks that a velocity
// already below the stopping speed is dropped rather than applied: after
// a long gap between frames (e.g. a finger held still, then lifted, with
// no frames in between), applying it for the whole gap would jump.
func TestControllerMomentumBelowMinimumNeverScrolls(t *testing.T) {
	c, v := newTestController(t, strings.Repeat(longDoc, 20))
	viewport := image.Pt(testWidth, testHeight)
	c.momentum = -(controllerMomentumMinVelocity - 1)

	if c.moving() {
		t.Error("moving() = true for a velocity below the minimum, want false")
	}
	c.Frame(nil, 0)
	c.Frame(nil, 3*time.Second)
	if got := v.VisibleViewBounds(viewport).Min.Y; got != 0 {
		t.Errorf("VisibleViewBounds top = %d after coasting a below-minimum velocity, want 0 (no scroll)", got)
	}
}
