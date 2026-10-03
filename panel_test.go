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

// TestPanelHoverAndClick checks the Panel passes input to its link
// callbacks, in the coordinates of its bounds.
func TestPanelHoverAndClick(t *testing.T) {
	// Away from the origin, to catch a coordinate-translation bug.
	bounds := image.Rect(50, 30, 50+testWidth, 30+testHeight)
	p := newTestPanel("A paragraph with [a link](dest) in it, and then some more text below it too.", bounds)
	lx, ly := findLinkPos(t, p.View(), bounds.Dx(), bounds.Dy())
	x, y := bounds.Min.X+lx, bounds.Min.Y+ly

	var hovered []string
	p.OnLinkHover = func(dest string) { hovered = append(hovered, dest) }
	var clicked []string
	p.OnLinkClick = func(dest string) { clicked = append(clicked, dest) }

	p.Frame([]input.Event{input.PointerMove{X: lx, Y: ly}}, 0)
	if len(hovered) != 0 {
		t.Errorf("hovered %q with the pointer outside the bounds, want nothing", hovered)
	}
	p.Frame([]input.Event{input.PointerMove{X: x, Y: y}}, 0)
	if len(hovered) != 1 || hovered[0] != "dest" {
		t.Errorf("hovered %q over the link, want [dest]", hovered)
	}
	p.Frame([]input.Event{press(x, y), release(x, y)}, 0)
	if len(clicked) != 1 || clicked[0] != "dest" {
		t.Errorf("clicked %q, want [dest]", clicked)
	}
}

func TestPanelAnchorScrolling(t *testing.T) {
	// The link is at the top and its heading far below.
	doc := "[jump](#target)\n\n" + strings.Repeat(longDoc, 10) + "# Target"
	p := newTestPanel(doc, image.Rect(0, 0, testWidth, testHeight))
	p.AnchorScrolling = true
	x, y := findLinkPos(t, p.View(), testWidth, testHeight)
	var clicked bool
	p.OnLinkClick = func(string) { clicked = true }

	viewport := image.Pt(testWidth, testHeight)
	before := p.View().VisibleViewBounds(viewport).Min.Y
	p.Frame([]input.Event{press(x, y), release(x, y)}, 0)
	if clicked {
		t.Error("OnLinkClick called for a #fragment link with AnchorScrolling")
	}
	if after := p.View().VisibleViewBounds(viewport).Min.Y; after <= before {
		t.Errorf("page top after clicking the anchor link = %d, want more than %d", after, before)
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
	top := func() int { return p.View().VisibleViewBounds(image.Pt(testWidth, testHeight)).Min.Y }
	page := float64(testHeight) * (1 - pageOverlapFrac)

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

// TestPanelScrollLeftRight checks ScrollLeft and ScrollRight scroll the
// wide block under the pointer, and only that.
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
	above := bounds.Min.Add(area.Visible.Min).Add(image.Pt(10, -5))
	p.Frame([]input.Event{input.PointerMove{X: above.X, Y: above.Y}}, 0)
	if p.ScrollRight() {
		t.Error("ScrollRight with the pointer above the code block = true, want false")
	}

	on := bounds.Min.Add(area.Visible.Min).Add(image.Pt(10, 10))
	p.Frame([]input.Event{input.PointerMove{X: on.X, Y: on.Y}}, 0)
	if !p.ScrollRight() {
		t.Fatal("ScrollRight with the pointer on the code block = false, want true")
	}
	if got := drawnTextX(t, v, "wide"); got != before-arrowScrollLines {
		t.Errorf("text at x=%d after ScrollRight, want %d", got, before-arrowScrollLines)
	}
	p.ScrollLeft()
	if got := drawnTextX(t, v, "wide"); got != before {
		t.Errorf("text at x=%d after ScrollLeft, want %d (back at the start)", got, before)
	}
}
