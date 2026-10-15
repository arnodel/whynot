package whynot

import (
	"image"
	"testing"
	"time"

	"github.com/arnodel/whynot/canvas/canvastest"
	"github.com/arnodel/whynot/input"
)

// wordAt returns a point inside the nth drawing (from 0) of the text s,
// offset dx pixels from its start.
func wordAt(t *testing.T, v *View, s string, nth, dx int) image.Point {
	t.Helper()
	r := &canvastest.Recorder{Area: v.Bounds()}
	v.Draw(r, 0)
	for _, tx := range r.Texts {
		if tx.S == s {
			if nth == 0 {
				return image.Pt(tx.X+dx, tx.Y-4)
			}
			nth--
		}
	}
	t.Fatalf("no text %q drawn", s)
	return image.Point{}
}

func move(p image.Point) input.Event { return input.PointerMove{X: p.X, Y: p.Y} }

func hasEvent(events []Event, want Event) bool {
	for _, e := range events {
		if e == want {
			return true
		}
	}
	return false
}

// TestDragSelects checks that pressing on text and dragging selects, and
// that a click elsewhere clears it.
func TestDragSelects(t *testing.T) {
	c, v := newTestController(t, "Hello brave new world.\n\nAnother paragraph here.\n")
	from, to := wordAt(t, v, "brave", 0, 0), wordAt(t, v, "new", 0, 1000)
	to.X = wordAt(t, v, "world", 0, 0).X - 2 // the gap after "new"
	events := frame(c, move(from), press(from.X, from.Y), move(to), release(to.X, to.Y))
	if !hasEvent(events, SelectionChange{}) {
		t.Errorf("events for a drag = %v, want a SelectionChange", events)
	}
	if got := v.SelectedText(); got != "brave new" {
		t.Errorf("dragged over %q, want \"brave new\"", got)
	}
	// A click on text clears it.
	p := wordAt(t, v, "paragraph", 0, 3)
	events = frame(c, press(p.X, p.Y), release(p.X, p.Y))
	if v.HasSelection() || !hasEvent(events, SelectionChange{}) {
		t.Errorf("after a click, selection %q, events %v", v.SelectedText(), events)
	}
	// A drag across paragraphs selects both whole.
	frame(c, press(from.X, from.Y), move(p), release(p.X, p.Y))
	if got, want := v.SelectedText(), "Hello brave new world.\n\nAnother paragraph here."; got != want {
		t.Errorf("dragged across paragraphs: %q, want %q", got, want)
	}
}

// TestMultiClickSelects checks double and triple presses, and Shift
// extending.
func TestMultiClickSelects(t *testing.T) {
	c, v := newTestController(t, "Hello brave new world.\n\nAnother paragraph here.\n")
	p := wordAt(t, v, "brave", 0, 3)
	at := func(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }
	c.Frame([]input.Event{press(p.X, p.Y), release(p.X, p.Y)}, at(0))
	c.Frame([]input.Event{press(p.X, p.Y), release(p.X, p.Y)}, at(100))
	if got := v.SelectedText(); got != "brave" {
		t.Errorf("double press: %q, want \"brave\"", got)
	}
	c.Frame([]input.Event{press(p.X, p.Y), release(p.X, p.Y)}, at(200))
	if got := v.SelectedText(); got != "Hello brave new world." {
		t.Errorf("triple press: %q, want the paragraph", got)
	}
	// A slow press starts over: a click, clearing it.
	c.Frame([]input.Event{press(p.X, p.Y), release(p.X, p.Y)}, at(2000))
	if v.HasSelection() {
		t.Errorf("a slow press left %q selected", v.SelectedText())
	}
	// Shift and a press move the selection's end there, keeping its
	// start: from a double press on "brave" to the end of "world".
	c.Frame([]input.Event{press(p.X, p.Y), release(p.X, p.Y)}, at(3000))
	c.Frame([]input.Event{press(p.X, p.Y), release(p.X, p.Y)}, at(3100))
	q := wordAt(t, v, ".", 0, 0)
	c.Frame([]input.Event{input.PointerButton{X: q.X, Y: q.Y, Button: input.ButtonPrimary, Down: true, Mods: input.ModShift}, release(q.X, q.Y)}, at(4000))
	if got := v.SelectedText(); got != "brave new world" {
		t.Errorf("Shift and a press: %q, want \"brave new world\"", got)
	}
}

// TestDragFromLink checks that a drag starting on a link selects instead
// of following it.
func TestDragFromLink(t *testing.T) {
	c, v := newTestController(t, "See [the docs](http://e.com) now and later.\n")
	from := wordAt(t, v, "docs", 0, 1)
	to := wordAt(t, v, "later", 0, 3)
	events := frame(c, move(from), press(from.X, from.Y), move(to), release(to.X, to.Y))
	for _, e := range events {
		if _, ok := e.(LinkClick); ok {
			t.Errorf("a drag from a link followed it: %v", events)
		}
	}
	if !v.HasSelection() {
		t.Error("a drag from a link selected nothing")
	}
}

// TestCursor checks the cursor over text, a link and nothing.
func TestCursor(t *testing.T) {
	c, v := newTestController(t, "Some text and [a link](x).\n")
	frame(c, move(wordAt(t, v, "text", 0, 2)))
	if v.Cursor() != CursorText {
		t.Errorf("over text, the cursor is %v", v.Cursor())
	}
	frame(c, move(wordAt(t, v, "link", 0, 2)))
	if v.Cursor() != CursorLink {
		t.Errorf("over a link, the cursor is %v", v.Cursor())
	}
	b := v.Bounds()
	frame(c, move(image.Pt(b.Max.X-2, b.Max.Y-2)))
	if v.Cursor() != CursorDefault {
		t.Errorf("over nothing, the cursor is %v", v.Cursor())
	}
}
