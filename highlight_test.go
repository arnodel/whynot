package whynot

import "testing"

// fakeHighlighter is a Highlighter test double whose Highlight method is
// swapped in per test case, so each test can control exactly what spans
// come back without needing a real tokenizer.
type fakeHighlighter struct {
	highlight func(language, code string) []HighlightSpan
}

func (h fakeHighlighter) Highlight(language, code string) []HighlightSpan {
	return h.highlight(language, code)
}

func lineTexts(t *testing.T, line []Inline) []string {
	t.Helper()
	texts := make([]string, len(line))
	for i, part := range line {
		text, ok := part.(*InlineText)
		if !ok {
			t.Fatalf("part %d = %T, want *InlineText", i, part)
		}
		texts[i] = text.text
	}
	return texts
}

func TestHighlightLinesEmpty(t *testing.T) {
	h := fakeHighlighter{highlight: func(language, code string) []HighlightSpan {
		t.Fatalf("Highlight called with no lines")
		return nil
	}}
	lines := highlightLines(h, nil, "go", nil)
	if len(lines) != 0 {
		t.Fatalf("lines = %#v, want empty", lines)
	}
}

func TestHighlightLinesMultipleSpansPerLine(t *testing.T) {
	h := fakeHighlighter{highlight: func(language, code string) []HighlightSpan {
		if code != "func f() {" {
			t.Fatalf("code = %q", code)
		}
		return []HighlightSpan{
			{Text: "func", Class: TokenKeyword},
			{Text: " f() {", Class: TokenPlain},
		}
	}}
	blockNode := (*ASTNode)(nil).AddChild(TagCodeBlock)
	lines := highlightLines(h, blockNode, "go", []string{"func f() {"})
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1", len(lines))
	}
	if got := lineTexts(t, lines[0]); len(got) != 2 || got[0] != "func" || got[1] != " f() {" {
		t.Fatalf("lines[0] texts = %#v", got)
	}
	keywordNode := lines[0][0].(*InlineText).node
	if keywordNode == blockNode {
		t.Fatalf("keyword span reused the block's own node, want a TagCodeKeyword child")
	}
	if keywordNode.Tag != TagCodeKeyword {
		t.Fatalf("keyword node tag = %v, want TagCodeKeyword", keywordNode.Tag)
	}
	plainNode := lines[0][1].(*InlineText).node
	if plainNode != blockNode {
		t.Fatalf("plain span node = %v, want the block's own node (no child allocated)", plainNode)
	}
}

func TestHighlightLinesSpanCrossingMultipleLines(t *testing.T) {
	h := fakeHighlighter{highlight: func(language, code string) []HighlightSpan {
		return []HighlightSpan{
			{Text: "/* a\nb */", Class: TokenComment},
			{Text: "\nrest", Class: TokenPlain},
		}
	}}
	blockNode := (*ASTNode)(nil).AddChild(TagCodeBlock)
	lines := highlightLines(h, blockNode, "", []string{"/* a", "b */", "rest"})
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	if got := lineTexts(t, lines[0]); len(got) != 1 || got[0] != "/* a" {
		t.Fatalf("lines[0] texts = %#v", got)
	}
	if got := lineTexts(t, lines[1]); len(got) != 1 || got[0] != "b */" {
		t.Fatalf("lines[1] texts = %#v", got)
	}
	if got := lineTexts(t, lines[2]); len(got) != 1 || got[0] != "rest" {
		t.Fatalf("lines[2] texts = %#v", got)
	}
	for i := 0; i < 2; i++ {
		if lines[i][0].(*InlineText).node.Tag != TagCodeComment {
			t.Fatalf("lines[%d] node tag = %v, want TagCodeComment", i, lines[i][0].(*InlineText).node.Tag)
		}
	}
	if lines[2][0].(*InlineText).node != blockNode {
		t.Fatalf("lines[2] node = %v, want the block's own node", lines[2][0].(*InlineText).node)
	}
}

// TestHighlightLinesRawLinesAlreadyCarryNewlines is a regression test:
// rawLines (as compile.go's KindCodeBlock case actually builds them) each
// already end in their own "\n" - highlightLines must concatenate them
// verbatim, not insert an extra "\n" between them, or a Highlighter that
// echoes its input back (like a real tokenizer effectively does for a
// plain span) would see a spurious blank line between every pair of
// lines, which - due to how per-line splitting and the len(rawLines)
// capacity guard interact - previously produced an empty-parts line
// rather than tripping the mismatched-line-count fallback.
func TestHighlightLinesRawLinesAlreadyCarryNewlines(t *testing.T) {
	h := fakeHighlighter{highlight: func(language, code string) []HighlightSpan {
		return []HighlightSpan{{Text: code, Class: TokenPlain}}
	}}
	blockNode := (*ASTNode)(nil).AddChild(TagCodeBlock)
	rawLines := []string{"line one\n", "line two\n", "line three\n"}
	lines := highlightLines(h, blockNode, "", rawLines)
	if len(lines) != len(rawLines) {
		t.Fatalf("len(lines) = %d, want %d: %#v", len(lines), len(rawLines), lines)
	}
	for i, want := range []string{"line one", "line two", "line three"} {
		if len(lines[i]) != 1 {
			t.Fatalf("lines[%d] = %#v, want exactly 1 part", i, lines[i])
		}
		if got := lines[i][0].(*InlineText).text; got != want {
			t.Errorf("lines[%d] text = %q, want %q", i, got, want)
		}
	}
}

// TestHighlightLinesBlankLineGetsAPart is a regression test: a blank
// source line inside a highlighted code block (e.g. separating two
// paragraphs of a comment, or just stylistic spacing) contributes no
// span text at all, which previously left that line's parts slice empty
// - LineBox indexes parts[0] unconditionally, so a real Highlighter
// (chroma) run against a document with a blank line in a fenced block
// crashed rendering. Every line must end up with at least one part.
func TestHighlightLinesBlankLineGetsAPart(t *testing.T) {
	h := fakeHighlighter{highlight: func(language, code string) []HighlightSpan {
		return []HighlightSpan{{Text: code, Class: TokenPlain}}
	}}
	blockNode := (*ASTNode)(nil).AddChild(TagCodeBlock)
	rawLines := []string{"func f() {\n", "\n", "}\n"}
	lines := highlightLines(h, blockNode, "", rawLines)
	if len(lines) != len(rawLines) {
		t.Fatalf("len(lines) = %d, want %d: %#v", len(lines), len(rawLines), lines)
	}
	if len(lines[1]) != 1 {
		t.Fatalf("blank line parts = %#v, want exactly 1 part", lines[1])
	}
	if got := lines[1][0].(*InlineText).text; got != "" {
		t.Errorf("blank line text = %q, want empty", got)
	}
}

func TestHighlightLinesMismatchedLineCountFallsBack(t *testing.T) {
	h := fakeHighlighter{highlight: func(language, code string) []HighlightSpan {
		// Only reproduces one line's worth of newlines, though rawLines
		// below has two - a misbehaving Highlighter.
		return []HighlightSpan{{Text: "just one line", Class: TokenPlain}}
	}}
	blockNode := (*ASTNode)(nil).AddChild(TagCodeBlock)
	lines := highlightLines(h, blockNode, "", []string{"line one", "line two"})
	if lines != nil {
		t.Fatalf("lines = %#v, want nil (fallback signal)", lines)
	}
}
