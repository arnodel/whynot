package whynot

import (
	"slices"
	"strings"
	"testing"
)

// wantPath asserts got (from some node's Path()) matches want.
func wantPath(t *testing.T, got ASTPath, want ASTPath) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("Path() = %v, want %v", got, want)
	}
}

func TestASTParagraphPath(t *testing.T) {
	doc := Parse([]byte("Hello"))
	stack := doc.(*StackBlock)
	wrapper, ok := stack.blocks[0].(*MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.blocks[0])
	}
	wantPath(t, wrapper.node.Path(), ASTPath{TagParagraph})
}

// TestASTHeadingID checks that a heading's ASTNode.ID gets goldmark's
// auto-generated slug - what a link's URL fragment targets - and that
// distinct headings (including ones needing de-duplication) get
// distinct ids.
func TestASTHeadingID(t *testing.T) {
	doc := Parse([]byte("# Hello World\n\n## Hello World\n"))
	stack := doc.(*StackBlock)

	first := stack.blocks[0].(*MarginBlock).node
	second := stack.blocks[1].(*MarginBlock).node

	if want := "hello-world"; first.ID != want {
		t.Errorf("first heading ID = %q, want %q", first.ID, want)
	}
	if first.ID == second.ID {
		t.Errorf("two headings with the same text got the same ID %q, want distinct ids", first.ID)
	}
}

func TestASTHeadingPath(t *testing.T) {
	cases := []struct {
		level int
		want  ASTTag
	}{
		{1, TagHeading1}, {2, TagHeading2}, {3, TagHeading3},
		{4, TagHeading4}, {5, TagHeading5}, {6, TagHeading6},
	}
	for _, tc := range cases {
		source := []byte(headingMarkdown(tc.level) + " Title")
		doc := Parse(source)
		stack := doc.(*StackBlock)
		wrapper, ok := stack.blocks[0].(*MarginBlock)
		if !ok {
			t.Fatalf("level %d: block = %T, want *MarginBlock", tc.level, stack.blocks[0])
		}
		wantPath(t, wrapper.node.Path(), ASTPath{tc.want})
	}
}

func headingMarkdown(level int) string {
	s := ""
	for range level {
		s += "#"
	}
	return s
}

// TestASTNestedListItemPath checks that ancestry accumulates correctly
// through nested lists: the innermost item's head should show the full
// List/ListItem chain down to it, not just its immediate parent.
func TestASTNestedListItemPath(t *testing.T) {
	doc := Parse([]byte("- one\n  - nested"))
	stack := doc.(*StackBlock)
	list := unwrap(stack.blocks[0]).(*StackBlock)

	outerItemWrapper, ok := list.blocks[0].(*MarginBlock)
	if !ok {
		t.Fatalf("item = %T, want *MarginBlock", list.blocks[0])
	}
	wantPath(t, outerItemWrapper.node.Path(), ASTPath{TagList, TagListItem})

	_, trailing := listItemParts(t, list.blocks[0])
	nestedList := unwrap(trailing).(*StackBlock)
	innerItemWrapper, ok := nestedList.blocks[0].(*MarginBlock)
	if !ok {
		t.Fatalf("nested item = %T, want *MarginBlock", nestedList.blocks[0])
	}
	wantPath(t, innerItemWrapper.node.Path(), ASTPath{TagList, TagListItem, TagList, TagListItem})
}

func TestASTBlockquotePath(t *testing.T) {
	doc := Parse([]byte("> quoted text"))
	stack := doc.(*StackBlock)
	bqWrapper, ok := stack.blocks[0].(*MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.blocks[0])
	}
	wantPath(t, bqWrapper.node.Path(), ASTPath{TagBlockquote})

	bq := bqWrapper.Block.(*BlockquoteBlock)
	if bq.node != bqWrapper.node {
		t.Error("BlockquoteBlock.node and its wrapping MarginBlock.node should be the same node")
	}

	paraWrapper, ok := bq.inner.(*MarginBlock)
	if !ok {
		t.Fatalf("inner = %T, want *MarginBlock", bq.inner)
	}
	wantPath(t, paraWrapper.node.Path(), ASTPath{TagBlockquote, TagParagraph})
}

func TestASTTableCellPath(t *testing.T) {
	doc := Parse([]byte("| A | B |\n|---|---|\n| x | y |\n"))
	stack := doc.(*StackBlock)
	wrapper, ok := stack.blocks[0].(*MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.blocks[0])
	}
	wantPath(t, wrapper.node.Path(), ASTPath{TagTable})

	table := wrapper.Block.(*TableBlock)
	if len(table.header) == 0 {
		t.Fatal("no header cells")
	}
	cellText := table.header[0].content.parts[0].(*InlineText)
	wantPath(t, cellText.node.Path(), ASTPath{TagTable, TagTableCell})
}

func TestASTThematicBreakPath(t *testing.T) {
	doc := Parse([]byte("---"))
	stack := doc.(*StackBlock)
	wrapper, ok := stack.blocks[0].(*MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.blocks[0])
	}
	wantPath(t, wrapper.node.Path(), ASTPath{TagThematicBreak})

	rule := wrapper.Block.(*ThematicBreakBlock)
	if rule.node != wrapper.node {
		t.Error("ThematicBreakBlock.node and its wrapping MarginBlock.node should be the same node")
	}
}

// TestASTInlineNestingPath checks that Emphasis/Strong/Link/CodeSpan each
// introduce their own ASTNode, nested under the paragraph's, and that
// plain text at the same level doesn't pick up a sibling span's tag.
func TestASTInlineNestingPath(t *testing.T) {
	doc := Parse([]byte("plain **bold *and italic*** [a link](https://example.com) `code`"))
	stack := doc.(*StackBlock)
	wrapper := stack.blocks[0].(*MarginBlock)
	para := wrapper.Block.(*TextBlock)

	texts := make([]*InlineText, 0, len(para.parts))
	for _, part := range para.parts {
		if it, ok := part.(*InlineText); ok {
			texts = append(texts, it)
		}
	}

	byWord := map[string]*InlineText{}
	for _, it := range texts {
		byWord[it.text] = it
	}

	wantPath(t, byWord["plain"].node.Path(), ASTPath{TagParagraph})
	wantPath(t, byWord["bold"].node.Path(), ASTPath{TagParagraph, TagStrong})
	wantPath(t, byWord["and"].node.Path(), ASTPath{TagParagraph, TagStrong, TagEmphasis})
	wantPath(t, byWord["italic"].node.Path(), ASTPath{TagParagraph, TagStrong, TagEmphasis})
	wantPath(t, byWord["a"].node.Path(), ASTPath{TagParagraph, TagLink})
	wantPath(t, byWord["link"].node.Path(), ASTPath{TagParagraph, TagLink})
	wantPath(t, byWord["code"].node.Path(), ASTPath{TagParagraph, TagCodeSpan})

	if got := byWord["a"].node.Destination; got != "https://example.com" {
		t.Errorf("link Destination = %q, want %q", got, "https://example.com")
	}
	if byWord["a"].node != byWord["link"].node {
		t.Error("both words of the link should share the same ASTNode")
	}
}

func TestASTStrikethroughPath(t *testing.T) {
	doc := Parse([]byte("~~gone~~"))
	stack := doc.(*StackBlock)
	wrapper := stack.blocks[0].(*MarginBlock)
	para := wrapper.Block.(*TextBlock)
	text := para.parts[0].(*InlineText)
	wantPath(t, text.node.Path(), ASTPath{TagParagraph, TagStrikethrough})
}

// TestASTLinkReferenceDefinitionIsInvisible checks that a reference-style
// link ([text][ref] plus a [ref]: url "title" definition line) resolves
// normally, and that the definition line itself - once an unhandled
// node kind that crashed compilation entirely - produces no block of
// its own.
func TestASTLinkReferenceDefinitionIsInvisible(t *testing.T) {
	doc := Parse([]byte("[a link][ref]\n\n[ref]: https://example.com \"title\"\n"))
	stack := doc.(*StackBlock)
	if len(stack.blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1 (the reference definition should produce no block)", len(stack.blocks))
	}

	para := stack.blocks[0].(*MarginBlock).Block.(*TextBlock)
	link := para.parts[0].(*InlineText).node
	if link.Tag != TagLink {
		t.Fatalf("node.Tag = %v, want TagLink", link.Tag)
	}
	if want := "https://example.com"; link.Destination != want {
		t.Errorf("Destination = %q, want %q", link.Destination, want)
	}
}

// TestASTHTMLCommentBlockIsInvisible checks that a block-level <!--
// comment --> - unlike other raw HTML, never visible in any Markdown
// renderer - produces no block of its own, rather than showing as
// TagUnsupported.
func TestASTHTMLCommentBlockIsInvisible(t *testing.T) {
	doc := Parse([]byte("Before.\n\n<!-- ignore -->\n\nAfter.\n"))
	stack := doc.(*StackBlock)
	if len(stack.blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2 (the comment should produce no block)", len(stack.blocks))
	}
	// "Before"/"." (and "After"/".") are two separate Inline items, not
	// one - the Typographer extension's trigger byte for "." splits the
	// text run even where it substitutes nothing, though InlineLayout.
	// Glued still renders/wraps them as a single unbreakable unit.
	for i, want := range [][]string{{"Before", "."}, {"After", "."}} {
		para := stack.blocks[i].(*MarginBlock).Block.(*TextBlock)
		if got := textOf(t, para.parts); !stringsEqual(got, want) {
			t.Errorf("blocks[%d] words = %v, want %v", i, got, want)
		}
	}
}

// TestASTHTMLCommentInlineIsInvisible is
// TestASTHTMLCommentBlockIsInvisible's inline counterpart: a <!--
// comment --> mid-paragraph is dropped rather than shown as
// TagUnsupported text.
func TestASTHTMLCommentInlineIsInvisible(t *testing.T) {
	doc := Parse([]byte("before <!-- ignore --> after\n"))
	stack := doc.(*StackBlock)
	para := stack.blocks[0].(*MarginBlock).Block.(*TextBlock)

	for _, part := range para.parts {
		if it, ok := part.(*InlineText); ok && it.node.Tag == TagUnsupported {
			t.Errorf("found a TagUnsupported part (%q); the comment should be invisible", it.text)
		}
	}
}

// TestASTUnsupportedHTMLBlockShowsSource checks that a raw HTML block -
// a real Markdown construct compile.go has no case for - renders as a
// TagUnsupported code block showing its own source, rather than
// panicking and taking down the whole document.
func TestASTUnsupportedHTMLBlockShowsSource(t *testing.T) {
	doc := Parse([]byte("<div>\n  <p>raw</p>\n</div>\n"))
	stack := doc.(*StackBlock)
	if len(stack.blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(stack.blocks))
	}

	cb := stack.blocks[0].(*MarginBlock).Block.(*CodeBlock)
	if cb.node.Tag != TagUnsupported {
		t.Errorf("Tag = %v, want TagUnsupported", cb.node.Tag)
	}
	if len(cb.lines) == 0 {
		t.Fatal("no lines rendered for the unsupported HTML block")
	}
	if first := cb.lines[0][0].(*InlineText).text; !strings.Contains(first, "<div>") {
		t.Errorf("first line = %q, want it to contain the raw source", first)
	}
}

// TestASTUnsupportedInlineHTMLShowsSource is
// TestASTUnsupportedHTMLBlockShowsSource's inline counterpart: raw
// inline HTML mid-paragraph renders as TagUnsupported text carrying its
// own source, spliced into the surrounding paragraph rather than
// panicking.
func TestASTUnsupportedInlineHTMLShowsSource(t *testing.T) {
	// <span> and </span> are each their own RawHTML node - CommonMark
	// doesn't pair inline HTML tags - so two separate TagUnsupported
	// runs are expected, one per tag.
	doc := Parse([]byte("before <span>x</span> after\n"))
	stack := doc.(*StackBlock)
	para := stack.blocks[0].(*MarginBlock).Block.(*TextBlock)

	var unsupported []string
	for _, part := range para.parts {
		if it, ok := part.(*InlineText); ok && it.node.Tag == TagUnsupported {
			unsupported = append(unsupported, it.text)
		}
	}
	if !slices.Contains(unsupported, "<span>") {
		t.Errorf("unsupported inline runs = %q, want one of them to be the raw %q", unsupported, "<span>")
	}
}
