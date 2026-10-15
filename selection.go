package whynot

import (
	"image"
	"math"
	"time"

	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/internal/engine"
)

// A selection is the View's state, like its scroll position: what the
// reader selected, between two places in the text. Within a paragraph,
// heading, table cell or code block, it's a range of characters; across
// them, it's whole blocks, as the reader would expect to copy: the list
// items from the one holding one end to the one holding the other, say,
// or whole table rows.

// selection is a View's selection, between anchor, where it started, and
// focus, where it ends, in tree, the selection tree of the View's blocks
// as of when it was made.
type selection struct {
	tree          *engine.SelTree
	treeVersion   int // the View's blocksVersion tree is of
	anchor, focus engine.Position
	active        bool

	// drawSpan is the selection as of the draw in progress, if drawing.
	drawSpan engine.Span
	drawing  bool
}

// selTree returns the selection tree of the View's blocks, carrying the
// selection over to it if the blocks have changed since, or clearing it
// if its ends are in blocks that were replaced.
func (v *View) selTree() *engine.SelTree {
	v.follow()
	if v.sel.tree != nil && v.sel.treeVersion == v.blocksVersion {
		return v.sel.tree
	}
	tree := engine.NewSelTree(v.blocks)
	if v.sel.active {
		a, ok1 := tree.Carry(v.sel.anchor)
		f, ok2 := tree.Carry(v.sel.focus)
		v.sel.anchor, v.sel.focus, v.sel.active = a, f, ok1 && ok2
	}
	v.sel.tree, v.sel.treeVersion = tree, v.blocksVersion
	return tree
}

// Cursor is what the pointer should look like over a View (see
// [View.Cursor]): a host maps it to its toolkit's cursors.
type Cursor int

const (
	CursorDefault Cursor = iota // an arrow
	CursorText                  // an I-beam, over text that can be selected
	CursorLink                  // a pointing hand, over a link
)

// Cursor returns what the pointer should look like where it last hovered
// (see [Controller.Frame]).
func (v *View) Cursor() Cursor {
	switch {
	case v.ctx.HighlightNode != nil:
		return CursorLink
	case v.overText && !v.unselectable:
		return CursorText
	}
	return CursorDefault
}

// textAt returns the Position in the text under (x, y), if there's text
// there.
func (v *View) textAt(x, y int) (engine.Position, bool) {
	hit, offset, _ := v.hitTest(x, y)
	if hit == nil {
		return engine.Position{}, false
	}
	item, ok := hit.Source().(engine.Inline)
	if !ok {
		return engine.Position{}, false
	}
	pos, ok := v.selTree().PositionOf(item, 0)
	if !ok {
		return engine.Position{}, false
	}
	if tb, isText := hit.(*engine.TextBox); isText {
		pos.Offset = tb.OffsetAt(x - offset.X)
	} else if b := hit.Bounds().Add(offset); x > (b.Min.X+b.Max.X)/2 {
		_, end := pos.Word()
		pos.Offset = end.Offset
	}
	return pos, true
}

// positionAt returns the Position in the text nearest (x, y): the text
// under it, or else the nearest word on its line, its end if it's to the
// left and its start if it's to the right, or else on the nearest line
// above or below.
func (v *View) positionAt(x, y int) (engine.Position, bool) {
	y = max(v.bounds.Min.Y, min(y, v.bounds.Max.Y-1))
	if p, ok := v.textAt(x, y); ok {
		return p, true
	}
	const step, reach = 4, 48
	for dy := 0; dy <= reach; dy += step {
		for _, py := range []int{y - dy, y + dy} {
			for px := x - step; px >= v.bounds.Min.X; px -= step {
				if p, ok := v.textAt(px, py); ok {
					_, end := p.Word()
					return end, true
				}
			}
			for px := x + step; px < v.bounds.Max.X; px += step {
				if p, ok := v.textAt(px, py); ok {
					start, _ := p.Word()
					return start, true
				}
			}
		}
	}
	return engine.Position{}, false
}

// selectedRange is the View's engine.Context.Selected: which bytes of
// item's text are selected, in the draw in progress.
func (v *View) selectedRange(item engine.Inline, textLen int) (start, end int, ok bool) {
	if !v.sel.drawing {
		return 0, 0, false
	}
	p, ok := v.sel.tree.PositionOf(item, 0)
	if !ok {
		return 0, 0, false
	}
	return v.sel.drawSpan.Contains(p.Leaf, p.Item, textLen)
}

// span returns the selection's normalized Span, if it selects anything.
func (v *View) span() (engine.Span, bool) {
	v.selTree()
	if !v.sel.active {
		return engine.Span{}, false
	}
	return engine.Normalize(v.sel.anchor, v.sel.focus)
}

// HasSelection reports whether anything is selected.
func (v *View) HasSelection() bool {
	_, ok := v.span()
	return ok
}

// SelectAll selects the whole document.
func (v *View) SelectAll() {
	tree := v.selTree()
	start, ok1 := tree.Start()
	end, ok2 := tree.End()
	v.setSelection(start, end, ok1 && ok2)
}

// ClearSelection selects nothing.
func (v *View) ClearSelection() {
	v.setSelection(engine.Position{}, engine.Position{}, false)
}

// setSelection sets the selection, between anchor and focus if active.
func (v *View) setSelection(anchor, focus engine.Position, active bool) {
	v.sel.anchor, v.sel.focus, v.sel.active = anchor, focus, active
}

// SelectedText returns the selection's plain text: the exact range within
// a block, or whole blocks separated by blank lines, list items by line
// breaks and keeping their markers, and table cells by tabs. It's empty if
// nothing is selected.
func (v *View) SelectedText() string {
	s, ok := v.span()
	if !ok {
		return ""
	}
	return s.Text()
}

// SelectedMarkdown returns the selection's Markdown: whole blocks as
// written; table rows, or cells of a row, as a table with its header; and
// within a block, the range widened to whole inline elements, such as
// emphasis or a link, so that their markup stays balanced. It's empty if
// nothing is selected.
func (v *View) SelectedMarkdown() string {
	s, ok := v.span()
	if !ok {
		return ""
	}
	return s.Markdown(v.doc.source())
}

// selectionState is what SelectionChange compares.
type selectionState struct {
	anchor, focus engine.Position
	active        bool
}

func (v *View) selectionState() selectionState {
	return selectionState{v.sel.anchor, v.sel.focus, v.sel.active}
}

// The mouse selects: a press on text and a drag select characters; a
// double press selects a word, and a triple one the block, and dragging
// then extends by words or blocks; Shift and a press extend the
// selection. A press and release without moving is a click: on a link,
// it follows it, and elsewhere it clears the selection.

const (
	// clickSlop is how far, in logical pixels, the pointer may move
	// between a press and its release for them to be a click, and between
	// presses for them to count as a double or triple one.
	clickSlop = 4
	// multiClickTime is how close presses must be to count together.
	multiClickTime = 500 * time.Millisecond
	// selectionScrollSpeed is how fast dragging past the View's top or
	// bottom scrolls, in logical pixels a second per pixel past it.
	selectionScrollSpeed = 12.0
)

// mousePress is the press of the primary button in progress.
type mousePress struct {
	down  bool
	at    image.Point
	moved bool
	// link is the link pressed, if onLink.
	link   string
	onLink bool
	// clicks counts presses in quick succession at the same place, and
	// last is the time of the latest; a drag selects by characters, words
	// or blocks for 1, 2 and 3 or more.
	clicks int
	last   time.Duration
	lastAt image.Point
	// from and to are the unit (character, word, block) pressed, if
	// anchored: a drag selects from it to the unit under the pointer.
	from, to engine.Position
	anchored bool
	// scrolled is when dragging past an edge last scrolled.
	scrolled time.Duration
}

// SetSelectable sets whether the mouse selects text, as it does by
// default. Off, pressing and dragging select nothing, and the View's
// Cursor is never CursorText; clicks on links still follow them, and the
// View's own methods, such as [View.SelectAll], still select. Turning it
// off clears the selection.
func (c *Controller) SetSelectable(on bool) {
	c.unselectable = !on
	c.view.unselectable = !on
	if !on {
		c.view.ClearSelection()
		c.view.overText = false
	}
}

// beginSelection starts a press at p.
func (c *Controller) beginSelection(p image.Point, mods input.Modifiers, now time.Duration) {
	m, v := &c.mouse, c.view
	slop := clickSlop * c.scale
	if m.clicks > 0 && now-m.last < multiClickTime && dist(p, m.lastAt) <= slop {
		m.clicks++
	} else {
		m.clicks = 1
	}
	m.down, m.at, m.moved, m.last, m.lastAt, m.scrolled = true, p, false, now, p, now
	m.link, m.onLink = v.LinkAt(p.X, p.Y)
	if c.unselectable {
		m.anchored = false
		return
	}
	pos, ok := v.positionAt(p.X, p.Y)
	m.anchored = ok
	if !ok {
		return
	}
	switch {
	case mods.Contain(input.ModShift) && v.sel.active:
		m.from, m.to, m.moved = v.sel.anchor, v.sel.anchor, true
		v.setSelection(v.sel.anchor, pos, true)
		return
	case m.clicks == 2:
		m.from, m.to = pos.Word()
	case m.clicks >= 3:
		m.from, m.to = pos.Block()
	default:
		m.from, m.to = pos, pos
		return
	}
	m.moved = true
	v.setSelection(m.from, m.to, true)
}

// dragSelection carries the press on to p.
func (c *Controller) dragSelection(p image.Point) {
	m, v := &c.mouse, c.view
	if !m.moved && dist(p, m.at) <= clickSlop*c.scale {
		return
	}
	m.moved = true
	if c.unselectable {
		return
	}
	pos, ok := v.positionAt(p.X, p.Y)
	if !ok {
		return
	}
	if !m.anchored {
		m.from, m.to, m.anchored = pos, pos, true
	}
	start, end := pos, pos
	switch {
	case m.clicks == 2:
		start, end = pos.Word()
	case m.clicks >= 3:
		start, end = pos.Block()
	}
	if engine.Compare(start, m.from) < 0 {
		v.setSelection(m.to, start, true)
	} else {
		v.setSelection(m.from, end, true)
	}
}

// endSelection ends the press at p: a click, if it didn't move.
func (c *Controller) endSelection(p image.Point) {
	m := &c.mouse
	m.down = false
	if m.moved || m.clicks > 1 {
		return
	}
	if m.onLink {
		c.click(m.link)
		return
	}
	if !c.unselectable {
		c.view.ClearSelection()
	}
}

// selectionScrolling reports whether a drag past the View's top or bottom
// is scrolling it.
func (c *Controller) selectionScrolling() bool {
	if c.unselectable || !c.mouse.down || !c.mouse.moved || !c.hasPointer {
		return false
	}
	b := c.bounds()
	return c.pointer.Y < b.Min.Y || c.pointer.Y >= b.Max.Y
}

// scrollSelection scrolls the View while a drag is past its top or bottom,
// faster the further past, and carries the selection on.
func (c *Controller) scrollSelection(now time.Duration) {
	if !c.selectionScrolling() {
		c.mouse.scrolled = now
		return
	}
	dt := (now - c.mouse.scrolled).Seconds()
	c.mouse.scrolled = now
	b := c.bounds()
	past := float64(c.pointer.Y - (b.Max.Y - 1))
	if c.pointer.Y < b.Min.Y {
		past = float64(c.pointer.Y - b.Min.Y)
	}
	c.view.ScrollBy(past * selectionScrollSpeed * dt)
	c.dragSelection(c.pointer)
}

func dist(a, b image.Point) float64 {
	return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y))
}
