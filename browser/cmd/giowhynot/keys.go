package main

import (
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/arnodel/whynot/browser/internal/browser"
)

// pollKeys handles global keyboard shortcuts - back/forward, page/line
// scroll, paste-to-navigate, zoom. key.Filter with no Focus requirement
// matches regardless of what's focused (Gio's own router special-cases
// a nil Focus - confirmed against io/input/key.go), so unlike
// pointer.Filter these need no prior area/event.Op registration: call
// once per frame, from anywhere.
func pollKeys(gtx layout.Context, app *browser.App) {
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
			key.Filter{Name: "J", Required: key.ModShortcut},
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
				app.PageUp()
			} else {
				app.PageDown()
			}
		case key.NameUpArrow:
			app.ScrollUp()
		case key.NameDownArrow:
			app.ScrollDown()
		case key.NameLeftArrow:
			app.ScrollLeft()
		case key.NameRightArrow:
			app.ScrollRight()
		case key.NameEscape:
			if app.TOCShowing() {
				app.HideTOC()
			}
		case "V":
			app.Paste()
		case "J":
			app.ToggleJournal()
		case "=":
			app.ZoomIn()
		case "-":
			app.ZoomOut()
		}
	}
}
