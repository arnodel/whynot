package markdown

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arnodel/whynot/internal/engine"
)

// tags describes blocks by their ast.Node tags, to compare what two texts
// compile to.
func tags(blocks []engine.Block) string {
	var s []string
	for _, b := range blocks {
		if n := b.Node(); n != nil {
			s = append(s, fmt.Sprint(n.Tag))
		} else {
			s = append(s, "?")
		}
	}
	return strings.Join(s, " ")
}

func pieceBlocks(pieces []Piece) []engine.Block {
	blocks := make([]engine.Block, len(pieces))
	for i, p := range pieces {
		blocks[i] = p.Block
	}
	return blocks
}

// frontMatterCase is a text, and what decideFrontMatter makes of it.
type frontMatterCase struct {
	name string
	text string
	// open says whether it's decided before Close, and if so, whether it's
	// front matter; frontMatter is what it is once decided, and rest what
	// the text compiles to besides, as Compile of rest would.
	openDecided bool
	frontMatter string // "" for none
	found       bool   // front matter, maybe empty, rather than none
	rest        string
}

// longFrontMatter returns front matter whose closing fence is on line
// closing (counting from 0, the opening fence).
func longFrontMatter(closing int) string {
	var b strings.Builder
	b.WriteString("---\n")
	for i := 1; i < closing; i++ {
		fmt.Fprintf(&b, "key%d: value\n", i)
	}
	b.WriteString("---\nText\n")
	return b.String()
}

var frontMatterCases = []frontMatterCase{
	{name: "empty text", text: "", openDecided: false, rest: ""},
	{name: "another character", text: "I", openDecided: true, rest: "I"},
	{name: "a dash, then something else", text: "-x", openDecided: true, rest: "-x"},
	{name: "a dash", text: "-", openDecided: false, rest: "-"},
	{name: "two dashes", text: "--", openDecided: false, rest: "--"},
	{name: "a fence, unfinished", text: "---", openDecided: false, rest: "---"},
	{name: "a fence", text: "---\n", openDecided: false, rest: "---\n"},
	{name: "a thematic break, then a blank line", text: "---\n\nText\n", openDecided: true, rest: "---\n\nText\n"},
	{name: "never closed", text: "---\ntitle: x\nmore: y\n", openDecided: false, rest: "---\ntitle: x\nmore: y\n"},
	{name: "front matter", text: "---\ntitle: x\n---\n# Heading\n", openDecided: true, found: true,
		frontMatter: "title: x\n", rest: "# Heading\n"},
	{name: "closing fence unfinished", text: "---\ntitle: x\n---", openDecided: false, found: true,
		frontMatter: "title: x\n", rest: ""},
	{name: "closed with ...", text: "---\ntitle: x\n...\nText\n", openDecided: true, found: true,
		frontMatter: "title: x\n", rest: "Text\n"},
	{name: "empty front matter", text: "---\n---\nText\n", openDecided: true, found: true,
		frontMatter: "", rest: "Text\n"},
	{name: "CRLF line ends", text: "---\r\ntitle: x\r\n---\r\nText\r\n", openDecided: true, found: true,
		frontMatter: "title: x\r\n", rest: "Text\r\n"},
	{name: "trailing spaces on fences", text: "--- \ntitle: x\n---  \nText\n", openDecided: true, found: true,
		frontMatter: "title: x\n", rest: "Text\n"},
	{name: "a longer opening fence", text: "----\ntitle: x\n----\nText\n", openDecided: true, rest: "----\ntitle: x\n----\nText\n"},
	{name: "an indented opening fence", text: " ---\ntitle: x\n---\nText\n", openDecided: true, rest: " ---\ntitle: x\n---\nText\n"},
	{name: "not at the start", text: "Text\n---\ntitle: x\n---\n", openDecided: true, rest: "Text\n---\ntitle: x\n---\n"},
	{name: "multi-line YAML", text: "---\ntitle: >\n  folded\n  title\ntags:\n  - a\n---\nText\n", openDecided: true, found: true,
		frontMatter: "title: >\n  folded\n  title\ntags:\n  - a\n", rest: "Text\n"},
	{name: "closing fence on the last line looked at", text: longFrontMatter(frontMatterLines - 1), openDecided: true, found: true,
		frontMatter: strings.TrimSuffix(strings.TrimPrefix(longFrontMatter(frontMatterLines-1), "---\n"), "---\nText\n"), rest: "Text\n"},
	{name: "closing fence one line too far", text: longFrontMatter(frontMatterLines), openDecided: true,
		rest: longFrontMatter(frontMatterLines)},
}

// TestFrontMatterDecision checks decideFrontMatter on each case, written
// in one go and closed, written in one go and still open, and written a
// byte at a time with an Update after each: a decision is only made when
// the text settles it, never changes once made, and comes out the same
// however the text arrives.
func TestFrontMatterDecision(t *testing.T) {
	for _, c := range frontMatterCases {
		t.Run(c.name, func(t *testing.T) {
			want := tags(Compile([]byte(c.rest), Options{}).Root.Blocks)
			check := func(how string, s *Stream, finished []Piece) {
				t.Helper()
				if !s.frontMatterDecided {
					t.Errorf("%s: undecided after Close", how)
				}
				if got := s.FrontMatter(); got != c.frontMatter {
					t.Errorf("%s: FrontMatter() = %q, want %q", how, got, c.frontMatter)
				}
				if got := tags(pieceBlocks(finished)); got != want {
					t.Errorf("%s: compiles to %q, want %q, as %q does", how, got, want, c.rest)
				}
			}

			// Whole, and closed.
			s := NewStream(Options{})
			s.Write([]byte(c.text))
			s.Close()
			finished, _ := s.Update()
			check("whole", s, finished)

			// Whole, and still open.
			s = NewStream(Options{})
			s.Write([]byte(c.text))
			_, tail := s.Update()
			if s.frontMatterDecided != c.openDecided {
				t.Errorf("open: decided = %v, want %v", s.frontMatterDecided, c.openDecided)
			}
			if !s.frontMatterDecided && len(tail) != 0 {
				t.Errorf("open and undecided, %d blocks show, want none", len(tail))
			}
			if s.frontMatterDecided && c.found && s.FrontMatter() != c.frontMatter {
				t.Errorf("open: FrontMatter() = %q, want %q", s.FrontMatter(), c.frontMatter)
			}

			// A byte at a time.
			s = NewStream(Options{})
			var all []Piece
			decided, frontMatter := false, ""
			for i := range len(c.text) {
				s.Write([]byte{c.text[i]})
				done, _ := s.Update()
				all = append(all, done...)
				if decided && (!s.frontMatterDecided || s.FrontMatter() != frontMatter) {
					t.Fatalf("a byte at a time: after %d bytes, the decision changed", i+1)
				}
				decided, frontMatter = s.frontMatterDecided, s.FrontMatter()
			}
			s.Close()
			done, _ := s.Update()
			check("a byte at a time", s, append(all, done...))
		})
	}
}
