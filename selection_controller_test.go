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
	// Between words: still text.
	gap := wordAt(t, v, "and", 0, -3)
	frame(c, move(gap))
	if v.Cursor() != CursorText {
		t.Errorf("between words, the cursor is %v", v.Cursor())
	}
	// Dragging a selection from the link: text, even over the link or
	// nothing, until the release.
	l := wordAt(t, v, "link", 0, 2)
	frame(c, move(l), press(l.X, l.Y), move(image.Pt(l.X+20, l.Y)))
	if v.Cursor() != CursorText {
		t.Errorf("dragging from a link, the cursor is %v", v.Cursor())
	}
	frame(c, move(image.Pt(b.Max.X-2, b.Max.Y-2)))
	if v.Cursor() != CursorText {
		t.Errorf("dragging over nothing, the cursor is %v", v.Cursor())
	}
	frame(c, move(l), release(l.X, l.Y))
	if v.Cursor() != CursorLink {
		t.Errorf("released over the link, the cursor is %v", v.Cursor())
	}
}

// TestUnselectable checks a Controller whose mouse doesn't select: drags
// and multiple presses select nothing, a click on a link still follows
// it, the cursor over text is the default one, and turning it off clears
// the selection.
func TestUnselectable(t *testing.T) {
	c, v := newTestController(t, "Hello brave new world, see [here](dest).\n")
	v.SelectAll()
	c.SetSelectable(false)
	if v.HasSelection() {
		t.Error("turning selection off left a selection")
	}
	from, to := wordAt(t, v, "Hello", 0, 0), wordAt(t, v, "new", 0, 3)
	frame(c, move(from), press(from.X, from.Y), move(to), release(to.X, to.Y))
	p := wordAt(t, v, "brave", 0, 2)
	c.Frame([]input.Event{press(p.X, p.Y), release(p.X, p.Y)}, 0)
	c.Frame([]input.Event{press(p.X, p.Y), release(p.X, p.Y)}, time.Millisecond)
	if v.HasSelection() {
		t.Errorf("with selection off, the mouse selected %q", v.SelectedText())
	}
	frame(c, move(p))
	if v.Cursor() != CursorDefault {
		t.Errorf("with selection off, the cursor over text is %v", v.Cursor())
	}
	link := wordAt(t, v, "here", 0, 2)
	events := frame(c, move(link), press(link.X, link.Y), release(link.X, link.Y))
	if !hasEvent(events, LinkClick{Destination: "dest"}) {
		t.Errorf("with selection off, a click on a link gave %v", events)
	}
	v.SelectAll()
	if !v.HasSelection() {
		t.Error("with selection off, SelectAll selected nothing")
	}
}

// TestCarriedSelectionIsWhole checks that a selection carried on from
// another View is whole blocks even within one paragraph, as a selection
// leaving a paragraph is within a View.
func TestCarriedSelectionIsWhole(t *testing.T) {
	c, v := newTestController(t, "First paragraph here.\n\nLast paragraph here.\n")
	// Pressed in the last paragraph, carried on past the end.
	p := wordAt(t, v, "paragraph", 1, 2)
	q := wordAt(t, v, "here", 1, 2)
	frame(c, press(p.X, p.Y), move(q), release(q.X, q.Y))
	v.SelectToEnd()
	if got := v.SelectedText(); got != "Last paragraph here." {
		t.Errorf("carried on to the end: %q, want the whole last paragraph", got)
	}
	// Carried in from above, to a point in the first paragraph.
	r := wordAt(t, v, "paragraph", 0, 2)
	v.SelectFromStartTo(r.X, r.Y)
	if got := v.SelectedText(); got != "First paragraph here." {
		t.Errorf("carried in from the start: %q, want the whole first paragraph", got)
	}
	// A selection made within the View afterwards is a range again.
	frame(c, press(p.X, p.Y), move(q), release(q.X, q.Y))
	if got := v.SelectedText(); got == "Last paragraph here." || got == "" {
		t.Errorf("a drag within a paragraph after a carried selection: %q", got)
	}
}
