//go:build !js

package browser

import (
	"strings"
	"testing"
	"time"
)

// TestResume checks persistence across sessions: a document asking to be
// resumed shows its latest page, as it was, when opened in a new session,
// and carries on from there, but starts afresh from a link on its own page; one that doesn't ask
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
	shows(next, "the next session, resumed", "count 2")
	next.Follow("?")
	shows(next, "the next session, carrying on", "count 3")
	next.Follow("resumed.md")
	shows(next, "a link to the document from its own page", "count 1")
	next.Back()
	next.Back()
	next.Back()
	next.Follow("plain.md")
	shows(next, "the next session, plain", "count 1")
}

// TestPageURL checks page URLs: a whymark page's location carries its
// page-id, opening it in another session shows that page, as it was, and
// carries on from it rather than from the latest, an unknown page-id opens the document afresh, and
// CopyLink copies it.
func TestPageURL(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Count](count.md)\n")
	writeTempMD(t, dir, "count.md", "---\nwhymark:\n  version: 1\n  resume: true\n---\n```lua\nfunction init(turn) return {n = 0} end\n```\n"+
		"# Count\n\n`turn.state.n = turn.state.n + 1`\ncount `= turn.state.n`\n")
	session := func() *App {
		app := newTestApp(t, dir, start, "# Start\n\n[Count](count.md)\n")
		if err := app.EnablePersistence(data); err != nil {
			t.Fatal(err)
		}
		return app
	}
	first := session()
	first.Follow("count.md")
	first.Follow("?")
	second := first.Location().String()
	if passageOf(first) != "count" || !strings.Contains(second, "#count@") {
		t.Fatalf("the page's URL is %s, want its passage and page-id", second)
	}
	if got := copiedText(t, first.CopyLink); got != second {
		t.Errorf("CopyLink copied %q, want %q", got, second)
	}
	first.Follow("?")
	if !strings.Contains(pageText(first), "count 3") {
		t.Fatalf("the first session's last page shows %q", pageText(first))
	}

	next := session()
	next.Follow(second)
	if text := pageText(next); !strings.Contains(text, "count 2") {
		t.Errorf("opening the second page's URL shows %q, want the page as it was: count 2", text)
	}
	next.Follow("?")
	if text := pageText(next); !strings.Contains(text, "count 3") {
		t.Errorf("the page after it shows %q, want it carried on from: count 3", text)
	}
	next.Follow(strings.Replace(second, "@", "@nosuchpage", 1))
	if text := pageText(next); !strings.Contains(text, "count 1") {
		t.Errorf("an unknown page-id shows %q, want the document afresh", text)
	}
}

// TestPageURLArg checks that a page URL given on the command line keeps its
// page-id.
func TestPageURLArg(t *testing.T) {
	path := writeTempMD(t, t.TempDir(), "count.md", "# Count\n").Path
	u, err := ResolveLocationArg("file://" + path + "#count@k3j9x2")
	if err != nil {
		t.Fatal(err)
	}
	if u.Fragment != "count@k3j9x2" {
		t.Errorf("the location is %s, want the page URL's fragment kept", u)
	}
}

// TestPageTrail checks that a page opened from its page URL shows, in
// journal mode, the pages that led to it, loaded in the background, and
// that nothing is loaded outside journal mode.
func TestPageTrail(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Count](count.md)\n")
	writeTempMD(t, dir, "count.md", "---\nwhymark:\n  version: 1\n---\n```lua\nfunction init(turn) return {n = 0} end\n```\n"+
		"# Count\n\n`turn.state.n = turn.state.n + 1`\ncount `= turn.state.n`\n")
	session := func() *App {
		app := newTestApp(t, dir, start, "# Start\n\n[Count](count.md)\n")
		if err := app.EnablePersistence(data); err != nil {
			t.Fatal(err)
		}
		return app
	}
	first := session()
	first.Follow("count.md")
	first.Follow("?")
	first.Follow("?")
	third := first.Location().String()

	next := session()
	next.Follow(third)
	before := len(next.history)
	next.Update()
	if p := next.pageOf(next.Panel.View().Document()); p.trailLoading {
		t.Error("the trail is loading outside journal mode")
	}

	next.ToggleJournal()
	deadline := time.Now().Add(5 * time.Second)
	for len(next.history) == before && time.Now().Before(deadline) {
		next.Update()
		time.Sleep(time.Millisecond)
	}
	trail := next.history[before:]
	if len(trail) != 2 {
		t.Fatalf("the trail has %d pages, want 2", len(trail))
	}
	for i, e := range trail {
		next.Panel.SetView(e.view)
		want := []string{"count 1", "count 2"}[i]
		if text := pageText(next); !strings.Contains(text, want) {
			t.Errorf("page %d of the trail shows %q, want %q", i, text, want)
		}
	}
	if got := len(next.journal.pages); got != before+3 {
		t.Errorf("the journal shows %d pages, want %d", got, before+3)
	}
}
