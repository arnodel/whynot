package whynot

import (
	"io"
	"testing"
)

// TestFrontMatter checks that front matter isn't shown and is available
// as written, and that what merely looks like it is shown: a thematic
// break followed by a blank line, or a "---" never closed.
func TestFrontMatter(t *testing.T) {
	for _, c := range []struct {
		name, source      string
		frontMatter, rest string // what's shown: rest, parsed on its own
	}{
		{"front matter", "---\ntitle: Notes\ntags: [a, b]\n---\n# Heading\n", "title: Notes\ntags: [a, b]\n", "# Heading\n"},
		{"closed with ...", "---\ntitle: Notes\n...\nText\n", "title: Notes\n", "Text\n"},
		{"empty", "---\n---\nText\n", "", "Text\n"},
		{"a thematic break", "---\n\nText\n", "", "---\n\nText\n"},
		{"never closed", "---\ntitle: Notes\n", "", "---\ntitle: Notes\n"},
		{"none", "# Heading\n", "", "# Heading\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := Parse([]byte(c.source))
			if got := doc.RawFrontMatter(); got != c.frontMatter {
				t.Errorf("RawFrontMatter() = %q, want %q", got, c.frontMatter)
			}
			if got, want := rendering(blocksOf(doc.finished)), rendering(blocksOf(Parse([]byte(c.rest)).finished)); got != want {
				t.Errorf("shows %s, want %q as shown on its own", got, c.rest)
			}
		})
	}
}

// TestFrontMatterNotRead checks that whynot doesn't read front matter: a
// title in it isn't the document's, which is still its first heading.
func TestFrontMatterNotRead(t *testing.T) {
	doc := Parse([]byte("---\ntitle: Release notes\n---\n# Version two\n"))
	if title, _ := doc.Title(); title != "Version two" {
		t.Errorf("Title() = %q, want the first heading's", title)
	}
}

// TestStreamFrontMatter checks that, while the start of a growing
// document may still be front matter, nothing of it is shown, and that
// once it's known, front matter isn't shown and a thematic break is.
func TestStreamFrontMatter(t *testing.T) {
	doc, w := NewParser().Stream()
	io.WriteString(w, "---\ntitle: Notes\n")
	if blocks := current(doc); len(blocks) != 0 {
		t.Errorf("while the front matter may continue, %d blocks show, want none", len(blocks))
	}
	io.WriteString(w, "---\n# Heading\n")
	if got, want := rendering(current(doc)), rendering(blocksOf(Parse([]byte("# Heading\n")).finished)); got != want {
		t.Errorf("once the front matter is closed, the document shows %s, want only the heading", got)
	}
	if got := doc.RawFrontMatter(); got != "title: Notes\n" {
		t.Errorf("RawFrontMatter() = %q", got)
	}

	doc, w = NewParser().Stream()
	io.WriteString(w, "--")
	if blocks := current(doc); len(blocks) != 0 {
		t.Errorf("\"--\" may become front matter: %d blocks show, want none", len(blocks))
	}
	io.WriteString(w, "-\n\nText\n")
	if got, want := rendering(current(doc)), rendering(blocksOf(Parse([]byte("---\n\nText\n")).finished)); got != want {
		t.Errorf("a thematic break then a blank line shows %s, want both", got)
	}
}
