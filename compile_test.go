package whynot

import (
	"testing"

	"golang.org/x/image/font"
)

// textOf collects the text of each InlineText in parts, in order. Since
// appendString splits on whitespace, a single source phrase becomes one
// InlineText per word.
func textOf(t *testing.T, parts []Inline) []string {
	t.Helper()
	words := make([]string, len(parts))
	for i, part := range parts {
		text, ok := part.(*InlineText)
		if !ok {
			t.Fatalf("part %d is a %T, not *InlineText", i, part)
		}
		words[i] = text.text
	}
	return words
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseParagraph(t *testing.T) {
	doc := Parse([]byte("Hello there world"))
	stack, ok := doc.(*StackBlock)
	if !ok || len(stack.blocks) != 1 {
		t.Fatalf("Parse result = %#v, want a single-block StackBlock", doc)
	}
	para, ok := stack.blocks[0].(*TextBlock)
	if !ok {
		t.Fatalf("block = %T, want *TextBlock", stack.blocks[0])
	}
	got := textOf(t, para.parts)
	want := []string{"Hello", "there", "world"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

func TestParseHeading(t *testing.T) {
	doc := Parse([]byte("### Level three"))
	stack := doc.(*StackBlock)
	heading, ok := stack.blocks[0].(*TextBlock)
	if !ok {
		t.Fatalf("block = %T, want *TextBlock", stack.blocks[0])
	}
	got := textOf(t, heading.parts)
	want := []string{"Level", "three"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
	// Headings render at baseLevel 2 (bold) regardless of level, so the
	// only per-level difference is the Margins/Size picked in Parse's
	// headingStyles, not anything CompileBlock computes from the AST -
	// nothing further to assert here structurally.
}

func TestParseTightList(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   []string
	}{
		{"dash", "- one\n- two\n- three", []string{"-", "-", "-"}},
		{"paren", "1) one\n2) two", []string{"1)", "2)"}},
		{"dot", "1. one\n2. two", []string{"1.", "2."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			stack := doc.(*StackBlock)
			list, ok := stack.blocks[0].(*StackBlock)
			if !ok || len(list.blocks) != len(tc.want) {
				t.Fatalf("list = %#v, want a %d-item StackBlock", stack.blocks[0], len(tc.want))
			}
			for i, block := range list.blocks {
				item, ok := block.(*ListItemBlock)
				if !ok {
					t.Fatalf("item %d = %T, want *ListItemBlock", i, block)
				}
				marker, ok := item.marker.(*InlineText)
				if !ok || marker.text != tc.want[i] {
					t.Errorf("item %d marker = %#v, want %q", i, item.marker, tc.want[i])
				}
			}
		})
	}
}

// TestParseLooseListPanics checks that a loose list (items separated by a
// blank line) still panics rather than silently rendering, matching v1's
// behavior of only ever handling tight lists.
func TestParseLooseListPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Parse on a loose list did not panic")
		}
	}()
	Parse([]byte("- one\n\n- two"))
}

func TestParseFencedCodeBlock(t *testing.T) {
	doc := Parse([]byte("```\nline one\nline two\n```"))
	stack := doc.(*StackBlock)
	code, ok := stack.blocks[0].(*CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.blocks[0])
	}
	if len(code.lines) != 2 {
		t.Fatalf("got %d lines, want 2: %#v", len(code.lines), code.lines)
	}
	want := []string{"line one\n", "line two\n"}
	for i, line := range code.lines {
		text, ok := line.(*InlineText)
		if !ok || text.text != want[i] {
			t.Errorf("line %d = %#v, want %q", i, line, want[i])
		}
	}
}

func TestParseEmphasisAndStrong(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		wantStyle  font.Style
		wantWeight font.Weight
	}{
		{"emphasis", "*word*", font.StyleItalic, font.WeightNormal},
		{"strong", "**word**", font.StyleNormal, font.WeightBold},
		{"both", "***word***", font.StyleItalic, font.WeightBold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			stack := doc.(*StackBlock)
			para := stack.blocks[0].(*TextBlock)
			if len(para.parts) != 1 {
				t.Fatalf("parts = %#v, want 1 part", para.parts)
			}
			text := para.parts[0].(*InlineText)
			if text.text != "word" {
				t.Errorf("text = %q, want %q", text.text, "word")
			}
			if text.style.Style != tc.wantStyle || text.style.Weight != tc.wantWeight {
				t.Errorf("style = %+v, want Style=%v Weight=%v", text.style, tc.wantStyle, tc.wantWeight)
			}
		})
	}
}

func TestParseCodeSpan(t *testing.T) {
	doc := Parse([]byte("see `code` here"))
	stack := doc.(*StackBlock)
	para := stack.blocks[0].(*TextBlock)
	got := textOf(t, para.parts)
	want := []string{"see", "code", "here"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	code := para.parts[1].(*InlineText)
	if code.style.Family != Monospace {
		t.Errorf("code span family = %v, want Monospace", code.style.Family)
	}
}

func TestParseImageWithTitle(t *testing.T) {
	doc := Parse([]byte(`![alt](cat.jpeg "a lovely cat")`))
	stack := doc.(*StackBlock)
	para := stack.blocks[0].(*TextBlock)
	if len(para.parts) != 1 {
		t.Fatalf("parts = %#v, want 1 part", para.parts)
	}
	img, ok := para.parts[0].(*InlineImage)
	if !ok {
		t.Fatalf("part = %T, want *InlineImage", para.parts[0])
	}
	if img.src != "cat.jpeg" {
		t.Errorf("src = %q, want %q", img.src, "cat.jpeg")
	}
	if img.title != "a lovely cat" {
		t.Errorf("title = %q, want %q", img.title, "a lovely cat")
	}
}

// TestParseResolvesEntitiesAndEscapes checks the v2 migration's behavior
// change noted in the migration plan: text values are resolved (entity
// references and backslash escapes decoded), unlike v1's raw Text().
func TestParseResolvesEntitiesAndEscapes(t *testing.T) {
	doc := Parse([]byte(`Fish \& chips and &amp; and \*literal\*`))
	stack := doc.(*StackBlock)
	para := stack.blocks[0].(*TextBlock)
	got := textOf(t, para.parts)
	want := []string{"Fish", "&", "chips", "and", "&", "and", "*literal*"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}
