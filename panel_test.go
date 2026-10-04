package whynot

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/internal/canvastest"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
)

func newTestPanel(source string, bounds image.Rectangle) *Panel {
	v := NewView(Parse([]byte(source)), fonts.NewGoSelector(), stylingtest.Basic())
	return NewPanel(v, bounds)
}

// TestPanelHoverAndClick checks the Panel hovers and reports links, in
// the coordinates of its bounds.
func TestPanelHoverAndClick(t *testing.T) {
	// Away from the origin, to catch a coordinate-translation bug.
	bounds := image.Rect(50, 30, 50+testWidth, 30+testHeight)
	p := newTestPanel("A paragraph with [a link](dest) in it, and then some more text below it too.", bounds)
	x, y := findLinkPos(t, p.View())

	p.Frame([]input.Event{input.PointerMove{X: 0, Y: 0}}, 0)
	if got := hovered(p.View()); got != "" {
		t.Errorf("hovered %q with the pointer outside the bounds, want nothing", got)
	}
	p.Frame([]input.Event{input.PointerMove{X: x, Y: y}}, 0)
	if got := hovered(p.View()); got != "dest" {
		t.Errorf("hovered %q over the link, want dest", got)
	}
	events := p.Frame([]input.Event{press(x, y), release(x, y)}, 0)
	if len(events) != 1 || events[0] != (LinkClick{Destination: "dest"}) {
		t.Errorf("events for a click on the link = %v, want [LinkClick{dest}]", events)
	}
}

// TestPanelAnchorScrolling checks a Panel scrolls to a "#id" link's
// heading by default, still reporting the AnchorClick, and that
// SetAnchorScrolling(false) leaves it to the app.
func TestPanelAnchorScrolling(t *testing.T) {
	// The link is at the top and its heading far below.
	doc := "[jump](#target)\n\n" + strings.Repeat(longDoc, 10) + "# Target"
	for _, on := range []bool{true, false} {
		p := newTestPanel(doc, image.Rect(0, 0, testWidth, testHeight))
		p.SetAnchorScrolling(on)
		x, y := findLinkPos(t, p.View())

		events := p.Frame([]input.Event{press(x, y), release(x, y)}, 0)
		if len(events) != 1 || events[0] != (AnchorClick{ID: "target"}) {
			t.Errorf("anchor scrolling %v: events = %v, want [AnchorClick{target}]", on, events)
		}
		if start, _ := p.View().VisibleRange(); (start > 0) != on {
			t.Errorf("anchor scrolling %v: VisibleRange start = %v after the click", on, start)
		}
	}
}

// TestPanelAnchorTop checks a bare "#" link, and "#top" when no heading
// has that id, scroll to the top, as in a browser, while a "Top" heading
// takes precedence.
func TestPanelAnchorTop(t *testing.T) {
	click := func(t *testing.T, doc string) (start float64) {
		t.Helper()
		p := newTestPanel(doc, image.Rect(0, 0, testWidth, testHeight))
		p.View().ScrollBy(5) // off the top, with the link still on screen
		x, y := findLinkPos(t, p.View())
		p.Frame([]input.Event{press(x, y), release(x, y)}, 0)
		start, _ = p.View().VisibleRange()
		return start
	}
	filler := strings.Repeat(longDoc, 10)
	for _, dest := range []string{"#", "#top", "#Top"} {
		if start := click(t, "[up]("+dest+")\n\n"+filler); start != 0 {
			t.Errorf("after clicking a %q link, VisibleRange start = %v, want 0", dest, start)
		}
	}
	if start := click(t, "[up](#top)\n\n"+filler+"# Top\n\n"+filler); start == 0 {
		t.Error(`a "#top" link went to the top of the document, want the "Top" heading`)
	}
}

// TestPanelSetViewKeepsSettings checks a new View gets the Panel's
// StyleSheet and scrollbar, and is laid out.
func TestPanelSetViewKeepsSettings(t *testing.T) {
	c := color.RGBA{1, 2, 3, 4}
	style := stylingtest.Basic()
	style.Scrollbar.Idle = c

	p := newTestPanel(strings.Repeat(longDoc, 20), image.Rect(0, 0, testWidth, testHeight))
	p.SetStyleSheet(style)
	p.SetScrollbar(true)
	v2 := NewView(Parse([]byte(strings.Repeat(longDoc, 20))), fonts.NewGoSelector(), stylingtest.Basic())
	p.SetView(v2)

	if p.View() != v2 {
		t.Error("View() after SetView isn't the new View")
	}
	if got := v2.ScrollbarColor(false, false); got != c {
		t.Errorf("new View's scrollbar color = %v, want %v (StyleSheet not re-applied)", got, c)
	}
	if _, ok := v2.scrollbarThumb(); !ok {
		t.Error("new View has no scrollbar thumb (scrollbar not re-applied, or not laid out)")
	}
}

// TestPanelDrawClipsToBounds checks the View is drawn at the Panel's
// position, and only within its bounds.
func TestPanelDrawClipsToBounds(t *testing.T) {
	bounds := image.Rect(50, 30, 50+testWidth, 30+testHeight)
	p := newTestPanel(strings.Repeat(longDoc, 20), bounds)
	dst := &canvastest.Recorder{Area: image.Rect(0, 0, 1000, 1000)}
	p.Draw(dst, 0)

	if len(dst.Clips) == 0 || dst.Clips[0] != bounds {
		t.Errorf("first clip = %v, want the Panel's bounds %v", dst.Clips, bounds)
	}
	if len(dst.Texts) == 0 {
		t.Fatal("nothing drawn")
	}
	if first := dst.Texts[0]; first.X < bounds.Min.X || first.Y < bounds.Min.Y {
		t.Errorf("first text at (%d, %d), want inside %v", first.X, first.Y, bounds)
	}
}

func TestPanelScrollAndPage(t *testing.T) {
	p := newTestPanel(strings.Repeat(longDoc, 40), image.Rect(0, 0, testWidth, testHeight))
	top := func() int { return stackVisibleBounds(p.View(), image.Pt(testWidth, testHeight)).Min.Y }
	page := float64(testHeight) * (1 - pageOverlap)

	steps := []struct {
		name     string
		do       func()
		min, max float64 // expected movement
	}{
		{"ScrollDown", p.ScrollDown, 1, page / 2},
		{"ScrollUp", p.ScrollUp, -page / 2, -1},
		// Loose bounds: layout makes them inexact, but a page is
		// page-sized, not line-sized.
		{"PageDown", p.PageDown, page / 2, page * 2},
		{"PageUp", p.PageUp, -page * 2, -page / 2},
	}
	for _, s := range steps {
		before := top()
		s.do()
		if d := float64(top() - before); d < s.min || d > s.max {
			t.Errorf("%s moved %v, want between %v and %v", s.name, d, s.min, s.max)
		}
	}
}

// TestPanelZoomedScrollStep checks the View is laid out at the scale
// times the zoom, while ScrollDown's step follows the scale alone.
func TestPanelZoomedScrollStep(t *testing.T) {
	p := newTestPanel(strings.Repeat(longDoc, 40), image.Rect(0, 0, testWidth, testHeight))
	p.SetScale(2)
	p.SetZoom(1.5)
	v := p.View()
	if v.Scale() != 2 || v.Zoom() != 1.5 || v.ctx.Scale != 3 {
		t.Errorf("View's scale, zoom and layout scale = %v, %v, %v; want 2, 1.5, 3", v.Scale(), v.Zoom(), v.ctx.Scale)
	}
	top := func() int { return stackVisibleBounds(p.View(), image.Pt(testWidth, testHeight)).Min.Y }
	before := top()
	p.ScrollDown()
	if d := top() - before; d != 2*commandStep {
		t.Errorf("ScrollDown moved %d, want %d", d, 2*commandStep)
	}
}

// TestPanelScrollLeftRight checks ScrollLeft and ScrollRight scroll the
// wide block under the pointer, else the one last scrolled.
func TestPanelScrollLeftRight(t *testing.T) {
	source := "Intro.\n\n```\n" + strings.Repeat("wide ", 100) + "\nshort\n```\n\n" +
		strings.Repeat("Filler paragraph.\n\n", 60)
	bounds := image.Rect(50, 30, 350, 430)
	v := NewView(Parse([]byte(source)), fonts.NewGoSelector(), stylingtest.NoViewMargin())
	p := NewPanel(v, bounds)
	p.Draw(&canvastest.Recorder{Area: image.Rect(0, 0, 1000, 1000)}, 0)
	area := v.hscroll.regions[0]
	before := drawnTextX(t, v, "wide")

	if p.ScrollRight() {
		t.Error("ScrollRight with no pointer = true, want false")
	}
	above := area.Visible.Min.Add(image.Pt(10, -5))
	p.Frame([]input.Event{input.PointerMove{X: above.X, Y: above.Y}}, 0)
	if p.ScrollRight() {
		t.Error("ScrollRight with the pointer above the code block = true, want false")
	}

	on := area.Visible.Min.Add(image.Pt(10, 10))
	p.Frame([]input.Event{input.PointerMove{X: on.X, Y: on.Y}}, 0)
	if !p.ScrollRight() {
		t.Fatal("ScrollRight with the pointer on the code block = false, want true")
	}
	if got := drawnTextX(t, v, "wide"); got != before-commandStep {
		t.Errorf("text at x=%d after ScrollRight, want %d", got, before-commandStep)
	}
	p.ScrollLeft()
	if got := drawnTextX(t, v, "wide"); got != before {
		t.Errorf("text at x=%d after ScrollLeft, want %d (back at the start)", got, before)
	}

	// With the pointer elsewhere, the block last scrolled is the target.
	p.Frame([]input.Event{input.PointerMove{X: above.X, Y: above.Y}}, 0)
	if !p.ScrollRight() {
		t.Fatal("ScrollRight after scrolling the code block = false, want true")
	}
	if got := drawnTextX(t, v, "wide"); got != before-commandStep {
		t.Errorf("text at x=%d after ScrollRight, want %d", got, before-commandStep)
	}
	// Until the View is replaced.
	p.SetView(v)
	if p.ScrollRight() {
		t.Error("ScrollRight after SetView = true, want false")
	}
}

// TestControllerSidewaysTargetTouched checks a block touched becomes the
// target of ScrollRight, even with no pointer.
func TestControllerSidewaysTargetTouched(t *testing.T) {
	source := "Intro.\n\n```\n" + strings.Repeat("wide ", 100) + "\nshort\n```\n\n" +
		strings.Repeat("Filler paragraph.\n\n", 60)
	bounds := image.Rect(0, 0, 300, 400)
	v := NewView(Parse([]byte(source)), fonts.NewGoSelector(), stylingtest.NoViewMargin())
	p := NewPanel(v, bounds)
	p.Draw(&canvastest.Recorder{Area: image.Rect(0, 0, 1000, 1000)}, 0)
	on := v.hscroll.regions[0].Visible.Min.Add(image.Pt(10, 10))
	before := drawnTextX(t, v, "wide")

	p.Frame([]input.Event{input.TouchStart{X: on.X, Y: on.Y}, input.TouchEnd{}}, 0)
	if !p.ScrollRight() {
		t.Fatal("ScrollRight after touching the code block = false, want true")
	}
	if got := drawnTextX(t, v, "wide"); got != before-commandStep {
		t.Errorf("text at x=%d after ScrollRight, want %d", got, before-commandStep)
	}
}
