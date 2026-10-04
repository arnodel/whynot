package whynot

import (
	"slices"

	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/engine"
)

// Document is a parsed Markdown document, made by Parse. It's immutable,
// and any number of Views can show it.
type Document struct {
	root *engine.StackBlock

	// headings is every top-level heading, in document order.
	headings []TOCEntry

	// soleImages maps each top-level text block consisting of a single
	// image to that image's Source - what View prefetches ahead of
	// scrolling.
	soleImages map[engine.Block]fetch.Source
}

// TOCEntry is one heading of a Document - enough to build a table of
// contents entry linking to ID.
type TOCEntry struct {
	ID    string // the heading's anchor id - what View.ScrollToAnchor takes
	Level int    // 1-6
	Text  string
}

// Title returns the text of the document's first heading, at any level,
// or ok=false if it has none.
//
// Only a top-level heading is found: one nested inside a blockquote or
// list isn't. The same holds for TOCEntries and [View.ScrollToAnchor].
func (d *Document) Title() (string, bool) {
	if len(d.headings) == 0 {
		return "", false
	}
	return d.headings[0].Text, true
}

// TOCEntries returns every top-level heading in the document, in document
// order.
func (d *Document) TOCEntries() []TOCEntry {
	return slices.Clone(d.headings)
}
