package giorenderer

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// testApp drives a whynot.Panel with Gio's input the way an app does,
// through a router standing in for the window.
type testApp struct {
	panel    *whynot.Panel
	input    Input
	renderer *Renderer
	router   input.Router
	ops      op.Ops
	start    time.Time
}

func newTestApp(source string, bounds image.Rectangle) *testApp {
	view := whynot.NewView(whynot.Parse([]byte(source)), fonts.NewGoSelector(), simpletheme.DarkStyleSheet)
	return &testApp{panel: whynot.NewPanel(view, bounds), renderer: New(), start: time.Now()}
}

// frame runs one frame: input, then drawing.
func (a *testApp) frame() {
	a.ops.Reset()
	gtx := layout.Context{Ops: &a.ops, Source: a.router.Source(), Now: time.Now()}
	now := time.Since(a.start)
	a.panel.Frame(a.input.Source(gtx, a.panel, a.panel.Bounds()).Events(), now)
	if a.panel.Animating() {
		gtx.Execute(op.InvalidateCmd{})
	}
	a.panel.Draw(a.renderer.NewCanvas(gtx.Ops, a.panel.Bounds()), now)
	a.router.Frame(&a.ops)
}

// TestTouchFlingKeepsScrollingAfterRelease checks that a touch fling
// keeps scrolling once the finger lifts: Gio only produces frames when
// something happens, so the app must request frames while the Panel is
// animating.
func TestTouchFlingKeepsScrollingAfterRelease(t *testing.T) {
	bounds := image.Rect(0, 0, 400, 300)
	app := newTestApp(strings.Repeat("A paragraph of filler text to scroll through.\n\n", 400), bounds)
	frame, router := app.frame, &app.router
	top := func() float64 {
		start, _ := app.panel.View().VisibleRange()
		return start
	}
	touch := func(kind pointer.Kind, y float32) {
		router.Queue(pointer.Event{Kind: kind, Source: pointer.Touch, Position: f32.Pt(200, y)})
	}

	frame() // registers the Panel's input area
	touch(pointer.Press, 250)
	frame()
	for y := float32(230); y >= 90; y -= 20 { // a fast upward flick
		touch(pointer.Move, y)
		frame()
		time.Sleep(10 * time.Millisecond)
	}
	touch(pointer.Release, 90)
	frame()

	if _, ok := router.WakeupTime(); !ok {
		t.Fatal("no frame requested after releasing a fling")
	}
	released := top()
	if released <= 0 {
		t.Fatalf("drag didn't scroll: top = %v", released)
	}
	for range 5 {
		time.Sleep(16 * time.Millisecond)
		frame()
	}
	if got := top(); got <= released {
		t.Errorf("top = %v after 5 frames following release, want more than %v (still coasting)", got, released)
	}
}

// TestTouchLeavesNoHover checks that a tap doesn't leave anything
// hovered once the finger lifts. There's no hover on touch, and Gio sends
// no further pointer events to move it off again - so a tapped link would
// stay highlighted, and a panned code block keep its scrollbar showing.
func TestTouchLeavesNoHover(t *testing.T) {
	app := newTestApp(strings.Repeat("[a link](#nowhere) ", 20), image.Rect(0, 0, 400, 300))
	frame, router, panel := app.frame, &app.router, app.panel
	var hovered []string
	panel.OnLinkHover = func(dest string) { hovered = append(hovered, dest) }
	var link f32.Point
	for y := 0; y < 100 && link == (f32.Point{}); y++ {
		for x := 0; x < 200; x++ {
			if _, ok := panel.View().LinkAt(x, y); ok {
				link = f32.Pt(float32(x), float32(y))
				break
			}
		}
	}
	if link == (f32.Point{}) {
		t.Fatal("test setup: no link found")
	}

	frame()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Touch, Position: link})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Touch, Position: link})
	frame()

	if len(hovered) == 0 || hovered[len(hovered)-1] != "" {
		t.Errorf("OnLinkHover calls = %q, want the last one to clear the hover (\"\")", hovered)
	}
}
