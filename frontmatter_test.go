package whynot

import (
	"encoding/json"
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
			if got := doc.FrontMatter(); got != c.frontMatter {
				t.Errorf("FrontMatter() = %q, want %q", got, c.frontMatter)
			}
			if got, want := rendering(blocksOf(doc.finished)), rendering(blocksOf(Parse([]byte(c.rest)).finished)); got != want {
				t.Errorf("shows %s, want %q as shown on its own", got, c.rest)
			}
		})
	}
}

// TestFrontMatterTitle checks that, with a decoder, the front matter's
// title is the document's, numbers included, and that without one, or
// without a title, the first heading is.
func TestFrontMatterTitle(t *testing.T) {
	decoder := WithFrontMatterDecoder(json.Unmarshal) // JSON is YAML too
	for _, c := range []struct {
		name, source string
		opts         []ParseOption
		want         string
	}{
		{"decoded", "---\n{\"title\": \"Release notes\"}\n---\n# Version two\n", []ParseOption{decoder}, "Release notes"},
		{"a number", "---\n{\"title\": 2024}\n---\n# Version two\n", []ParseOption{decoder}, "2024"},
		{"no title", "---\n{\"tags\": [\"a\"]}\n---\n# Version two\n", []ParseOption{decoder}, "Version two"},
		{"not a mapping", "---\n[\"a\"]\n---\n# Version two\n", []ParseOption{decoder}, "Version two"},
		{"no decoder", "---\n{\"title\": \"Release notes\"}\n---\n# Version two\n", nil, "Version two"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := Parse([]byte(c.source), c.opts...).Title(); got != c.want {
				t.Errorf("Title() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestDecodeFrontMatter checks that DecodeFrontMatter fills the program's
// struct through the decoder, leaves it alone for a document without
// front matter, and fails without a decoder.
func TestDecodeFrontMatter(t *testing.T) {
	source := []byte("---\n{\"tags\": [\"go\", \"markdown\"], \"draft\": true}\n---\nText\n")
	var meta struct {
		Tags  []string
		Draft bool
	}
	if err := Parse(source, WithFrontMatterDecoder(json.Unmarshal)).DecodeFrontMatter(&meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Tags) != 2 || meta.Tags[1] != "markdown" || !meta.Draft {
		t.Errorf("decoded %+v", meta)
	}
	if err := Parse([]byte("Text\n"), WithFrontMatterDecoder(json.Unmarshal)).DecodeFrontMatter(&meta); err != nil {
		t.Errorf("a document without front matter: %v", err)
	}
	if err := Parse(source).DecodeFrontMatter(&meta); err == nil {
		t.Error("decoding without a decoder succeeded")
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
	if got := doc.FrontMatter(); got != "title: Notes\n" {
		t.Errorf("FrontMatter() = %q", got)
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
