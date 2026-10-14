package main

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/browser/internal/browser"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// TestKeysWhileTyping checks pollKeys through Gio's key router: with a
// text field focused, a shortcut still runs its command, but not one
// that edits text, such as Cmd+Left, and a key typed doesn't run a
// command; without one, keys run their commands.
func TestKeysWhileTyping(t *testing.T) {
	dir := t.TempDir()
	long := "# B\n\n" + strings.Repeat("A line.\n\n", 200)
	for name, content := range map[string]string{"a.md": "# A\n\n[B](b.md)\n", "b.md": long} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	location, err := browser.ResolveLocationArg(filepath.Join(dir, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	app := browser.NewApp(fonts.NewGoSelector(), simpletheme.DarkStyleSheet, true, browser.NewRegistry(nil))
	doc, err := app.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	app.Panel = whynot.NewPanel(app.NewView(doc), image.Rectangle{})
	app.Relayout(800, 600, 1, 40)
	app.Open(location)
	app.Follow("b.md")

	var r input.Router
	var field widget.Editor
	typing := false
	frame := func(events ...event.Event) {
		t.Helper()
		r.Queue(events...)
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Constraints: layout.Exact(image.Pt(800, 600))}
		if typing {
			event.Op(gtx.Ops, &field)
			gtx.Execute(key.FocusCmd{Tag: &field})
			for {
				if _, ok := field.Update(gtx); !ok {
					break
				}
			}
		}
		pollKeys(gtx, app, typing)
		r.Frame(&ops)
		if typing && len(events) > 0 && !r.Source().Focused(&field) {
			t.Fatal("the field isn't focused")
		}
	}
	press := func(name key.Name, mods key.Modifiers) event.Event {
		return key.Event{Name: name, Modifiers: mods, State: key.Press}
	}

	typing = true
	frame()
	frame()
	frame(press("J", key.ModShortcut))
	if !app.Journal() {
		t.Error("Cmd+J, typing, didn't turn journal mode on")
	}
	app.ToggleJournal()
	frame(press(key.NameLeftArrow, key.ModShortcut))
	if !strings.HasSuffix(app.Location().Path, "/b.md") {
		t.Errorf("Cmd+Left, typing, went to %s: it's the field's", app.Location())
	}
	frame(press(key.NameSpace, 0))
	if start, _ := app.Panel.View().VisibleRange(); start != 0 {
		t.Errorf("typing a space scrolled the page to %v", start)
	}

	typing = false
	frame()
	frame(press(key.NameSpace, 0))
	if start, _ := app.Panel.View().VisibleRange(); start == 0 {
		t.Error("a space without a field focused didn't scroll the page")
	}
	frame(press("-", key.ModShortcut))
	if app.Zoom() >= 1 {
		t.Errorf("Cmd+- without a field focused left the zoom at %v", app.Zoom())
	}
}
