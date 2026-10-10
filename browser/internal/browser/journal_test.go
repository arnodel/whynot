//go:build !js

package browser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arnodel/whynot/input"
)

// journalApp returns an App that has visited pages a, b and c, each
// linking to the next and to a page d, in a temporary directory.
func journalApp(t *testing.T) (app *App, dir string) {
	t.Helper()
	dir = t.TempDir()
	page := func(name, next string) string {
		return fmt.Sprintf("# Page %s\n\n[Next](%s) and [elsewhere](d.md).\n", name, next)
	}
	a := writeTempMD(t, dir, "a.md", page("a", "b.md"))
	writeTempMD(t, dir, "b.md", page("b", "c.md"))
	writeTempMD(t, dir, "c.md", page("c", "a.md"))
	writeTempMD(t, dir, "d.md", "# Page d\n")
	app = newTestApp(t, dir, a, page("a", "b.md"))
	app.Follow("b.md")
	app.Follow("c.md")
	return app, dir
}

// titles returns the titles of the pages in the journal's stack.
func titles(app *App) []string {
	var out []string
	for _, v := range app.journal.stack.views {
		title, _ := v.Document().Title()
		out = append(out, title)
	}
	return out
}

// drawApp draws app, so the journal's stack knows where its pages are.
func drawApp(app *App) {
	app.Draw(&recorder{Area: app.Bounds()}, 0)
}

// clickLink clicks the link to dest in the journal's page i, which must
// be on screen.
func clickLink(t *testing.T, app *App, i int, dest string) {
	t.Helper()
	drawApp(app)
	r := app.journal.stack.regions[i]
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if d, ok := app.journal.stack.views[i].LinkAt(x, y); ok && d == dest {
				app.Frame([]input.Event{
					input.PointerMove{X: x, Y: y},
					input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary, Down: true},
					input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary},
				}, 0)
				return
			}
		}
	}
	t.Fatalf("no link to %s on screen in page %d", dest, i)
}

// TestJournalToggle checks that journal mode stacks the pages visited
// above the current one, the same Views, and that turning it off shows
// the current page alone again.
func TestJournalToggle(t *testing.T) {
	app, _ := journalApp(t)
	current := app.Panel.View()
	app.ToggleJournal()
	if !app.Journal() {
		t.Fatal("journal mode isn't on")
	}
	if got := strings.Join(titles(app), ", "); got != "Page a, Page b, Page c" {
		t.Errorf("the journal shows %s, want pages a, b and c", got)
	}
	if views := app.journal.stack.views; views[len(views)-1] != current {
		t.Error("the journal's last page isn't the current page's View")
	}
	if app.CanShowTOC() {
		t.Error("the TOC can be shown in journal mode")
	}
	app.ToggleJournal()
	if app.Journal() || app.Panel.View() != current {
		t.Error("turning journal mode off doesn't show the current page alone")
	}
}

// TestJournalInPageJump checks that a jump within a page doesn't show the
// page twice.
func TestJournalInPageJump(t *testing.T) {
	dir := t.TempDir()
	a := writeTempMD(t, dir, "a.md", "# A\n\n[down](#below)\n\n"+paragraphs(20)+"## Below\n")
	app := newTestApp(t, dir, a, "# A\n\n[down](#below)\n\n"+paragraphs(20)+"## Below\n")
	app.FollowAnchor("below")
	app.ToggleJournal()
	if n := len(app.journal.stack.views); n != 1 {
		t.Errorf("the journal shows %d pages after a jump within one, want 1", n)
	}
}

// TestJournalFollowAndBack checks that a page followed to in journal mode
// joins the stack below the others, and that Back takes it away.
func TestJournalFollowAndBack(t *testing.T) {
	app, _ := journalApp(t)
	app.ToggleJournal()
	clickLink(t, app, 2, "d.md")
	if got := strings.Join(titles(app), ", "); got != "Page a, Page b, Page c, Page d" {
		t.Fatalf("after following a link, the journal shows %s, want pages a to d", got)
	}
	drawApp(app)
	if last := app.journal.stack.regions[3]; last.Empty() || last.Max.Y > app.Bounds().Max.Y {
		t.Errorf("the new page is drawn in %v, want all of it on screen", last)
	}
	app.Back()
	if got := strings.Join(titles(app), ", "); got != "Page a, Page b, Page c" {
		t.Errorf("after Back, the journal shows %s, want pages a, b and c", got)
	}
}

// TestJournalBranch checks that a link followed from an earlier page goes
// on from that page: the pages after it leave history.
func TestJournalBranch(t *testing.T) {
	app, _ := journalApp(t)
	app.ToggleJournal()
	app.journal.stack.Reveal(0)
	clickLink(t, app, 0, "d.md")
	if got := strings.Join(titles(app), ", "); got != "Page a, Page d" {
		t.Errorf("after following a link in page a, the journal shows %s, want pages a and d", got)
	}
	if len(app.history) != 1 || app.CanGoForward() {
		t.Errorf("history has %d entries, forward %v, want just page a, and no forward", len(app.history), app.CanGoForward())
	}
	app.ToggleJournal()
	if title, _ := app.Panel.View().Document().Title(); title != "Page d" {
		t.Errorf("the current page is %q, want Page d", title)
	}
}

// TestJournalHoverDest checks that a link hovered in an earlier page is
// resolved against that page's location.
func TestJournalHoverDest(t *testing.T) {
	app, dir := journalApp(t)
	app.ToggleJournal()
	app.journal.stack.Reveal(0)
	drawApp(app)
	r := app.journal.stack.regions[0]
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if d, ok := app.journal.stack.views[0].LinkAt(x, y); ok && d == "b.md" {
				app.Frame([]input.Event{input.PointerMove{X: x, Y: y}}, 0)
				want := writeTempMD(t, dir, "b.md", "").String()
				if got := app.HoverDest(); got != want {
					t.Errorf("HoverDest = %q, want %q", got, want)
				}
				return
			}
		}
	}
	t.Fatal("no link to b.md on screen in page a")
}

// TestJournalTheme checks that switching theme in journal mode restyles
// every page and the stack's own colors.
func TestJournalTheme(t *testing.T) {
	app, _ := journalApp(t)
	app.ToggleJournal()
	app.SetTheme(false)
	for i, v := range app.journal.stack.views {
		if !usesStyleSheet(v, app.StyleSheet()) {
			t.Errorf("page %d isn't restyled", i)
		}
	}
	background, separator := app.stackColors()
	if app.journal.stack.background != background || app.journal.stack.separator != separator {
		t.Error("the stack's colors aren't the new theme's")
	}
}

// TestJournalKeepsAnchorInPage checks that an anchor link in an earlier
// page scrolls the stack to that heading, without changing history.
func TestJournalKeepsAnchorInPage(t *testing.T) {
	dir := t.TempDir()
	source := "# A\n\n[down](#below)\n\n" + paragraphs(20) + "## Below\n\n[next](b.md)\n"
	a := writeTempMD(t, dir, "a.md", source)
	writeTempMD(t, dir, "b.md", "# B\n\n"+paragraphs(20))
	app := newTestApp(t, dir, a, source)
	app.Follow("b.md")
	app.ToggleJournal()
	app.journal.stack.Reveal(0)
	clickLink(t, app, 0, "#below")
	if len(app.history) != 1 {
		t.Errorf("history has %d entries after an anchor link in an earlier page, want 1", len(app.history))
	}
	if child, _ := app.journal.stack.Position(); child != 0 {
		t.Errorf("the stack shows page %d at its top, want page 0", child)
	}
	if id, _ := app.journal.stack.views[0].CurrentHeadingID(); id != "below" {
		t.Errorf("page 0's current heading is %q, want below", id)
	}
}
