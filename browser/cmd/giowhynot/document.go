package main

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/arnodel/whynot/backends/giobackend"
	"github.com/arnodel/whynot/browser/internal/browser"
	"github.com/arnodel/whynot/input"
)

// document shows the browser's document in the window, with Gio's own
// scrollbar.
type document struct {
	app       *browser.App
	renderer  *giobackend.Renderer
	input     giobackend.Input
	scrollbar giobackend.NativeScrollbar
	start     time.Time // the clock the document runs on

	// onPress is called when a press lands on the document: Gio's key
	// focus doesn't follow clicks, so the toolbar can't otherwise tell
	// the user clicked away from the address bar.
	onPress func()
}

// layout runs the document's frame: input, then drawing.
func (d *document) layout(gtx layout.Context) {
	now := time.Since(d.start)
	bounds := d.app.Bounds()
	events := d.input.Source(gtx, d, bounds).Events()
	if d.onPress != nil && pressed(events) {
		d.onPress()
	}
	d.app.Frame(events, now)
	// Gio only produces frames when something happens.
	if d.app.Animating() {
		gtx.Execute(op.InvalidateCmd{})
	}
	d.app.Draw(d.renderer.NewCanvas(gtx.Ops, bounds), now)
	if stack := d.app.JournalStack(); stack != nil {
		d.scrollbar.Layout(gtx, stack)
	} else {
		d.scrollbar.Layout(gtx, d.app.Panel.View())
	}
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
