//go:build !js

package browser

import (
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whynot/input"
)

// TestKeyString checks how keys are written for people.
func TestKeyString(t *testing.T) {
	shortcut := "Ctrl+"
	if macOS {
		shortcut = "Cmd+"
	}
	for _, c := range []struct {
		key  Key
		want string
	}{
		{Key{Name: "J", Shortcut: true}, shortcut + "J"},
		{Key{Name: "Space", Shift: true}, "Shift+Space"},
		{Key{Name: "Left", Alt: true}, "Alt+←"},
		{Key{Name: "/", Shift: true}, "?"},
		{Key{Name: "-", Shortcut: true}, shortcut + "−"},
		{Key{Name: "Escape"}, "Esc"},
	} {
		if got := c.key.String(); got != c.want {
			t.Errorf("%+v is written %q, want %q", c.key, got, c.want)
		}
	}
}

// TestPressKey checks that a key runs its command, whatever its
// modifiers say, and that a key no command has does nothing.
func TestPressKey(t *testing.T) {
	app, _ := journalApp(t)
	if !app.PressKey(Key{Name: "J", Shortcut: true}) || !app.Journal() {
		t.Error("the journal shortcut didn't turn journal mode on")
	}
	if !app.PressKey(Key{Name: "=", Shortcut: true}) || app.Zoom() <= 1 {
		t.Errorf("the zoom-in shortcut didn't zoom in: zoom %v", app.Zoom())
	}
	if app.PressKey(Key{Name: "="}) {
		t.Error("= without the shortcut modifier ran a command")
	}
	if !app.PressKey(Key{Name: "0", Shortcut: true}) || app.Zoom() != 1 {
		t.Errorf("the zoom reset didn't reset: zoom %v", app.Zoom())
	}
	if app.PressKey(Key{Name: "J"}) {
		t.Error("J without the shortcut modifier ran a command")
	}
}

// TestActionsPage checks the actions page: its labels follow the state,
// an action that shows on the page keeps it open, generated again, and
// one that changes what's shown closes it first.
func TestActionsPage(t *testing.T) {
	app, _ := journalApp(t)
	app.ShowActions()
	if !app.ActionsShowing() {
		t.Fatal("the actions page isn't showing")
	}
	source := string(app.actionsSource())
	for _, want := range []string{"| Journal mode | **off** | [turn on](whynot:journal) |", "| Zoom | **100%** |", "[Go back](whynot:back)", "Go forward |"} {
		if !strings.Contains(source, want) {
			t.Errorf("the actions page doesn't have %q:\n%s", want, source)
		}
	}

	page := app.Panel.View()
	app.Follow("whynot:zoom-in")
	if !app.ActionsShowing() || app.Panel.View() == page {
		t.Error("zooming didn't show the actions page again")
	}
	if !strings.Contains(string(app.actionsSource()), "| Zoom | **110%** |") {
		t.Error("the actions page doesn't show the new zoom")
	}
	dark := app.DarkTheme()
	app.Follow("whynot:theme")
	if app.DarkTheme() == dark || !app.ActionsShowing() {
		t.Error("the theme action didn't switch the theme, keeping the page")
	}

	app.Follow("whynot:journal")
	if app.ActionsShowing() || !app.Journal() {
		t.Error("the journal action didn't close the page and turn journal mode on")
	}
	app.ShowActions()
	if !strings.Contains(string(app.actionsSource()), "| Journal mode | **on** | [turn off](whynot:journal) |") {
		t.Error("in journal mode, the actions page doesn't offer to turn it off")
	}
	app.PressKey(Key{Name: "Escape"})
	if app.ActionsShowing() {
		t.Error("Escape didn't close the actions page")
	}
}

// TestActionLinksOnlyOnActionsPage checks that a "whynot:" link does
// nothing outside the actions page.
func TestActionLinksOnlyOnActionsPage(t *testing.T) {
	app, _ := journalApp(t)
	dark := app.DarkTheme()
	app.Follow("whynot:theme")
	if app.DarkTheme() != dark {
		t.Error("a whynot: link in a document switched the theme")
	}
}

// TestActionsReplaceTOC checks the actions page and the TOC replace each
// other, the document staying the one behind them.
func TestActionsReplaceTOC(t *testing.T) {
	dir := t.TempDir()
	a := writeTempMD(t, dir, "a.md", "# A\n\n## One\n")
	app := newTestApp(t, dir, a, "# A\n\n## One\n")
	doc := app.Panel.View()
	app.ShowTOC()
	app.ShowActions()
	if !app.ActionsShowing() || app.TOCShowing() {
		t.Fatal("the actions page didn't replace the TOC")
	}
	app.Follow("whynot:toc")
	if !app.TOCShowing() {
		t.Fatal("the TOC action didn't show the TOC")
	}
	app.HideOverlay()
	if app.Panel.View() != doc {
		t.Error("closing the TOC didn't show the document again")
	}
}

// TestActionsPageTouch checks that after a touch the actions page leaves
// keys out, and that mouse input soon after a touch, as a web browser
// follows a tap with, doesn't bring them back, while later mouse input
// does.
func TestActionsPageTouch(t *testing.T) {
	app, _ := journalApp(t)
	app.Frame([]input.Event{input.TouchStart{ID: 1, X: 10, Y: 10}, input.TouchEnd{ID: 1}}, 0)
	app.Frame([]input.Event{input.PointerMove{X: 10, Y: 10}}, 100*time.Millisecond)
	source := string(app.actionsSource())
	if strings.Contains(source, "Keys") || strings.Contains(source, "## Reading") {
		t.Errorf("after a touch, the actions page lists keys:\n%s", source)
	}
	if !strings.Contains(source, "| Journal mode | **off** | [turn on](whynot:journal) |") {
		t.Errorf("after a touch, the actions page lost its settings:\n%s", source)
	}
	app.Frame([]input.Event{input.PointerMove{X: 20, Y: 10}}, 5*time.Second)
	if !strings.Contains(string(app.actionsSource()), "## Reading") {
		t.Error("after mouse input, the actions page doesn't list keys again")
	}
}
