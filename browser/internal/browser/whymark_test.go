//go:build !js

package browser

import (
	"strings"
	"testing"
)

// cellarDoc is a tiny whymark adventure, with the journal hint if
// journal is set.
func cellarDoc(journal bool) string {
	hint := ""
	if journal {
		hint = "  journal: true\n"
	}
	return "---\ntitle: The Cellar\nwhymark:\n  version: 1\n" + hint + "---\n\n" +
		"```lua\nfunction init(turn) return { lamp = false } end\n```\n\n" +
		"# Hall\n\nYou're in a hall. `= turn.state.lamp and \"Your lamp glows.\" or \"It's dark below.\"`\n" +
		"`if turn.query.shout then`\nYour voice echoes.\n`end`\n\n" +
		"`if not turn.state.lamp then`\n- [Take the lamp](#take-the-lamp)\n`end`\n" +
		"- [Go down](#cellar) · [Shout](?shout=yes)\n\n" +
		"# (Take the lamp)\n\n```lua\nturn.state.lamp = true\n```\n\nYou take the lamp. [Back to the hall](#hall)\n\n" +
		"# Cellar\n\n`if turn.state.lamp then`\nA bottle of port. You win!\n`else`\nIt's pitch black. [Back up](#hall)\n`end`\n"
}

// pageText returns the text the current page shows, drawn.
func pageText(app *App) string {
	view := app.Panel.View()
	r := &recorder{Area: view.Bounds()}
	view.Draw(r, 0)
	var b strings.Builder
	for _, t := range r.Texts {
		b.WriteString(t.S)
		b.WriteString(" ")
	}
	return b.String()
}

// TestWhymarkInBrowser plays a whymark document in the browser: its
// pages, their titles and locations, links as turns, Back showing a kept
// page, and Reload sending the same request again.
func TestWhymarkInBrowser(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Play](cellar.md)\n")
	writeTempMD(t, dir, "cellar.md", cellarDoc(false))
	app := newTestApp(t, dir, start, "# Start\n\n[Play](cellar.md)\n")
	var title string
	app.OnTitleChange = func(s string) { title = s }

	app.Follow("cellar.md")
	if !strings.Contains(pageText(app), "dark below") || title != "Hall" || app.Location().Fragment != "hall" {
		t.Fatalf("the first page shows %q, titled %q, at %s; want the hall", pageText(app), title, app.Location())
	}
	if strings.Contains(pageText(app), "turn.state") {
		t.Error("the page shows the document's code")
	}
	hall := app.Panel.View()

	app.FollowAnchor("take-the-lamp")
	if !strings.Contains(pageText(app), "You take the lamp") || title != "Take the lamp" {
		t.Errorf("after taking the lamp, the page shows %q, titled %q", pageText(app), title)
	}
	app.Follow("#hall")
	if !strings.Contains(pageText(app), "Your lamp glows") {
		t.Errorf("back in the hall, the page shows %q", pageText(app))
	}

	app.Back()
	app.Back()
	if app.Panel.View() != hall {
		t.Error("going back didn't show the kept page")
	}
	app.Follow("?shout=yes")
	if !strings.Contains(pageText(app), "Your voice echoes") || app.Location().Fragment != "hall" {
		t.Errorf("shouting shows %q, at %s; want the hall, echoing", pageText(app), app.Location())
	}
	shouted := app.Panel.View()
	app.Reload()
	if app.Panel.View() == shouted || !strings.Contains(pageText(app), "Your voice echoes") {
		t.Error("Reload didn't send the same request again")
	}

	// From the first page, the lamp was never taken.
	app.Back()
	app.FollowAnchor("cellar")
	if !strings.Contains(pageText(app), "pitch black") {
		t.Errorf("the cellar, from the first page, shows %q", pageText(app))
	}
}

// TestWhymarkJournal checks that a document hinting at journal mode turns
// it on, and that a link in an earlier page goes on from that page.
func TestWhymarkJournal(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Play](cellar.md)\n")
	writeTempMD(t, dir, "cellar.md", cellarDoc(true))
	app := newTestApp(t, dir, start, "# Start\n\n[Play](cellar.md)\n")
	app.Follow("cellar.md")
	if !app.Journal() {
		t.Fatal("the journal hint didn't turn journal mode on")
	}
	app.FollowAnchor("take-the-lamp")
	app.Follow("#hall")
	if got := strings.Join(titles(app), ", "); got != "Start, Hall, Take the lamp, Hall" {
		t.Errorf("the journal shows %s", got)
	}

	// The first hall's "Go down", where the lamp wasn't taken yet.
	app.journal.stack.Reveal(1)
	clickLink(t, app, 1, "#cellar")
	if got := strings.Join(titles(app), ", "); got != "Start, Hall, Cellar" {
		t.Errorf("after a link in the first hall, the journal shows %s", got)
	}
	app.ToggleJournal()
	if !strings.Contains(pageText(app), "pitch black") {
		t.Errorf("the branch's cellar shows %q", pageText(app))
	}
}
