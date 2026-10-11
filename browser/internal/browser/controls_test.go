//go:build !js

package browser

import (
	"strings"
	"testing"
)

// TestControlsStart checks where a page's controls start: at its last
// top-level thematic break, not at a setext heading's underline, nor at a
// break inside a code block.
func TestControlsStart(t *testing.T) {
	for _, c := range []struct {
		source string
		want   string // what's before the controls, or "" for none
	}{
		{"A page.\n\n---\n\n[Next](#next)\n", "A page.\n\n"},
		{"One.\n\n---\n\nTwo.\n\n***\n\n[Next](#next)\n", "One.\n\n---\n\nTwo.\n\n"},
		{"Title\n---\n\nText.\n", ""},
		{"Text.\n\n```\n---\n```\n", ""},
	} {
		start := controlsStart([]byte(c.source))
		got := ""
		if start >= 0 {
			got = c.source[:start]
		}
		if got != c.want {
			t.Errorf("before the controls of %q: %q, want %q", c.source, got, c.want)
		}
	}
}

// TestJournalHidesControls checks that in journal mode, earlier pages of
// a whymark document are shown without their controls, the current page
// with them, and that the pages themselves keep them.
func TestJournalHidesControls(t *testing.T) {
	doc := "---\nwhymark:\n  version: 1\n---\n" +
		"# Hall\n\nA hall.\n\n---\n\n[Go down](#cellar)\n\n" +
		"# Cellar\n\nA cellar.\n\n---\n\n[Go up](#hall)\n"
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Play](house.md)\n")
	writeTempMD(t, dir, "house.md", doc)
	app := newTestApp(t, dir, start, "# Start\n\n[Play](house.md)\n")
	app.Follow("house.md")
	hall := app.Panel.View()
	app.ToggleJournal()
	app.FollowAnchor("cellar")

	// Each page's text, from its start, at the stack's size.
	if top, _ := app.journal.stack.Position(); top == 0 {
		t.Error("the stack went back to the start, losing the reader's place")
	}
	text := func(i int) string {
		v := app.journal.stack.views[i]
		v.SetBounds(app.Bounds())
		v.ScrollToRatio(0)
		r := &recorder{Area: v.Bounds()}
		v.Draw(r, 0)
		var b strings.Builder
		for _, t := range r.Texts {
			b.WriteString(t.S + " ")
		}
		return b.String()
	}
	drawApp(app)
	if got := text(1); !strings.Contains(got, "A hall") || strings.Contains(got, "Go down") {
		t.Errorf("the hall, in the trail, shows %q: want it without its controls", got)
	}
	if got := text(2); !strings.Contains(got, "Go up") {
		t.Errorf("the current page shows %q: want its controls", got)
	}

	app.Back()
	if app.Panel.View() != hall {
		t.Error("going back didn't show the hall's own View")
	}
	app.ToggleJournal()
	if !strings.Contains(pageText(app), "Go down") {
		t.Errorf("the hall, alone, shows %q: want its controls", pageText(app))
	}
}
