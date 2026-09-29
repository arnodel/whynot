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
)

// TestPanelTouchFlingKeepsScrollingAfterRelease checks that a touch fling
// keeps scrolling once the finger lifts: Gio only produces frames when
// something happens, so the Panel must request frames itself while
// there's momentum left, and apply it on each.
func TestPanelTouchFlingKeepsScrollingAfterRelease(t *testing.T) {
	doc := whynot.Parse([]byte(strings.Repeat("A paragraph of filler text to scroll through.\n\n", 400)))
	bounds := image.Rect(0, 0, 400, 300)
	panel := NewPanel(whynot.NewView(doc, whynot.NewGoFontFaceSelector(72)), New(), bounds)

	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now()}
		panel.Update(gtx)
		router.Frame(&ops)
	}
	top := func() int {
		return panel.View().VisibleViewBounds(bounds.Size()).Min.Y
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
		t.Fatalf("drag didn't scroll: top = %d", released)
	}
	for range 5 {
		time.Sleep(16 * time.Millisecond)
		frame()
	}
	if got := top(); got <= released {
		t.Errorf("top = %d after 5 frames following release, want more than %d (still coasting)", got, released)
	}
}
