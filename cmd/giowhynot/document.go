package main

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/giorenderer"
	"github.com/arnodel/whynot/input"
)

// document shows the browser's Panel in the window, with Gio's own
// scrollbar.
type document struct {
	panel     *whynot.Panel
	renderer  *giorenderer.Renderer
	input     giorenderer.Input
	scrollbar giorenderer.NativeScrollbar
	start     time.Time // the clock the panel runs on

	// onPress is called when a press lands on the document: Gio's key
	// focus doesn't follow clicks, so the toolbar can't otherwise tell
	// the user clicked away from the address bar.
	onPress func()
}

// layout runs the document's frame: input, then drawing.
func (d *document) layout(gtx layout.Context) {
	now := time.Since(d.start)
	events := d.input.Source(gtx, d.panel, d.panel.Bounds()).Events()
	if d.onPress != nil && pressed(events) {
		d.onPress()
	}
	d.panel.Frame(events, now)
	// Gio only produces frames when something happens.
	if d.panel.Animating() {
		gtx.Execute(op.InvalidateCmd{})
	}
	d.panel.Draw(d.renderer.NewCanvas(gtx.Ops, d.panel.Bounds()), now)
	d.scrollbar.Layout(gtx, d.panel.View(), d.panel.Bounds())
}

// pressed reports whether events include a press: a mouse button or a
// touch.
func pressed(events []input.Event) bool {
	for _, e := range events {
		switch e := e.(type) {
		case input.PointerButton:
			if e.Down {
				return true
			}
		case input.TouchStart:
			return true
		}
	}
	return false
}
