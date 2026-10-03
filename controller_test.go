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

func findLinkPos(t *testing.T, v *View) (x, y int) {
	t.Helper()
	b := v.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
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
	layoutView(v, testWidth, testHeight, 1, 0)
	return NewController(v), v
}

// frame runs one frame of events at time 0, returning what happened.
func frame(c *Controller, events ...input.Event) []Event {
	return c.Frame(events, 0)
}

func press(x, y int) input.Event {
	return input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary, Down: true}
}

func release(x, y int) input.Event {
	return input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary}
}

// hovered is v's hovered link, or "" if none.
func hovered(v *View) string {
	dest, _ := v.HoveredLink()
	return dest
}

func TestControllerHoverAndClick(t *testing.T) {
	const doc = "A paragraph with [a link](dest) in it, and then some more text below it too."
	c, v := newTestController(t, doc)
	// Offset bounds, to catch a coordinate-translation bug.
	v.SetBounds(image.Rect(50, 30, 50+testWidth, 30+testHeight))
	cx, cy := findLinkPos(t, v)

	// Outside bounds entirely - no hover.
	frame(c, input.PointerMove{X: 0, Y: 0})
	if got := hovered(v); got != "" {
		t.Errorf("hovered link outside bounds = %q, want none", got)
	}

	// Over the link, and it stays hovered without further input.
	frame(c, input.PointerMove{X: cx, Y: cy})
	if got := hovered(v); got != "dest" {
		t.Errorf("hovered link over the link = %q, want dest", got)
	}
	if events := frame(c); len(events) != 0 || hovered(v) != "dest" {
		t.Errorf("a frame without input: events %v, hovered %q; want none, dest", events, hovered(v))
	}

	// Click it: reported once, on the press.
	events := frame(c, press(cx, cy))
	if len(events) != 1 || events[0] != (LinkClick{Destination: "dest"}) {
		t.Errorf("events for a press on the link = %v, want [LinkClick{dest}]", events)
	}
	if events := frame(c, release(cx, cy)); len(events) != 0 {
		t.Errorf("events for the release = %v, want none", events)
	}

	// Move off - hover clears.
	frame(c, input.PointerMove{X: v.Bounds().Min.X, Y: v.Bounds().Min.Y})
	if got := hovered(v); got != "" {
		t.Errorf("hovered link after moving off = %q, want none", got)
	}
}

// TestControllerLeaveClearsHover checks the pointer leaving clears the
// hovered link.
func TestControllerLeaveClearsHover(t *testing.T) {
	c, v := newTestController(t, "A paragraph with [a link](dest) in it.")
	lx, ly := findLinkPos(t, v)

	frame(c, input.PointerMove{X: lx, Y: ly})
	if got := hovered(v); got != "dest" {
		t.Fatalf("hovered = %q, want dest", got)
	}
	frame(c, input.PointerLeave{})
	if got := hovered(v); got != "" {
		t.Errorf("hovered after leaving = %q, want none", got)
	}
}

// TestControllerSecondaryButtonDoesNotClick checks only the primary
// button clicks links.
func TestControllerSecondaryButtonDoesNotClick(t *testing.T) {
	c, v := newTestController(t, "A paragraph with [a link](dest) in it.")
	lx, ly := findLinkPos(t, v)

	events := frame(c, input.PointerButton{X: lx, Y: ly, Button: input.ButtonSecondary, Down: true})
	if len(events) != 0 {
		t.Errorf("events for a secondary-button press on a link = %v, want none", events)
	}
}

// TestControllerTapClicksLink checks a touch on a link clicks it, and
// leaves nothing hovered once lifted.
func TestControllerTapClicksLink(t *testing.T) {
	c, v := newTestController(t, "A paragraph with [a link](dest) in it.")
	lx, ly := findLinkPos(t, v)

	events := frame(c, input.TouchStart{ID: 1, X: lx, Y: ly})
	if len(events) != 1 || events[0] != (LinkClick{Destination: "dest"}) {
		t.Errorf("events for a tap on the link = %v, want [LinkClick{dest}]", events)
	}
	frame(c, input.TouchEnd{ID: 1})
	if got := hovered(v); got != "" {
		t.Errorf("hovered = %q after the touch ended, want none", got)
	}
}

// TestControllerReportsAnchorClick checks a "#id" link is reported as an
// AnchorClick, with its id percent-decoded, and that the Controller
// doesn't act on it.
func TestControllerReportsAnchorClick(t *testing.T) {
	doc := "[jump](#a%20target)\n\n" + strings.Repeat(longDoc, 10) + "# A target"
	c, v := newTestController(t, doc)
	lx, ly := findLinkPos(t, v)

	events := frame(c, press(lx, ly))
	if len(events) != 1 || events[0] != (AnchorClick{ID: "a target"}) {
		t.Errorf("events for a press on an anchor link = %v, want [AnchorClick{a target}]", events)
	}
	if start, _ := v.VisibleRange(); start != 0 {
		t.Errorf("VisibleRange start = %v after the click, want 0 (the Controller doesn't scroll)", start)
	}
}

// TestControllerSetViewClearsHover checks that switching View leaves
// nothing hovered in the old one, and that the next frame hovers what's
// under the pointer in the new one.
func TestControllerSetViewClearsHover(t *testing.T) {
	c, v := newTestController(t, "A paragraph with [a link](dest) in it.")
	lx, ly := findLinkPos(t, v)
	frame(c, input.PointerMove{X: lx, Y: ly})

	plain := NewView(Parse([]byte("Just text, no links at all.")), fonts.NewGoSelector(), stylingtest.Basic())
	plain.SetBounds(v.Bounds())
	c.SetView(plain)
	if got := hovered(v); got != "" {
		t.Errorf("old View's hovered link after SetView = %q, want none", got)
	}

	// Back to the first View: the link under the pointer is hovered again.
	c.SetView(v)
	frame(c)
	if got := hovered(v); got != "dest" {
		t.Errorf("hovered link after switching back and a frame = %q, want dest", got)
	}
}

func TestControllerWheelGatedByBounds(t *testing.T) {
	c, v := newTestController(t, strings.Repeat(longDoc, 20))
	viewport := image.Pt(testWidth, testHeight)

	// Scroll deep into the document first, so scrolling back afterwards
	// has room to move without clamping at the top.
	frame(c, input.Wheel{X: 0, Y: 0, DY: 5000})
	before := visibleViewBounds(v, viewport).Min.Y
	if before == 0 {
		t.Fatal("test setup: wheel didn't scroll down")
	}

	frame(c, input.Wheel{X: -1000, Y: -1000, DY: -37}) // outside bounds
	if got := visibleViewBounds(v, viewport).Min.Y; got != before {
		t.Errorf("VisibleViewBounds top after an out-of-bounds wheel = %d, want unchanged %d", got, before)
	}

	frame(c, input.Wheel{X: 0, Y: 0, DY: -37}) // inside bounds
	if got := visibleViewBounds(v, viewport).Min.Y; got != before-37 {
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
	if got := visibleViewBounds(v, viewport).Min.Y; got != 0 {
		t.Fatalf("VisibleViewBounds top after a frame with no fling = %d, want 0", got)
	}

	now := fling(c, -60, 100*time.Millisecond) // -60px over 100ms = -600px/s

	before := visibleViewBounds(v, viewport).Min.Y
	now += 100 * time.Millisecond
	c.Frame(nil, now)
	if got := visibleViewBounds(v, viewport).Min.Y; got <= before {
		t.Errorf("VisibleViewBounds top after coasting = %d, want more than %d (coasting forward)", got, before)
	}

	// The app moving the View stops the fling.
	v.ScrollBy(10)
	before = visibleViewBounds(v, viewport).Min.Y
	now += 100 * time.Millisecond
	c.Frame(nil, now)
	if got := visibleViewBounds(v, viewport).Min.Y; got != before {
		t.Errorf("VisibleViewBounds top after coasting following View.ScrollBy = %d, want unchanged %d", got, before)
	}
}

// TestControllerSetViewStopsFling checks a fling doesn't carry over to a
// new View.
func TestControllerSetViewStopsFling(t *testing.T) {
	c, _ := newTestController(t, strings.Repeat(longDoc, 20))
	now := fling(c, -60, 100*time.Millisecond)

	v2 := NewView(Parse([]byte(strings.Repeat(longDoc, 20))), fonts.NewGoSelector(), stylingtest.Basic())
	layoutView(v2, testWidth, testHeight, 1, now)
	c.SetView(v2)
	c.Frame(nil, now+100*time.Millisecond)
	if got := visibleViewBounds(v2, image.Pt(testWidth, testHeight)).Min.Y; got != 0 {
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
	c := NewController(&View{})
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
	if got := visibleViewBounds(v, viewport).Min.Y; got != 0 {
		t.Errorf("VisibleViewBounds top = %d after coasting a below-minimum velocity, want 0 (no scroll)", got)
	}
}
