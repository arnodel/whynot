//go:build !js

package browser

import (
	"image"
	"strings"
	"testing"

	"github.com/arnodel/whynot/input"
)

// copiedText runs CopyMarkdown, or the actions page's link to it, and
// returns what it put on the clipboard.
func copiedText(t *testing.T, copy func()) string {
	t.Helper()
	var got string
	write := clipboardWriter
	clipboardWriter = func(text string) error { got = text; return nil }
	defer func() { clipboardWriter = write }()
	copy()
	return got
}

// TestCopyMarkdown checks copying a plain document, without its front
// matter, and a whymark trail in journal mode, oldest first, without the
// pages' controls and input blocks.
func TestCopyMarkdown(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Notes](notes.md) · [Play](game.md)\n")
	writeTempMD(t, dir, "notes.md", "---\ntitle: Notes\n---\n# Notes\n\nSome *notes*.\n")
	writeTempMD(t, dir, "game.md", "---\nwhymark:\n  version: 1\n---\n# Room\n\nA room.\n\n## (Controls)\n\n- [Look](#look)\n\n"+
		"# Look\n\nYou look around.\n\n```input say\nSay…\n```\n\n## (Controls)\n\n- [Back](#room)\n")
	app := newTestApp(t, dir, start, "# Start\n\n[Notes](notes.md) · [Play](game.md)\n")

	app.Follow("notes.md")
	if got, want := copiedText(t, app.CopyMarkdown), "# Notes\n\nSome *notes*.\n"; got != want {
		t.Errorf("copying a document gave %q, want %q", got, want)
	}

	app.Back()
	app.Follow("game.md")
	app.Follow("#look")
	app.ToggleJournal()
	got := copiedText(t, app.CopyMarkdown)
	// The start page wasn't loaded, so its source isn't kept; notes.md,
	// gone back from, isn't in the trail.
	want := "# Room\n\nA room.\n\n---\n\n# Look\n\nYou look around.\n"
	if got != want {
		t.Errorf("copying the journal gave %q, want %q", got, want)
	}
}

// TestCopyFromActions checks the actions page's copy link: it copies the
// document under the page, and says so.
func TestCopyFromActions(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Notes](notes.md)\n")
	writeTempMD(t, dir, "notes.md", "# Notes\n\nSome notes.\n")
	app := newTestApp(t, dir, start, "# Start\n\n[Notes](notes.md)\n")
	app.Follow("notes.md")
	app.ShowActions()
	// The page is longer than the window, so its source is checked.
	page := func() string { return string(app.actionsSource()) }
	if !strings.Contains(page(), "[Copy this page as Markdown](whynot:copy)") || strings.Contains(page(), "copied") {
		t.Fatalf("the actions page is %q", page())
	}
	if got := copiedText(t, func() { app.Follow("whynot:copy") }); got != "# Notes\n\nSome notes.\n" {
		t.Errorf("the actions page's link copied %q", got)
	}
	if !app.ActionsShowing() || !strings.Contains(page(), "*copied*") {
		t.Errorf("after copying, the actions page is %q", page())
	}
	app.HideOverlay()
	app.ToggleJournal()
	app.ShowActions()
	if !strings.Contains(page(), "[Copy the journal as Markdown](whynot:copy)") || strings.Contains(page(), "copied") {
		t.Errorf("in journal mode, shown again, the actions page is %q", page())
	}
}

// TestCopySelection checks Cmd+C's command: it copies the selection as
// Markdown, and does nothing without one; and Select all.
func TestCopySelection(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Notes](notes.md)\n")
	writeTempMD(t, dir, "notes.md", "# Notes\n\nSome *notes*.\n")
	app := newTestApp(t, dir, start, "# Start\n\n[Notes](notes.md)\n")
	app.Follow("notes.md")
	if got := copiedText(t, app.CopySelection); got != "" || app.HasSelection() {
		t.Errorf("with nothing selected, copied %q", got)
	}
	app.SelectAll()
	if got := copiedText(t, app.CopySelection); got != "# Notes\n\nSome *notes*." {
		t.Errorf("copied the page selected all as %q", got)
	}
	app.ShowActions()
	if !strings.Contains(string(app.actionsSource()), "[Copy the selection as Markdown](whynot:copy-selection)") {
		t.Errorf("with a selection, the actions page is %q", app.actionsSource())
	}
	if got := copiedText(t, func() { app.Follow("whynot:copy-selection") }); got != "# Notes\n\nSome *notes*." {
		t.Errorf("the actions page copied the selection as %q", got)
	}
}

// TestJournalKeepsOneSelection checks that selecting in one page of the
// journal clears the others' selections.
func TestJournalKeepsOneSelection(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Notes](notes.md)\n")
	writeTempMD(t, dir, "notes.md", "# Notes\n\nSome notes.\n")
	app := newTestApp(t, dir, start, "# Start\n\n[Notes](notes.md)\n")
	app.Follow("notes.md")
	app.ToggleJournal()
	views := app.journal.stack.views
	views[0].SelectAll()
	app.SelectAll() // the current page's
	if views[0].HasSelection() || !views[1].HasSelection() {
		t.Errorf("after selecting all of the current page, selections: %v, %v", views[0].HasSelection(), views[1].HasSelection())
	}
	if got := copiedText(t, app.CopySelection); got != "# Notes\n\nSome notes." {
		t.Errorf("copied %q", got)
	}
}

// twoPageJournal returns an app in journal mode on two pages, start.md then
// notes.md.
func twoPageJournal(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\nAlpha beta gamma.\n\n[Notes](notes.md)\n")
	writeTempMD(t, dir, "notes.md", "# Notes\n\nDelta epsilon zeta.\n")
	app := newTestApp(t, dir, start, "# Start\n\nAlpha beta gamma.\n\n[Notes](notes.md)\n")
	app.Follow("notes.md")
	app.ToggleJournal()
	return app
}

// textAt returns a point inside the first drawing of the text s in app
// that shows.
func textAt(t *testing.T, app *App, s string) image.Point {
	t.Helper()
	r := &recorder{Area: app.Bounds()}
	app.Draw(r, 0)
	for _, tx := range r.Texts {
		if p := image.Pt(tx.X+2, tx.Y-4); tx.S == s && p.In(app.Bounds()) {
			return p
		}
	}
	t.Fatalf("no text %q drawn where it shows", s)
	return image.Point{}
}

// TestSelectAcrossPages checks that a drag from one page of the journal
// into the next selects in both, and copying joins them.
func TestSelectAcrossPages(t *testing.T) {
	app := twoPageJournal(t)
	// The journal shows the end of the trail: on the first page, its link.
	from, to := textAt(t, app, "Notes"), textAt(t, app, "epsilon")
	app.Frame([]input.Event{
		input.PointerMove{X: from.X, Y: from.Y},
		input.PointerButton{X: from.X, Y: from.Y, Button: input.ButtonPrimary, Down: true},
		input.PointerMove{X: to.X, Y: to.Y},
		input.PointerButton{X: to.X, Y: to.Y, Button: input.ButtonPrimary},
	}, 0)
	// From the link to the first page's end, then the second page from
	// its start to "epsilon", which spans its heading and paragraph whole.
	want := "[Notes](notes.md)\n\n---\n\n# Notes\n\nDelta epsilon zeta."
	if got := copiedText(t, app.CopySelection); got != want {
		t.Errorf("copied %q, want %q", got, want)
	}
	// A drag within one page then clears the other's selection.
	a, b := textAt(t, app, "Delta"), textAt(t, app, "zeta")
	b.X += 10
	app.Frame([]input.Event{
		input.PointerMove{X: a.X, Y: a.Y},
		input.PointerButton{X: a.X, Y: a.Y, Button: input.ButtonPrimary, Down: true},
		input.PointerMove{X: b.X, Y: b.Y},
		input.PointerButton{X: b.X, Y: b.Y, Button: input.ButtonPrimary},
	}, 0)
	if views := app.journal.stack.views; views[0].HasSelection() || !views[1].HasSelection() {
		t.Errorf("after a drag within the second page, selections: %v, %v", views[0].HasSelection(), views[1].HasSelection())
	}
}

// TestSelectAllTwice checks that Select all selects the current page, and
// again, the whole journal.
func TestSelectAllTwice(t *testing.T) {
	app := twoPageJournal(t)
	views := app.journal.stack.views
	app.SelectAll()
	if views[0].HasSelection() || !views[1].AllSelected() {
		t.Fatal("Select all didn't select just the current page")
	}
	app.SelectAll()
	if !views[0].AllSelected() || !views[1].AllSelected() {
		t.Fatal("Select all again didn't select the whole journal")
	}
	want := "# Start\n\nAlpha beta gamma.\n\n[Notes](notes.md)\n\n---\n\n# Notes\n\nDelta epsilon zeta."
	if got := copiedText(t, app.CopySelection); got != want {
		t.Errorf("copied %q, want %q", got, want)
	}
}
