package whynot

import (
	"strings"
	"testing"

	"github.com/arnodel/whynot/canvas/canvastest"
	"github.com/arnodel/whynot/internal/engine"
)

// at returns the Position in v before the byte at offset in the nth word
// (from 0) whose text is word.
func at(t *testing.T, v *View, word string, nth, offset int) engine.Position {
	t.Helper()
	tree := v.selTree()
	var found []engine.Position
	for _, b := range v.blocks {
		for _, w := range words(b) {
			if w.Text == word {
				if p, ok := tree.PositionOf(w, offset); ok {
					found = append(found, p)
				}
			}
		}
	}
	if nth >= len(found) {
		t.Fatalf("no word %q number %d (found %d)", word, nth, len(found))
	}
	return found[nth]
}

// words returns the words under b, in order.
func words(b engine.Block) []*engine.InlineText {
	var out []*engine.InlineText
	add := func(items []engine.Inline) {
		for _, item := range items {
			if w, ok := item.(*engine.InlineText); ok {
				out = append(out, w)
			}
		}
	}
	switch b := b.(type) {
	case *engine.MarginBlock:
		return words(b.Block)
	case *engine.StackBlock:
		for _, c := range b.Blocks {
			out = append(out, words(c)...)
		}
	case *engine.BlockquoteBlock:
		return words(b.Inner)
	case *engine.TextBlock:
		add(b.Parts)
	case *engine.ListItemHeadBlock:
		add(b.Parts)
	case *engine.CodeBlock:
		for _, l := range b.Lines {
			add(l)
		}
	case *engine.TableBlock:
		for _, row := range append([][]engine.TableCell{b.Header}, b.Rows...) {
			for _, c := range row {
				add(c.Content.Parts)
			}
		}
	}
	return out
}

// TestSelection checks the selection model through the View: ranges within
// a block, whole blocks across them, and both kinds of extraction.
func TestSelection(t *testing.T) {
	type end struct {
		word        string
		nth, offset int
	}
	for _, c := range []struct {
		name, source   string
		from, to       end
		text, markdown string
	}{
		{
			name:   "within a paragraph, across emphasis",
			source: "Some **bold text** here.\n",
			from:   end{"Some", 0, 2}, to: end{"here", 0, 2},
			text: "me bold text he", markdown: "me **bold text** he",
		},
		{
			name:   "within emphasis, widened",
			source: "Some **bold text** here.\n",
			from:   end{"bold", 0, 2}, to: end{"text", 0, 2},
			text: "ld te", markdown: "**bold text**",
		},
		{
			name:   "a link, widened",
			source: "See [the docs](http://e.com) now.\n",
			from:   end{"See", 0, 0}, to: end{"docs", 0, 2},
			text: "See the do", markdown: "See [the docs](http://e.com)",
		},
		{
			name:   "an escaped word",
			source: "A a\\*b c.\n",
			from:   end{"a*b", 0, 0}, to: end{"c", 0, 1},
			text: "a*b c", markdown: "a\\*b c",
		},
		{
			name:   "across paragraphs",
			source: "Para one.\n\nPara two.\n\nPara three.\n",
			from:   end{"one", 0, 2}, to: end{"Para", 1, 1},
			text: "Para one.\n\nPara two.", markdown: "Para one.\n\nPara two.",
		},
		{
			name:   "list items",
			source: "- a\n- b\n- c\n- d\n",
			from:   end{"b", 0, 0}, to: end{"c", 0, 1},
			text: "- b\n- c", markdown: "- b\n- c",
		},
		{
			name:   "paragraphs in a blockquote",
			source: "> One.\n>\n> Two.\n\nAfter.\n",
			from:   end{"One", 0, 1}, to: end{"Two", 0, 1},
			text: "One.\n\nTwo.", markdown: "> One.\n>\n> Two.",
		},
		{
			name:   "table rows",
			source: "| h1 | h2 |\n|:---|---:|\n| a | b |\n| c | d |\n| e | f |\n",
			from:   end{"b", 0, 0}, to: end{"c", 0, 1},
			text: "a\tb\nc\td", markdown: "| h1 | h2 |\n| :--- | ---: |\n| a | b |\n| c | d |",
		},
		{
			name:   "cells of a row",
			source: "| h1 | h2 | h3 |\n|---|---|---|\n| a | b | c |\n",
			from:   end{"b", 0, 0}, to: end{"c", 0, 1},
			text: "b\tc", markdown: "| h2 | h3 |\n| --- | --- |\n| b | c |",
		},
		{
			name:   "within a code block",
			source: "```\nline one\nline two\n```\n",
			from:   end{"line one\n", 0, 2}, to: end{"line two\n", 0, 7},
			text: "ne one\nline tw", markdown: "ne one\nline tw",
		},
	} {
		v := NewView(Parse([]byte(c.source)))
		v.setSelection(at(t, v, c.from.word, c.from.nth, c.from.offset), at(t, v, c.to.word, c.to.nth, c.to.offset), true)
		if got := v.SelectedText(); got != c.text {
			t.Errorf("%s: text %q, want %q", c.name, got, c.text)
		}
		if got := v.SelectedMarkdown(); got != c.markdown {
			t.Errorf("%s: Markdown %q, want %q", c.name, got, c.markdown)
		}
	}
}

// TestSelectAll checks selecting a whole document, and clearing it.
func TestSelectAll(t *testing.T) {
	const source = "# Title\n\nSome *text*.\n\n- a\n- b\n"
	v := NewView(Parse([]byte(source)))
	if v.HasSelection() || v.SelectedText() != "" {
		t.Error("a new View has a selection")
	}
	v.SelectAll()
	if got := v.SelectedMarkdown(); got != strings.TrimSpace(source) {
		t.Errorf("Markdown %q, want the whole source", got)
	}
	if got, want := v.SelectedText(), "Title\n\nSome text.\n\n- a\n- b"; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
	v.ClearSelection()
	if v.HasSelection() {
		t.Error("cleared, there's still a selection")
	}
}

// TestSelectionWhileStreaming checks that a selection in a finished block
// stays as more arrives, and one in the unfinished end is cleared when
// that end is compiled again.
func TestSelectionWhileStreaming(t *testing.T) {
	doc, w := NewParser().Stream()
	// A complete line starting the second paragraph finishes the first.
	w.Write([]byte("First paragraph.\n\nSecond,\nunfinished"))
	v := NewView(doc)
	v.setSelection(at(t, v, "First", 0, 0), at(t, v, "paragraph", 0, 4), true)
	w.Write([]byte(" and more.\n\nThird.\n"))
	if got := v.SelectedText(); got != "First para" {
		t.Errorf("after more arrived, the selection in a finished block is %q", got)
	}
	v.setSelection(at(t, v, "Third", 0, 0), at(t, v, "Third", 0, 3), true)
	w.Write([]byte("Still third.\n"))
	if v.HasSelection() {
		t.Errorf("a selection in the unfinished end survived its compiling again: %q", v.SelectedText())
	}
	w.Close()
}

// TestSelectionDrawn checks that the selection is drawn behind the text:
// the selected part of a word, and whole words with the gaps between them.
func TestSelectionDrawn(t *testing.T) {
	v := NewView(Parse([]byte("Hello brave new world.\n")))
	layoutView(v, 600, 400, 1, 0)
	selColor := v.ctx.Styles.SelectionColor()
	draw := func() (rects []canvastest.Rect, xs map[string]int) {
		r := &canvastest.Recorder{Area: v.Bounds()}
		v.Draw(r, 0)
		xs = map[string]int{}
		for _, tx := range r.Texts {
			xs[tx.S] = tx.X
		}
		for _, rc := range r.Rects {
			if rc.Color == selColor {
				rects = append(rects, rc)
			}
		}
		return rects, xs
	}
	if rects, _ := draw(); len(rects) != 0 {
		t.Fatalf("nothing selected, but %d selection rectangles drawn", len(rects))
	}

	// Part of a word: within it.
	v.setSelection(at(t, v, "brave", 0, 1), at(t, v, "brave", 0, 4), true)
	rects, xs := draw()
	if len(rects) != 1 || rects[0].X <= xs["brave"] || rects[0].X+rects[0].W >= xs["new"] {
		t.Errorf("selecting \"rav\": rectangles %v, words at %v", rects, xs)
	}

	// Whole words: from the first's start to the last's end, gaps included.
	v.setSelection(at(t, v, "brave", 0, 0), at(t, v, "new", 0, 3), true)
	rects, xs = draw()
	if len(rects) != 2 || rects[0].X != xs["brave"] || rects[0].X+rects[0].W != rects[1].X || rects[1].X != xs["new"] || rects[1].X+rects[1].W >= xs["world"] {
		t.Errorf("selecting \"brave new\": rectangles %v, words at %v", rects, xs)
	}
}
