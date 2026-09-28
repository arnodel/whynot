package whynot

import "slices"

// Document is a parsed Markdown document: its Block tree plus what the
// compiler recorded about its top-level structure along the way. Built by
// Parse, immutable afterward, and renderable by any number of Views.
type Document struct {
	root *StackBlock

	// headings is every top-level heading, in document order.
	headings []TOCEntry

	// soleImages maps each top-level text block consisting of a single
	// image to that image's src - what View prefetches ahead of scrolling.
	soleImages map[Block]string
}

// TOCEntry is one heading of a Document - enough to build a table of
// contents entry linking to ID.
type TOCEntry struct {
	ID    string // ASTNode.ID - what View.ScrollToAnchor takes
	Level int    // 1-6, from TagHeading1..TagHeading6
	Text  string
}

// Root returns the document's Block tree, ready for GetBlockLayout.
func (d *Document) Root() Block {
	return d.root
}

// Title returns the text of the document's first heading, at any level,
// or ok=false if it has none.
//
// Only a top-level heading is found: one nested inside a blockquote or
// list isn't. The same holds for TOCEntries and View.ScrollToAnchor.
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
