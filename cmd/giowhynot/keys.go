package main

import (
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/internal/browser"
)

// pollKeys handles global keyboard shortcuts - back/forward, page/line
// scroll, paste-to-navigate, zoom. key.Filter with no Focus requirement
// matches regardless of what's focused (Gio's own router special-cases
// a nil Focus - confirmed against io/input/key.go), so unlike
// pointer.Filter these need no prior area/event.Op registration: call
// once per frame, from anywhere.
func pollKeys(gtx layout.Context, app *browser.App, panel *whynot.Panel) {
	for {
		e, ok := gtx.Event(
			key.Filter{Name: key.NameDeleteBackward},
			key.Filter{Name: key.NameSpace},
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NameLeftArrow},
			key.Filter{Name: key.NameRightArrow},
			key.Filter{Name: key.NameEscape},
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
		case key.NameUpArrow:
			panel.ScrollUp()
		case key.NameDownArrow:
			panel.ScrollDown()
		case key.NameLeftArrow:
			panel.ScrollLeft()
		case key.NameRightArrow:
			panel.ScrollRight()
		case key.NameEscape:
			if app.TOCShowing() {
				app.HideTOC()
			}
		case "V":
			app.Paste()
		case "=":
			app.ZoomIn()
		case "-":
			app.ZoomOut()
		}
	}
}
