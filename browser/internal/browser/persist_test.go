//go:build !js

package browser

import (
	"strings"
	"testing"
)

// TestResume checks persistence across sessions: a document asking to be
// resumed carries on from its latest page when opened in a new session,
// but starts afresh from a link on its own page; one that doesn't ask
// always starts afresh.
func TestResume(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	counter := func(resume string) string {
		return "---\nwhymark:\n  version: 1\n" + resume + "---\n```lua\nfunction init(turn) return {n = 0} end\n```\n# (Count)\n\n" +
			"`turn.state.n = turn.state.n + 1`\ncount `= turn.state.n`\n\n[again](?) · [start over](counter.md)\n"
	}
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Resumed](resumed.md) · [Plain](plain.md)\n")
	writeTempMD(t, dir, "resumed.md", strings.Replace(counter("  resume: true\n"), "counter.md", "resumed.md", 1))
	writeTempMD(t, dir, "plain.md", strings.Replace(counter(""), "counter.md", "plain.md", 1))
	session := func() *App {
		app := newTestApp(t, dir, start, "# Start\n\n[Resumed](resumed.md) · [Plain](plain.md)\n")
		if err := app.EnablePersistence(data); err != nil {
			t.Fatal(err)
		}
		return app
	}
	shows := func(app *App, what, want string) {
		t.Helper()
		if text := pageText(app); !strings.Contains(text, want) {
			t.Errorf("%s: the page shows %q, want %q", what, text, want)
		}
	}

	first := session()
	first.Follow("resumed.md")
	first.Follow("?")
	shows(first, "the first session", "count 2")
	first.Back()
	first.Back()
	first.Follow("plain.md")
	first.Follow("?")
	shows(first, "the plain document", "count 2")

	next := session()
	next.Follow("resumed.md")
	shows(next, "the next session, resumed", "count 3")
	next.Follow("resumed.md")
	shows(next, "a link to the document from its own page", "count 1")
	next.Back()
	next.Back()
	next.Follow("plain.md")
	shows(next, "the next session, plain", "count 1")
}
