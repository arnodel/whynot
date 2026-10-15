package markdown

import (
	"strings"
	"testing"

	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
)

// sourced is what a compiled document records of where things are: every
// node reachable from its blocks and words, and its words.
type sourced struct {
	nodes []*ast.Node
	words []*engine.InlineText
}

// collect gathers the nodes and words of blocks.
func collect(blocks []engine.Block) sourced {
	var s sourced
	seen := map[*ast.Node]bool{}
	addNode := func(n *ast.Node) {
		for ; n != nil && !seen[n]; n = n.Parent {
			seen[n] = true
			s.nodes = append(s.nodes, n)
		}
	}
	addInlines := func(items []engine.Inline) {
		for _, item := range items {
			addNode(item.Node())
			if w, ok := item.(*engine.InlineText); ok {
				s.words = append(s.words, w)
			}
		}
	}
	var walk func(b engine.Block)
	walk = func(b engine.Block) {
		if b == nil {
			return
		}
		addNode(b.Node())
		switch b := b.(type) {
		case *engine.MarginBlock:
			addNode(b.MarginNode)
			walk(b.Block)
		case *engine.StackBlock:
			for _, child := range b.Blocks {
				walk(child)
			}
		case *engine.TextBlock:
			addInlines(b.Parts)
		case *engine.ListItemHeadBlock:
			addInlines(b.Parts)
		case *engine.BlockquoteBlock:
			walk(b.Inner)
		case *engine.CodeBlock:
			for _, line := range b.Lines {
				addInlines(line)
			}
		case *engine.TableBlock:
			for _, row := range append([][]engine.TableCell{b.Header}, b.Rows...) {
				for _, cell := range row {
					walk(cell.Content)
				}
			}
		}
	}
	for _, b := range blocks {
		walk(b)
	}
	return s
}

// compileWhole compiles source as a document arriving at once.
func compileWhole(source string) sourced {
	s := NewStream(Options{})
	s.Write([]byte(source))
	s.Close()
	finished, _ := s.Update()
	return collect(pieceBlocks(finished))
}

// sources returns the source of each node of s tagged tag, in document
// order.
func (s sourced) sources(source string, tag ast.Tag) []string {
	var out []string
	for _, n := range s.nodes {
		if n.Tag == tag && n.Source != (ast.Range{}) {
			out = append(out, source[n.Source.Start:n.Source.End])
		}
	}
	return out
}

// wantSources checks that the nodes of s tagged tag have exactly want as
// their sources, in any order.
func wantSources(t *testing.T, s sourced, source string, tag ast.Tag, want ...string) {
	t.Helper()
	got := s.sources(source, tag)
	missing := map[string]int{}
	for _, w := range want {
		missing[w]++
	}
	for _, g := range got {
		missing[g]--
	}
	for w, n := range missing {
		if n != 0 {
			t.Errorf("tag %d: sources %q, want %q (%q off by %d)", tag, got, want, w, n)
			return
		}
	}
}

// TestInlineSources checks the sources of inline elements: their markup
// included, nested or not.
func TestInlineSources(t *testing.T) {
	const source = "Some **bold _and_ x** [a *b*](http://e.com \"t (x)\") and `` co`de `` ~~gone~~ ![alt *y*](i.png) <http://x.y> [ref] end.\n\n[ref]: http://r.example\n"
	s := compileWhole(source)
	wantSources(t, s, source, ast.TagStrong, "**bold _and_ x**")
	wantSources(t, s, source, ast.TagEmphasis, "_and_", "*b*")
	wantSources(t, s, source, ast.TagLink, `[a *b*](http://e.com "t (x)")`, "<http://x.y>", "[ref]")
	wantSources(t, s, source, ast.TagCodeSpan, "`` co`de ``")
	wantSources(t, s, source, ast.TagStrikethrough, "~~gone~~")
	wantSources(t, s, source, ast.TagImage, "![alt *y*](i.png)")
}

// TestBlockSources checks the sources of blocks: their markers and fences
// included, without the blank lines after them, and nested blocks within
// their parents.
func TestBlockSources(t *testing.T) {
	const source = "# Title #\n\nA paragraph\non two lines.\n\n" +
		"- one\n- two\n\n  more of two\n\n  - nested\n\n" +
		"> quoted\n> more\n\n" +
		"```go\nx := 1\n```\n\n" +
		"| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
		"***\n\nSetext\n======\n"
	s := compileWhole(source)
	wantSources(t, s, source, ast.TagHeading1, "# Title #", "Setext\n======")
	wantSources(t, s, source, ast.TagParagraph, "A paragraph\non two lines.", "more of two", "quoted\n> more")
	wantSources(t, s, source, ast.TagList, "- one\n- two\n\n  more of two\n\n  - nested", "- nested")
	wantSources(t, s, source, ast.TagListItem, "- one", "- two\n\n  more of two\n\n  - nested", "- nested")
	wantSources(t, s, source, ast.TagBlockquote, "> quoted\n> more")
	wantSources(t, s, source, ast.TagCodeBlock, "```go\nx := 1\n```")
	wantSources(t, s, source, ast.TagTable, "| a | b |\n|---|---|\n| 1 | 2 |")
	wantSources(t, s, source, ast.TagTableCell, "a", "b", "1", "2")
	wantSources(t, s, source, ast.TagThematicBreak, "***")
}

// TestWordSources checks words' sources: exact where the text is the
// source, and otherwise the source of the text the word is part of.
func TestWordSources(t *testing.T) {
	const source = "Plain words, `code here`, a\\*b and x&amp;y and \"quoted\" text.\n\n```\nline one\n\tindented\n```\n"
	s := compileWhole(source)
	// The words whose text isn't their source, and what their source
	// holds.
	inexact := map[string]string{"a*b": `a\*b`, "x&y": "x&amp;y", "    indented\n": "\tindented", "“": `"`, "”": `"`}
	// Those whose source is known exactly, though it isn't their text.
	precise := map[string]bool{"a*b": true, "x&y": true}
	sawCoarse := map[string]bool{}
	for _, w := range s.words {
		if w.Source == (ast.Range{}) {
			t.Errorf("word %q has no source", w.Text)
			continue
		}
		got := source[w.Source.Start:w.Source.End]
		if w.SourceExact {
			if got != w.Text {
				t.Errorf("word %q: exact source %q", w.Text, got)
			}
			continue
		}
		sawCoarse[w.Text] = true
		raw, ok := inexact[w.Text]
		if !ok || !strings.Contains(got, raw) || precise[w.Text] && got != raw {
			t.Errorf("word %q: inexact source %q, want %q", w.Text, got, raw)
		}
	}
	for word := range inexact {
		if !sawCoarse[word] {
			t.Errorf("word %q should have an inexact source", word)
		}
	}
}

// TestStreamedSources checks that a document arriving a piece at a time
// records sources as offsets in the whole text, for its finished blocks
// and its unfinished end alike.
func TestStreamedSources(t *testing.T) {
	const source = "# One\n\nFirst *para*.\n\n- a\n- b\n\nLast **para** here.\n"
	s := NewStream(Options{})
	var finished []Piece
	var tail []Piece
	for i := 0; i < len(source); i += 7 {
		s.Write([]byte(source[i:min(i+7, len(source))]))
		f, tl := s.Update()
		finished, tail = append(finished, f...), tl
	}
	got := collect(append(pieceBlocks(finished), pieceBlocks(tail)...))
	wantSources(t, got, source, ast.TagHeading1, "# One")
	wantSources(t, got, source, ast.TagEmphasis, "*para*")
	wantSources(t, got, source, ast.TagListItem, "- a", "- b")
	wantSources(t, got, source, ast.TagStrong, "**para**")
	for _, w := range got.words {
		if w.SourceExact && source[w.Source.Start:w.Source.End] != w.Text {
			t.Errorf("word %q: source %q", w.Text, source[w.Source.Start:w.Source.End])
		}
	}
}
