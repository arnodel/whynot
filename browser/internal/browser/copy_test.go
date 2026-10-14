//go:build !js

package browser

import (
	"strings"
	"testing"
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
