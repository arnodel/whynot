package whynot

import (
	"slices"
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
}

func TestASTStrikethroughPath(t *testing.T) {
	doc := Parse([]byte("~~gone~~"))
	stack := doc.(*StackBlock)
	wrapper := stack.blocks[0].(*MarginBlock)
	para := wrapper.Block.(*TextBlock)
	text := para.parts[0].(*InlineText)
	wantPath(t, text.node.Path(), ASTPath{TagParagraph, TagStrikethrough})
}
