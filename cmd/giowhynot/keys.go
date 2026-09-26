package main

import (
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/arnodel/whynot/browser"
	"github.com/arnodel/whynot/giorenderer"
)

// arrowScrollLines matches cmd/whynot's own input.go constant.
const arrowScrollLines = 40

// pollKeys handles global keyboard shortcuts - back/forward, page/line
// scroll, paste-to-navigate, zoom. key.Filter with no Focus requirement
// matches regardless of what's focused (Gio's own router special-cases
// a nil Focus - confirmed against io/input/key.go), so unlike
// pointer.Filter these need no prior area/event.Op registration: call
// once per frame, from anywhere. deviceScale is the display's own
// scale, with no zoom applied.
func pollKeys(gtx layout.Context, app *browser.App, panel *giorenderer.Panel, deviceScale float64) {
	for {
		e, ok := gtx.Event(
			key.Filter{Name: key.NameDeleteBackward},
			key.Filter{Name: key.NameSpace},
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: "V", Required: key.ModShortcut},
			key.Filter{Name: "="},
			key.Filter{Name: "-"},
		)
		if !ok {
			break
		}
		ke, ok := e.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		switch ke.Name {
		case key.NameDeleteBackward:
			if ke.Modifiers.Contain(key.ModShift) {
				app.Forward()
			} else {
				app.Back()
			}
		case key.NameSpace:
			if ke.Modifiers.Contain(key.ModShift) {
				panel.PageUp()
			} else {
				panel.PageDown()
			}
		// View.Scroll directly rather than panel.ScrollUp/Down, which
		// scale by the panel's own combined scale (deviceScale*zoom) -
		// this stays deviceScale-alone, deliberately zoom-independent,
		// matching cmd/whynot's own arrow-key scroll exactly.
		case key.NameUpArrow:
			panel.View().Scroll(arrowScrollLines * deviceScale)
		case key.NameDownArrow:
			panel.View().Scroll(-arrowScrollLines * deviceScale)
		case "V":
			app.Paste()
		case "=":
			app.ZoomIn()
		case "-":
			app.ZoomOut()
		}
	}
}
