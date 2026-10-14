package main

import (
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/arnodel/whynot/browser/internal/browser"
)

// gioKeyNames are the Gio key names of browser.Key names that differ.
var gioKeyNames = map[string]key.Name{
	"Space":     key.NameSpace,
	"Backspace": key.NameDeleteBackward,
	"Escape":    key.NameEscape,
	"Up":        key.NameUpArrow,
	"Down":      key.NameDownArrow,
	"Left":      key.NameLeftArrow,
	"Right":     key.NameRightArrow,
}

// editingKeys are the keys that move or delete in a text field, with or
// without modifiers (Cmd+Left goes to the line's start on macOS), so they
// never run commands while typing.
var editingKeys = map[string]bool{"Left": true, "Right": true, "Up": true, "Down": true, "Backspace": true}

// pollKeys runs the commands of the keys pressed (see browser.App.Keys):
// while typing in a text field, only those with the shortcut modifier,
// and not editingKeys, so that keys typed don't also run commands. A
// focused field still gets the shortcuts it handles itself, such as
// Cmd+V: Gio gives the focused widget's filters precedence.
// key.Filter with no Focus requirement matches regardless of what's
// focused (Gio's own router special-cases a nil Focus - confirmed
// against io/input/key.go), so unlike pointer.Filter these need no prior
// area/event.Op registration: call once per frame, from anywhere. A held
// key's repeated presses come as more Press events.
func pollKeys(gtx layout.Context, app *browser.App, typing bool) {
	names := map[key.Name]string{}
	var filters []event.Filter
	for _, k := range app.Keys() {
		if typing && (!k.Shortcut || editingKeys[k.Name]) {
			continue
		}
		name, ok := gioKeyNames[k.Name]
		if !ok {
			name = key.Name(k.Name)
		}
		if _, seen := names[name]; seen {
			continue
		}
		names[name] = k.Name
		// The modifiers are matched exactly by PressKey.
		filters = append(filters, key.Filter{Name: name, Optional: key.ModShortcut | key.ModShift | key.ModAlt})
	}
	if !typing {
		// "?" may come as itself rather than as Shift+"/".
		names["?"] = "/"
		filters = append(filters, key.Filter{Name: "?", Optional: key.ModShift})
	}
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		ke, ok := e.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		k := browser.Key{
			Name:     names[ke.Name],
			Shortcut: ke.Modifiers.Contain(key.ModShortcut),
			Shift:    ke.Modifiers.Contain(key.ModShift) || ke.Name == "?",
			Alt:      ke.Modifiers.Contain(key.ModAlt),
		}
		app.PressKey(k)
	}
}
