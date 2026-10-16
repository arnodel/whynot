//go:build !js

package browser

import (
	"strings"
	"testing"
)

// TestShowSource checks the source overlay: a document's Markdown, as it
// arrived, toggled by its command, from the actions page too.
func TestShowSource(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[A](a.md)\n")
	writeTempMD(t, dir, "a.md", "# A\n\nSome *text*.\n")
	app := newTestApp(t, dir, start, "# Start\n\n[A](a.md)\n")
	app.Follow("a.md")
	doc := app.Panel.View()
	app.PressKey(Key{Name: "U", Shortcut: true})
	if !app.SourceShowing() || !strings.Contains(pageText(app), "Some *text*.") {
		t.Fatalf("the source shows %q, want the document's Markdown", pageText(app))
	}
	app.PressKey(Key{Name: "U", Shortcut: true})
	if app.SourceShowing() || app.Panel.View() != doc {
		t.Fatal("pressing the key again didn't show the document again")
	}
	app.ShowActions()
	app.Follow("whynot:source")
	if !app.SourceShowing() {
		t.Fatal("the actions page's link didn't show the source")
	}
	app.Back()
	if app.SourceShowing() || app.Panel.View() != doc {
		t.Error("Back didn't close the source")
	}
}

// TestShowSourceWhymark checks that a whymark page's source is what its
// turn wrote, followed by the document's own source.
func TestShowSourceWhymark(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Play](cellar.md)\n")
	writeTempMD(t, dir, "cellar.md", cellarDoc(false))
	app := newTestApp(t, dir, start, "# Start\n\n[Play](cellar.md)\n")
	app.Follow("cellar.md")
	text := string(app.sourcePage(app.Panel.View().Document()))
	page, document, ok := strings.Cut(text, "## Document")
	if !ok {
		t.Fatalf("the source shows %q, want a Document section", text)
	}
	if !strings.Contains(page, "dark below") || strings.Contains(page, "turn.state") {
		t.Errorf("the page's source shows %q, want what its turn wrote", page)
	}
	if !strings.Contains(document, "turn.state") {
		t.Errorf("the document's source shows %q, want its Lua", document)
	}
}
