package browser

import (
	"strconv"
	"strings"

	"github.com/arnodel/whynot"
)

// TOCShowing reports whether ShowTOC is currently in effect - see its
// own doc comment for what that means for Back/Forward/Reload/Navigate.
func (a *App) TOCShowing() bool { return a.tocDocView != nil }

// CanShowTOC reports whether ShowTOC would do anything - false for a
// document with no headings (see whynot.Document.TOCEntries). Checks
// whichever View is the real document right now: a.tocDocView while
// the TOC is already showing, a.Panel.View() otherwise.
func (a *App) CanShowTOC() bool {
	if a.journal != nil {
		return true // the journal's pages are entries themselves
	}
	view := a.Panel.View()
	if a.tocDocView != nil {
		view = a.tocDocView
	}
	return len(view.Document().TOCEntries()) > 0
}

// ShowTOC replaces the current document in Panel with a synthetic
// table-of-contents document: a nested list of links, one per heading,
// generated as ordinary Markdown and parsed like any other document -
// so scrolling, hit-testing, and clicking a link (see Follow) all just
// work, with no new rendering/input code needed. The entry for the
// section currently on screen is rendered in bold and scrolled into
// view (View.CurrentHeadingID).
//
// A no-op if the TOC is already showing, or the document has no
// headings (see CanShowTOC).
func (a *App) ShowTOC() {
	if a.tocDocView != nil || !a.CanShowTOC() {
		return
	}
	docView := a.Panel.View()
	var source []byte
	if j := a.journal; j != nil {
		source = a.journalTOCSource()
		// The Panel shows the TOC, as it would without the journal.
		a.Panel.SetBounds(a.bounds)
		a.Panel.SetScale(a.deviceScale)
		a.Panel.SetZoom(a.zoom)
	} else {
		currentID, _ := docView.CurrentHeadingID()
		source = buildTOCSource(docView.Document().TOCEntries(), currentID)
	}

	tocView := whynot.NewView(
		whynot.Parse(source),
		whynot.WithFaceSelector(a.faceSelector),
		whynot.WithStyleSheet(a.styleSheet),
	)
	a.place(tocView)

	a.tocDocView = docView
	a.arrival = arrival{}
	a.Panel.SetView(tocView)
	a.updateWindowTitle()
}

// HideTOC restores the document ShowTOC replaced, exactly where it was
// left (ShowTOC/HideTOC never touch its scroll position) - a no-op if
// the TOC isn't showing.
func (a *App) HideTOC() {
	if a.tocDocView == nil {
		return
	}
	docView := a.tocDocView
	a.Panel.SetView(docView) // re-applies a.styleSheet, then relayouts
	a.tocDocView = nil
	if a.journal != nil {
		a.prepare(docView) // a page of the stack again
	}
	a.updateWindowTitle()
}

// journalTOCSource generates the Markdown source for journal mode's table
// of contents: an entry for each page of the trail, its title, with its
// headings nested below it. A page's link is "#i/", and a heading's
// "#i/id", for page i (see followJournalTOC). The entry for where the
// reader is, the heading or page at the top of the stack, is in bold.
func (a *App) journalTOCSource() []byte {
	j := a.journal
	top, _ := j.stack.Position()
	topID, _ := j.stack.views[top].CurrentHeadingID()
	var b strings.Builder
	b.WriteString("# Journal\n\n")
	for i, v := range j.stack.views {
		doc := v.Document()
		title, ok := a.documentTitle(doc)
		if !ok {
			title = "Untitled document"
		}
		entries := doc.TOCEntries()
		// A page titled by its first heading would list it twice.
		if len(entries) > 0 && entries[0].Text == title {
			if i == top && topID == entries[0].ID {
				topID = ""
			}
			entries = entries[1:]
		}
		link := "[" + escapeMarkdown(title) + "](#" + strconv.Itoa(i) + "/)"
		if i == top && topID == "" {
			link = "**" + link + "**"
		}
		b.WriteString("- " + link + "\n")
		for _, e := range entries {
			b.WriteString(strings.Repeat("  ", e.Level))
			link := "[" + escapeMarkdown(e.Text) + "](#" + strconv.Itoa(i) + "/" + e.ID + ")"
			if i == top && e.ID == topID {
				link = "**" + link + "**"
			}
			b.WriteString("- " + link + "\n")
		}
	}
	return []byte(b.String())
}

// followJournalTOC follows a link of journal mode's table of contents,
// whose fragment is "i/id" (see journalTOCSource): it closes the TOC, and
// scrolls the stack to the heading with that id in page i, or to the
// page's start for an empty id. It reports whether fragment was one.
func (a *App) followJournalTOC(fragment string) bool {
	page, id, ok := strings.Cut(fragment, "/")
	i, err := strconv.Atoi(page)
	if !ok || err != nil || i < 0 || i >= len(a.journal.stack.views) {
		return false
	}
	a.HideTOC()
	a.arrival = arrival{}
	a.journal.stack.scrollToFragment(i, id)
	return true
}

// buildTOCSource generates the Markdown source for a table-of-contents
// document from entries: a heading, then a nested list with one
// "- [text](#id)" item per entry, indented by Level so goldmark nests
// it the same way the real document's own heading levels do. The entry
// matching currentID (if any) is wrapped in **bold** - ordinary Markdown
// emphasis, not a new rendering feature.
func buildTOCSource(entries []whynot.TOCEntry, currentID string) []byte {
	var b strings.Builder
	b.WriteString("# Table of contents\n\n")
	for _, e := range entries {
		b.WriteString(strings.Repeat("  ", e.Level-1))
		b.WriteString("- ")
		link := "[" + escapeMarkdown(e.Text) + "](#" + e.ID + ")"
		if e.ID == currentID {
			link = "**" + link + "**"
		}
		b.WriteString(link)
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// markdownEscaper backslash-escapes the CommonMark characters that
// would otherwise change buildTOCSource's generated list/link/bold
// structure if they appeared literally in a heading's own text (e.g. a
// heading actually titled "A * B") - TOCEntry.Text is plain text, not
// Markdown.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	"*", `\*`,
	"_", `\_`,
	"[", `\[`,
	"]", `\]`,
)

func escapeMarkdown(s string) string { return markdownEscaper.Replace(s) }
