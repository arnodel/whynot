package whynot

import (
	"image"
	"strings"
	"testing"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/internal/styling"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
)

// scrollbarView returns a View of source with its scrollbar on, laid out
// at testWidth x testHeight, and a Controller for it.
func scrollbarView(t *testing.T, source string, opts ...ViewOption) (*View, *Controller) {
	t.Helper()
	v := NewView(Parse([]byte(source)), fonts.NewGoSelector(), stylingtest.Basic(), append([]ViewOption{WithScrollbar()}, opts...)...)
	v.Layout(testWidth, testHeight, 1, 0)
	return v, NewController(v, image.Rect(0, 0, testWidth, testHeight))
}

func TestScrollbarNoThumbWhenItFitsOrIsOff(t *testing.T) {
	v, _ := scrollbarView(t, "just one short line")
	if _, ok := v.scrollbarThumb(); ok {
		t.Error("thumb shown for a document that fits")
	}
	v, _ = scrollbarView(t, strings.Repeat(longDoc, 20))
	v.SetScrollbar(false)
	if _, ok := v.scrollbarThumb(); ok {
		t.Error("thumb shown with the scrollbar off")
	}
}

func TestScrollbarThumbTracksPosition(t *testing.T) {
	v, _ := scrollbarView(t, strings.Repeat(longDoc, 20))
	bar := v.scrollbarBar()

	r, ok := v.scrollbarThumb()
	if !ok {
		t.Fatal("no thumb for a document taller than the View")
	}
	if r.Max.X != testWidth-bar.inset || r.Dx() != bar.thickness {
		t.Errorf("thumb %v, want %d wide, %d from the right edge", r, bar.thickness, bar.inset)
	}
	if r.Min.Y != 0 {
		t.Errorf("thumb top at the start = %d, want 0", r.Min.Y)
	}
	if r.Dy() < bar.minThumb {
		t.Errorf("thumb length %d, want at least %d", r.Dy(), bar.minThumb)
	}

	v.ScrollToRatio(1)
	if r, _ := v.scrollbarThumb(); r.Max.Y != testHeight {
		t.Errorf("thumb bottom at the end = %d, want %d", r.Max.Y, testHeight)
	}
}

func TestScrollbarDrag(t *testing.T) {
	v, c := scrollbarView(t, strings.Repeat(longDoc, 20))
	viewport := image.Pt(testWidth, testHeight)
	thumb, _ := v.scrollbarThumb()
	grab := image.Pt(thumb.Min.X+1, thumb.Min.Y+thumb.Dy()/2)

	// A press away from the scrollbar doesn't drag it.
	frame(c, press(10, grab.Y))
	if v.vbar.dragging {
		t.Error("a press away from the scrollbar started a drag")
	}
	frame(c, release(10, grab.Y))

	frame(c, press(grab.X, grab.Y))
	if !v.vbar.dragging {
		t.Fatal("a press on the thumb didn't start a drag")
	}
	before := v.VisibleViewBounds(viewport).Min.Y
	// Dragging keeps tracking the pointer even off the scrollbar.
	frame(c, input.PointerMove{X: grab.X - 100, Y: grab.Y + testHeight/2})
	if after := v.VisibleViewBounds(viewport).Min.Y; after <= before {
		t.Errorf("page top after dragging the thumb down = %d, want more than %d", after, before)
	}
	frame(c, release(grab.X-100, grab.Y+testHeight/2))
	if v.vbar.dragging {
		t.Error("still dragging after the release")
	}
}

func TestScrollbarFades(t *testing.T) {
	v, _ := scrollbarView(t, strings.Repeat(longDoc, 20))
	if got := v.scrollbarOpacity(v.ctx.Time); got != 0 {
		t.Errorf("opacity before any scrolling = %v, want 0", got)
	}
	v.ScrollBy(50)
	if got := v.scrollbarOpacity(v.ctx.Time); got != 1 {
		t.Errorf("opacity right after scrolling = %v, want 1", got)
	}
	later := v.ctx.Time + hscrollRevealHold + hscrollRevealFade
	if got := v.scrollbarOpacity(later); got != 0 {
		t.Errorf("opacity once faded = %v, want 0", got)
	}
	v.vbar.hovered = true
	if got := v.scrollbarOpacity(later); got != 1 {
		t.Errorf("opacity while hovered = %v, want 1", got)
	}
}

func TestScrollbarAlwaysVisible(t *testing.T) {
	style := stylingtest.Basic()
	style.Scrollbar.ScrollbarGeometry = styling.ScrollbarGeometry{Thickness: 6, Inset: 2, MinThumbLength: 24, AlwaysVisible: true}
	v := NewView(Parse([]byte(strings.Repeat(longDoc, 20))), fonts.NewGoSelector(), style, WithScrollbar())
	v.Layout(testWidth, testHeight, 1, 0)
	if got := v.scrollbarOpacity(v.ctx.Time); got != 1 {
		t.Errorf("opacity of an always-visible scrollbar = %v, want 1", got)
	}
}

// TestScrollbarHiddenDoesNotTakeTouches checks a touch along the right
// edge scrolls the page while the scrollbar is hidden, rather than
// dragging it.
func TestScrollbarHiddenDoesNotTakeTouches(t *testing.T) {
	v, c := scrollbarView(t, strings.Repeat(longDoc, 20))
	thumb, _ := v.scrollbarThumb()
	x := thumb.Min.X + 1
	c.Frame([]input.Event{input.TouchStart{ID: 1, X: x, Y: 50}}, 0)
	if v.vbar.dragging {
		t.Error("a touch on the hidden scrollbar started a drag")
	}
}
