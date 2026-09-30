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
	"github.com/arnodel/whynot/styles/simpletheme"
)

// TestPanelTouchFlingKeepsScrollingAfterRelease checks that a touch fling
// keeps scrolling once the finger lifts: Gio only produces frames when
// something happens, so the Panel must request frames itself while
// there's momentum left, and apply it on each.
func TestPanelTouchFlingKeepsScrollingAfterRelease(t *testing.T) {
	doc := whynot.Parse([]byte(strings.Repeat("A paragraph of filler text to scroll through.\n\n", 400)))
	bounds := image.Rect(0, 0, 400, 300)
	panel := NewPanel(whynot.NewView(doc, whynot.NewGoFontFaceSelector(72), simpletheme.DarkStyleSheet), New(), bounds)

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

// TestPanelTouchLeavesNoHover checks that a tap doesn't leave anything
// hovered once the finger lifts. There's no hover on touch, and Gio sends
// no further pointer events to move it off again - so a tapped link would
// stay highlighted, and a panned code block keep its scrollbar showing.
func TestPanelTouchLeavesNoHover(t *testing.T) {
	doc := whynot.Parse([]byte(strings.Repeat("[a link](#nowhere) ", 20)))
	bounds := image.Rect(0, 0, 400, 300)
	panel := NewPanel(whynot.NewView(doc, whynot.NewGoFontFaceSelector(72), simpletheme.DarkStyleSheet), New(), bounds)
	var hovered []string
	panel.OnLinkHover = func(dest string) { hovered = append(hovered, dest) }

	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now()}
		panel.Update(gtx)
		panel.Draw(gtx)
		router.Frame(&ops)
	}
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
