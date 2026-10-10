//go:build !js

package browser

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whynot"
)

// slowServer serves first, then, once release is closed, rest: a
// document arriving in two parts, the second well after loadWait.
func slowServer(t *testing.T, first, rest string) (server *httptest.Server, release chan struct{}) {
	t.Helper()
	release = make(chan struct{})
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte(first))
		w.(http.Flusher).Flush()
		<-release
		w.Write([]byte(rest))
	}))
	t.Cleanup(server.Close)
	return server, release
}

// arrived waits for the current document to be complete.
func arrived(t *testing.T, app *App) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		for range app.Panel.View().Document().Updates() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the document didn't arrive")
	}
}

// paragraphs is n paragraphs, together much taller than the View.
func paragraphs(n int) string {
	return strings.Repeat("Some text, quite a bit of it actually, more than a line's worth.\n\n", n)
}

// atEnd reports whether the end of the current document is at the bottom
// of the View, with more of it above.
func atEnd(app *App) bool {
	view := app.Panel.View()
	r := view.DocumentBounds(0)
	return r.Max.Y == view.Bounds().Max.Y && r.Min.Y < view.Bounds().Min.Y
}

// TestAppShowsDocumentAsItArrives checks a document that arrives slowly
// is shown before it's complete, and grows in the same View.
func TestAppShowsDocumentAsItArrives(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(t, dir, writeTempMD(t, dir, "a.md", "# A"), "# A")
	server, release := slowServer(t, "# Slow\n\nThe start.\n\n", "The rest.\n")

	if err := app.Navigate(server.URL); err != nil {
		t.Fatal(err)
	}
	view := app.Panel.View()
	if view.Document().Complete() {
		t.Fatal("the document is complete before the rest was sent")
	}
	close(release)
	arrived(t, app)
	if app.Panel.View() != view {
		t.Error("the View changed as the document arrived")
	}
}

// TestAppQuickDocumentIsComplete checks a document that arrives within
// loadWait is shown complete.
func TestAppQuickDocumentIsComplete(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(t, dir, writeTempMD(t, dir, "a.md", "# A"), "# A")
	app.Follow(writeTempMD(t, dir, "b.md", "# B").String())
	if !app.Panel.View().Document().Complete() {
		t.Error("a local document isn't complete when shown")
	}
}

// TestAppFollowsEnd checks a link to "#end" keeps the end of a document
// in view as it arrives, and that the reader scrolling stops it.
func TestAppFollowsEnd(t *testing.T) {
	for _, scroll := range []bool{false, true} {
		dir := t.TempDir()
		app := newTestApp(t, dir, writeTempMD(t, dir, "a.md", "# A"), "# A")
		server, release := slowServer(t, paragraphs(20), paragraphs(20)+"The very end.\n")

		if err := app.Navigate(server.URL + "#end"); err != nil {
			t.Fatal(err)
		}
		if !atEnd(app) {
			t.Fatalf("scroll %v: the end of what has arrived isn't in view", scroll)
		}
		if scroll {
			app.HandleEvents([]whynot.Event{whynot.Scroll{}})
		}
		close(release)
		arrived(t, app)
		before := app.Panel.View().ScrollPosition()
		app.Update()
		switch {
		case scroll && app.Panel.View().ScrollPosition() != before:
			t.Error("Update scrolled after the reader did")
		case !scroll && !atEnd(app):
			t.Error("the end isn't in view once arrived")
		}
	}
}

// TestAppJumpsToFragmentOnceArrived checks a link to a heading that
// hasn't arrived yet lands on it once it has, unless the reader scrolled
// meanwhile.
func TestAppJumpsToFragmentOnceArrived(t *testing.T) {
	for _, scroll := range []bool{false, true} {
		dir := t.TempDir()
		app := newTestApp(t, dir, writeTempMD(t, dir, "a.md", "# A"), "# A")
		server, release := slowServer(t, "# Top\n\n"+paragraphs(20), "# Later\n\n"+paragraphs(20))

		if err := app.Navigate(server.URL + "#later"); err != nil {
			t.Fatal(err)
		}
		if scroll {
			app.PageDown()
		}
		close(release)
		arrived(t, app)
		app.Update()
		id, _ := app.Panel.View().CurrentHeadingID()
		if (id == "later") == scroll {
			t.Errorf("scroll %v: current heading once arrived = %q", scroll, id)
		}
	}
}

// TestAppTitleAsItArrives checks the window title follows the document
// as it arrives, and that front matter's title comes first.
func TestAppTitleAsItArrives(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(t, dir, writeTempMD(t, dir, "a.md", "# A"), "# A")
	var title string
	app.OnTitleChange = func(t string) { title = t }

	server, release := slowServer(t, "Some text first.\n\n", "# Heading\n")
	if err := app.Navigate(server.URL); err != nil {
		t.Fatal(err)
	}
	if title != "Untitled document" {
		t.Errorf("title before the heading arrived = %q, want \"Untitled document\"", title)
	}
	close(release)
	arrived(t, app)
	app.Update()
	if title != "Heading" {
		t.Errorf("title once the heading arrived = %q, want \"Heading\"", title)
	}

	app.Follow(writeTempMD(t, dir, "b.md", "---\ntitle: From Front Matter\n---\n# Heading\n").String())
	if title != "From Front Matter" {
		t.Errorf("title of a document with front matter = %q, want \"From Front Matter\"", title)
	}
}
