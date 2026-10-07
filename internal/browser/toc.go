package browser

import (
	"strings"

	"github.com/arnodel/whynot"
)

// TOCShowing reports whether ShowTOC is currently in effect - see its
// own doc comment for what that means for Back/Forward/Reload/Navigate.
func (a *App) TOCShowing() bool { return a.overlay == tocOverlay }

// CanShowTOC reports whether ShowTOC would do anything - false for a
// document with no headings (see whynot.Document.TOCEntries). Checks
// the real document, even while an overlay is showing.
func (a *App) CanShowTOC() bool {
	return len(a.documentView().Document().TOCEntries()) > 0
}

// ShowTOC replaces the current document in Panel with a synthetic
// table-of-contents document: a nested list of links, one per heading,
// generated as ordinary Markdown and parsed like any other document -
// so scrolling, hit-testing, and clicking a link (see Follow) all just
// work, with no new rendering/input code needed. The entry for the
// section currently on screen is rendered in bold and scrolled into
// view (View.CurrentHeadingID).
//
// It replaces the source, if that's showing. A no-op if the TOC is
// already showing, or the document has no headings (see CanShowTOC).
func (a *App) ShowTOC() {
	if a.overlay == tocOverlay || !a.CanShowTOC() {
		return
	}
	docView := a.documentView()
	entries := docView.Document().TOCEntries()
	currentID, _ := docView.CurrentHeadingID()

	tocView := whynot.NewView(
		whynot.Parse(buildTOCSource(entries, currentID)),
		whynot.WithFaceSelector(a.faceSelector),
		whynot.WithStyleSheet(a.styleSheet),
	)
	a.place(tocView)
	if currentID != "" {
		tocView.ScrollToAnchor(currentID)
	}

	a.showOverlay(tocOverlay, tocView)
}

// HideTOC is HideOverlay, if the TOC is showing.
func (a *App) HideTOC() {
	if a.overlay == tocOverlay {
		a.HideOverlay()
	}
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
