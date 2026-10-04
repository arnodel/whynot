package whynot

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/arnodel/whynot/internal/canvastest"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
)

// TestViewBoundsShiftEverything checks that moving a View's bounds moves
// all of it on the canvas: what it draws, what HitTest and LinkAt find,
// and its scrollbar, which a Controller drags in the same coordinates.
func TestViewBoundsShiftEverything(t *testing.T) {
	source := "A paragraph with [a link](dest) in it.\n\n" + strings.Repeat(longDoc, 20)
	d := image.Pt(50, 30)
	views := make([]*View, 2)
	for i, origin := range []image.Point{{}, d} {
		views[i] = NewView(Parse([]byte(source)), WithStyleSheet(stylingtest.Basic()), WithScrollbar())
		views[i].SetBounds(image.Rectangle{Min: origin, Max: origin.Add(image.Pt(testWidth, testHeight))})
	}
	at, moved := views[0], views[1]

	// Drawing.
	draw := func(v *View) *canvastest.Recorder {
		dst := &canvastest.Recorder{Area: image.Rect(0, 0, 1000, 1000)}
		v.Draw(dst, 0)
		return dst
	}
	t0, t1 := draw(at).Texts, draw(moved).Texts
	if len(t0) == 0 || len(t0) != len(t1) {
		t.Fatalf("drew %d and %d texts, want the same, non-zero", len(t0), len(t1))
	}
	if t1[0].X != t0[0].X+d.X || t1[0].Y != t0[0].Y+d.Y {
		t.Errorf("first text at (%d, %d), want (%d, %d)", t1[0].X, t1[0].Y, t0[0].X+d.X, t0[0].Y+d.Y)
	}

	// HitTest and LinkAt.
	x, y := findLinkPos(t, at)
	r0, ok0 := at.HitTest(x, y)
	r1, ok1 := moved.HitTest(x+d.X, y+d.Y)
	if !ok0 || !ok1 || r1 != r0.Add(d) {
		t.Errorf("HitTest = %v, %v at the origin and %v, %v moved; want the rectangle moved by %v", r0, ok0, r1, ok1, d)
	}
	if dest, ok := moved.LinkAt(x+d.X, y+d.Y); !ok || dest != "dest" {
		t.Errorf("LinkAt moved = %q, %v; want \"dest\", true", dest, ok)
	}
	if r, ok := moved.HitTest(d.X-1, d.Y+10); ok {
		t.Errorf("HitTest left of the bounds = %v, true; want a miss", r)
	}

	// The scrollbar, and dragging it.
	b0, _ := at.scrollbarThumb()
	b1, ok := moved.scrollbarThumb()
	if !ok || b1 != b0.Add(d) {
		t.Errorf("scrollbar thumb moved = %v, %v; want %v", b1, ok, b0.Add(d))
	}
	c := NewController(moved)
	frame(c, press(b1.Min.X+1, b1.Min.Y+1))
	if !moved.vbar.dragging {
		t.Error("a press on the moved thumb didn't start a drag")
	}
}

// TestViewSetBoundsKeepsLayout checks that moving a View, or changing
// only its height, keeps its layout and scroll position, while a new
// width lays it out again.
func TestViewSetBoundsKeepsLayout(t *testing.T) {
	v := NewView(Parse([]byte(strings.Repeat(longDoc, 20))), WithStyleSheet(stylingtest.Basic()))
	v.SetBounds(image.Rect(0, 0, testWidth, testHeight))
	v.ScrollBy(200)
	box, pos := v.stack.box, v.ScrollPosition()

	v.SetBounds(image.Rect(40, 40, 40+testWidth, 40+testHeight/2))
	if v.stack.box != box || v.ScrollPosition() != pos {
		t.Error("moving the View, or changing its height, laid it out again")
	}
	v.SetBounds(image.Rect(40, 40, 40+testWidth/2, 40+testHeight))
	if v.stack.box == box {
		t.Error("a new width didn't lay the View out again")
	}
}

func TestViewVisibleRange(t *testing.T) {
	v := NewView(Parse([]byte(strings.Repeat(longDoc, 20))), WithStyleSheet(stylingtest.Basic()))
	if start, end := v.VisibleRange(); start != 0 || end != 1 {
		t.Errorf("VisibleRange before SetBounds = %v, %v; want 0, 1", start, end)
	}
	v.SetBounds(image.Rect(0, 0, testWidth, testHeight))
	// Lay out the whole document, so the height is exact rather than an
	// estimate: how good the estimate is depends on how much a frame's
	// time budget lays out ahead.
	v.stack.visibleRange(math.MaxInt32)

	start, end := v.VisibleRange()
	if start != 0 || end <= 0 || end >= 1 {
		t.Errorf("VisibleRange at the top = %v, %v; want 0 and a fraction", start, end)
	}
	visible := end - start
	for _, ratio := range []float64{0.5, 0.2, 0.8} {
		v.ScrollToRatio(ratio)
		start, end := v.VisibleRange()
		if math.Abs(start-ratio) > 1e-9 {
			t.Errorf("VisibleRange after ScrollToRatio(%v) starts at %v", ratio, start)
		}
		if math.Abs((end-start)-visible) > 1e-9 {
			t.Errorf("VisibleRange after ScrollToRatio(%v) spans %v, want about %v", ratio, end-start, visible)
		}
	}
}
