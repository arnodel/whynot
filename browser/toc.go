package browser

import (
	"strings"

	"github.com/arnodel/whynot"
)

// TOCShowing reports whether ShowTOC is currently in effect - see its
// own doc comment for what that means for Back/Forward/Reload/Navigate.
func (a *App) TOCShowing() bool { return a.tocDocView != nil }

// CanShowTOC reports whether ShowTOC would do anything - false for a
// document with no headings (see whynot.View.TOCEntries). Checks
// whichever View is the real document right now: a.tocDocView while
// the TOC is already showing, a.Panel.View() otherwise.
func (a *App) CanShowTOC() bool {
	view := a.Panel.View()
	if a.tocDocView != nil {
		view = a.tocDocView
	}
	return len(view.TOCEntries()) > 0
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
	entries := docView.TOCEntries()
	currentID, _ := docView.CurrentHeadingID()

	tocView := whynot.NewView(buildTOCSource(entries, currentID), a.faceSelector,
		whynot.WithStyleSheet(a.styleSheet))
	tocView.Layout(a.width, a.height-a.toolbarHeight, a.scale, a.elapsed())
	if currentID != "" {
		tocView.ScrollToAnchor(currentID)
	}

	a.tocDocView = docView
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
	a.Panel.SetView(a.tocDocView) // re-applies a.styleSheet, then relayouts
	a.tocDocView = nil
	a.updateWindowTitle()
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
