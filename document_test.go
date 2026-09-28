package whynot

import (
	"slices"
	"testing"
)

// TestDocumentTitle checks that Title finds the document's first heading,
// at any level, skipping non-heading blocks before it, and joins a
// multi-word heading's words back into a single string.
func TestDocumentTitle(t *testing.T) {
	doc := Parse([]byte("Some intro text, before any heading.\n\n## A Heading\n\nMore text.\n"))
	title, ok := doc.Title()
	if !ok || title != "A Heading" {
		t.Errorf("Title() = %q, %v, want %q, true", title, ok, "A Heading")
	}
}

// TestDocumentTitleNoHeading checks that Title reports ok=false for a
// document with no heading at all, rather than an empty string being
// mistaken for a real (if blank) title.
func TestDocumentTitleNoHeading(t *testing.T) {
	doc := Parse([]byte("Just a paragraph, no heading anywhere.\n"))
	if title, ok := doc.Title(); ok {
		t.Errorf("Title() = %q, true, want ok=false", title)
	}
}

// TestDocumentTOCEntries checks that every top-level heading is reported,
// in document order, with its level and auto-assigned id - and that a
// heading nested inside a blockquote is not.
func TestDocumentTOCEntries(t *testing.T) {
	doc := Parse([]byte("# One\n\nText.\n\n> ## Quoted\n\n## Two\n\nText.\n\n### Three\n\nText.\n"))
	want := []TOCEntry{
		{ID: "one", Level: 1, Text: "One"},
		{ID: "two", Level: 2, Text: "Two"},
		{ID: "three", Level: 3, Text: "Three"},
	}
	if got := doc.TOCEntries(); !slices.Equal(got, want) {
		t.Errorf("TOCEntries() = %+v, want %+v", got, want)
	}
}

// TestDocumentTOCEntriesNoHeadings checks that a document with no headings
// reports no entries, rather than e.g. a single bogus one.
func TestDocumentTOCEntriesNoHeadings(t *testing.T) {
	doc := Parse([]byte("Just a paragraph, no heading anywhere.\n"))
	if entries := doc.TOCEntries(); len(entries) != 0 {
		t.Errorf("TOCEntries() = %+v, want none", entries)
	}
}

// TestDocumentSoleImages checks that only a top-level paragraph whose
// entire content is one image is recorded - not an image mixed into
// text, and not one nested inside a blockquote.
func TestDocumentSoleImages(t *testing.T) {
	doc := Parse([]byte("![a](a.png)\n\ntext ![b](b.png)\n\n> ![c](c.png)\n"))
	if len(doc.soleImages) != 1 {
		t.Fatalf("soleImages = %v, want exactly one entry", doc.soleImages)
	}
	if src, ok := doc.soleImages[doc.root.blocks[0]]; !ok || src != "a.png" {
		t.Errorf("soleImages[first block] = %q, %v, want %q, true", src, ok, "a.png")
	}
}
