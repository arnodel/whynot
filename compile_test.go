package whynot

import (
	"image"
	"image/color"
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

func TestParseTaskList(t *testing.T) {
	doc := Parse([]byte("- [ ] todo item\n- [x] done item\n- plain item"))
	stack := doc.(*StackBlock)
	list, ok := stack.blocks[0].(*StackBlock)
	if !ok || len(list.blocks) != 3 {
		t.Fatalf("list = %#v, want a 3-item StackBlock", stack.blocks[0])
	}

	wantMarker := []string{"□", "■", "-"}
	wantWords := [][]string{{"todo", "item"}, {"done", "item"}, {"plain", "item"}}
	for i, block := range list.blocks {
		item, ok := block.(*ListItemBlock)
		if !ok {
			t.Fatalf("item %d = %T, want *ListItemBlock", i, block)
		}
		marker, ok := item.marker.(*InlineText)
		if !ok || marker.text != wantMarker[i] {
			t.Errorf("item %d marker = %#v, want %q", i, item.marker, wantMarker[i])
		}
		// The checkbox syntax must be fully consumed by the task list
		// parser - it shouldn't leak into the item's own text as a
		// leftover "[ ]"/"[x]" word.
		got := textOf(t, item.parts)
		if !stringsEqual(got, wantWords[i]) {
			t.Errorf("item %d words = %v, want %v", i, got, wantWords[i])
		}
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

func TestParseIndentedCodeBlock(t *testing.T) {
	doc := Parse([]byte("    line one\n    line two"))
	stack := doc.(*StackBlock)
	code, ok := stack.blocks[0].(*CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.blocks[0])
	}
	want := []string{"line one\n", "line two\n"}
	if len(code.lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %#v", len(code.lines), len(want), code.lines)
	}
	for i, line := range code.lines {
		text, ok := line.(*InlineText)
		if !ok || text.text != want[i] {
			t.Errorf("line %d = %#v, want %q", i, line, want[i])
		}
	}
}

func TestParseThematicBreak(t *testing.T) {
	doc := Parse([]byte("---"))
	stack := doc.(*StackBlock)
	rule, ok := stack.blocks[0].(*ThematicBreakBlock)
	if !ok {
		t.Fatalf("block = %T, want *ThematicBreakBlock", stack.blocks[0])
	}
	if rule.margins != (Margins{Top: 20, Bottom: 20}) {
		t.Errorf("margins = %+v, want {Top: 20, Bottom: 20}", rule.margins)
	}

	ctx := RenderingContext{Scale: 1}
	box := rule.GetBox(ctx, 100)
	ruleBox, ok := box.(*RuleBox)
	if !ok {
		t.Fatalf("GetBox = %T, want *RuleBox", box)
	}
	want := image.Rect(0, 0, 100, thematicBreakThickness)
	if got := ruleBox.Bounds(); got != want {
		t.Errorf("Bounds() = %v, want %v", got, want)
	}
}

// TestParseBlockquote checks that a single-block blockquote's inner is
// that block directly, not wrapped in a StackBlock (wrapBlocks only wraps
// when there's more than one).
func TestParseBlockquote(t *testing.T) {
	doc := Parse([]byte("> quoted text"))
	stack := doc.(*StackBlock)
	bq, ok := stack.blocks[0].(*BlockquoteBlock)
	if !ok {
		t.Fatalf("block = %T, want *BlockquoteBlock", stack.blocks[0])
	}
	para, ok := bq.inner.(*TextBlock)
	if !ok {
		t.Fatalf("inner = %T, want *TextBlock", bq.inner)
	}
	got := textOf(t, para.parts)
	want := []string{"quoted", "text"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

// TestParseBlockquoteMultipleBlocks checks that a blockquote spanning more
// than one block wraps them in a StackBlock, unlike the single-block case.
func TestParseBlockquoteMultipleBlocks(t *testing.T) {
	doc := Parse([]byte("> first\n>\n> second"))
	stack := doc.(*StackBlock)
	bq := stack.blocks[0].(*BlockquoteBlock)
	inner, ok := bq.inner.(*StackBlock)
	if !ok || len(inner.blocks) != 2 {
		t.Fatalf("inner = %#v, want a 2-block StackBlock", bq.inner)
	}
}

// TestParseNestedBlockquote checks that a blockquote inside a blockquote
// (`> > ...`) compiles recursively - each level gets its own bar when
// drawn, with no special-casing needed since BlockquoteBlock positions
// itself rather than relying on its parent.
func TestParseNestedBlockquote(t *testing.T) {
	doc := Parse([]byte("> > nested quote"))
	stack := doc.(*StackBlock)
	outer := stack.blocks[0].(*BlockquoteBlock)
	inner, ok := outer.inner.(*BlockquoteBlock)
	if !ok {
		t.Fatalf("inner = %T, want *BlockquoteBlock", outer.inner)
	}
	para, ok := inner.inner.(*TextBlock)
	if !ok {
		t.Fatalf("innermost = %T, want *TextBlock", inner.inner)
	}
	got := textOf(t, para.parts)
	want := []string{"nested", "quote"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

func TestBlockquoteBoxIndent(t *testing.T) {
	bq := &BlockquoteBlock{
		inner:    &fixedHeightBlock{height: 10},
		barColor: color.White,
	}
	ctx := RenderingContext{Scale: 1}
	box := bq.GetBox(ctx, 100)
	bqBox, ok := box.(*BlockquoteBox)
	if !ok {
		t.Fatalf("GetBox = %T, want *BlockquoteBox", box)
	}
	if bqBox.indent != blockquoteIndent {
		t.Errorf("indent = %d, want %d", bqBox.indent, blockquoteIndent)
	}
	wantInnerWidth := 100 - blockquoteIndent
	if got := bqBox.inner.Bounds().Dx(); got != wantInnerWidth {
		t.Errorf("inner width = %d, want %d", got, wantInnerWidth)
	}
	want := image.Rect(0, 0, 100, 10)
	if got := bqBox.Bounds(); got != want {
		t.Errorf("Bounds() = %v, want %v", got, want)
	}
}

func TestParseLink(t *testing.T) {
	doc := Parse([]byte(`[click *here*](https://example.com "a title")`))
	stack := doc.(*StackBlock)
	para := stack.blocks[0].(*TextBlock)
	got := textOf(t, para.parts)
	want := []string{"click", "here"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	for i, part := range para.parts {
		text := part.(*InlineText)
		if text.color == color.White {
			t.Errorf("part %d color = white, want link color", i)
		}
	}
	// Nested emphasis inside the link text should still apply on top of
	// the link's color.
	if para.parts[1].(*InlineText).style.Style != font.StyleItalic {
		t.Errorf("style of %q = %+v, want italic", "here", para.parts[1].(*InlineText).style)
	}
}

func TestParseAutoLink(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"url", "<https://example.com>", "https://example.com"},
		{"email", "<user@example.com>", "user@example.com"},
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
			if text.text != tc.want {
				t.Errorf("text = %q, want %q", text.text, tc.want)
			}
			if text.color == color.White {
				t.Errorf("color = white, want link color")
			}
		})
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
