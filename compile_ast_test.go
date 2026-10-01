package whynot

import (
	"slices"
	"strings"
	"testing"

	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
)

// wantPath asserts got (from some node's Path()) matches want.
func wantPath(t *testing.T, got ast.Path, want ast.Path) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("Path() = %v, want %v", got, want)
	}
}

func TestASTParagraphPath(t *testing.T) {
	doc := Parse([]byte("Hello"))
	stack := doc.root
	wrapper, ok := stack.Blocks[0].(*engine.MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.Blocks[0])
	}
	wantPath(t, wrapper.MarginNode.Path(), ast.Path{ast.TagParagraph})
}

// TestASTHeadingID checks that a heading's ast.Node.ID gets goldmark's
// auto-generated slug - what a link's URL fragment targets - and that
// distinct headings (including ones needing de-duplication) get
// distinct ids.
func TestASTHeadingID(t *testing.T) {
	doc := Parse([]byte("# Hello World\n\n## Hello World\n"))
	stack := doc.root

	first := stack.Blocks[0].(*engine.MarginBlock).MarginNode
	second := stack.Blocks[1].(*engine.MarginBlock).MarginNode

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
		want  ast.Tag
	}{
		{1, ast.TagHeading1}, {2, ast.TagHeading2}, {3, ast.TagHeading3},
		{4, ast.TagHeading4}, {5, ast.TagHeading5}, {6, ast.TagHeading6},
	}
	for _, tc := range cases {
		source := []byte(headingMarkdown(tc.level) + " Title")
		doc := Parse(source)
		stack := doc.root
		wrapper, ok := stack.Blocks[0].(*engine.MarginBlock)
		if !ok {
			t.Fatalf("level %d: block = %T, want *MarginBlock", tc.level, stack.Blocks[0])
		}
		wantPath(t, wrapper.MarginNode.Path(), ast.Path{tc.want})
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
	stack := doc.root
	list := unwrap(stack.Blocks[0]).(*engine.StackBlock)

	outerItemWrapper, ok := list.Blocks[0].(*engine.MarginBlock)
	if !ok {
		t.Fatalf("item = %T, want *MarginBlock", list.Blocks[0])
	}
	wantPath(t, outerItemWrapper.MarginNode.Path(), ast.Path{ast.TagList, ast.TagListItem})

	_, trailing := listItemParts(t, list.Blocks[0])
	nestedList := unwrap(trailing).(*engine.StackBlock)
	innerItemWrapper, ok := nestedList.Blocks[0].(*engine.MarginBlock)
	if !ok {
		t.Fatalf("nested item = %T, want *MarginBlock", nestedList.Blocks[0])
	}
	wantPath(t, innerItemWrapper.MarginNode.Path(), ast.Path{ast.TagList, ast.TagListItem, ast.TagList, ast.TagListItem})
}

func TestASTBlockquotePath(t *testing.T) {
	doc := Parse([]byte("> quoted text"))
	stack := doc.root
	bqWrapper, ok := stack.Blocks[0].(*engine.MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.Blocks[0])
	}
	wantPath(t, bqWrapper.MarginNode.Path(), ast.Path{ast.TagBlockquote})

	bq := bqWrapper.Block.(*engine.BlockquoteBlock)
	if bq.ASTNode != bqWrapper.MarginNode {
		t.Error("BlockquoteBlock.node and its wrapping MarginBlock.node should be the same node")
	}

	paraWrapper, ok := bq.Inner.(*engine.MarginBlock)
	if !ok {
		t.Fatalf("inner = %T, want *MarginBlock", bq.Inner)
	}
	wantPath(t, paraWrapper.MarginNode.Path(), ast.Path{ast.TagBlockquote, ast.TagParagraph})
}

func TestASTTableCellPath(t *testing.T) {
	doc := Parse([]byte("| A | B |\n|---|---|\n| x | y |\n"))
	stack := doc.root
	wrapper, ok := stack.Blocks[0].(*engine.MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.Blocks[0])
	}
	wantPath(t, wrapper.MarginNode.Path(), ast.Path{ast.TagTable})

	table := wrapper.Block.(*engine.TableBlock)
	if len(table.Header) == 0 {
		t.Fatal("no header cells")
	}
	cellText := table.Header[0].Content.Parts[0].(*engine.InlineText)
	wantPath(t, cellText.ASTNode.Path(), ast.Path{ast.TagTable, ast.TagTableCell})
}

func TestASTThematicBreakPath(t *testing.T) {
	doc := Parse([]byte("---"))
	stack := doc.root
	wrapper, ok := stack.Blocks[0].(*engine.MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.Blocks[0])
	}
	wantPath(t, wrapper.MarginNode.Path(), ast.Path{ast.TagThematicBreak})

	rule := wrapper.Block.(*engine.ThematicBreakBlock)
	if rule.ASTNode != wrapper.MarginNode {
		t.Error("ThematicBreakBlock.node and its wrapping MarginBlock.node should be the same node")
	}
}

// TestASTInlineNestingPath checks that Emphasis/Strong/Link/CodeSpan each
// introduce their own ast.Node, nested under the paragraph's, and that
// plain text at the same level doesn't pick up a sibling span's tag.
func TestASTInlineNestingPath(t *testing.T) {
	doc := Parse([]byte("plain **bold *and italic*** [a link](https://example.com) `code`"))
	stack := doc.root
	wrapper := stack.Blocks[0].(*engine.MarginBlock)
	para := wrapper.Block.(*engine.TextBlock)

	texts := make([]*engine.InlineText, 0, len(para.Parts))
	for _, part := range para.Parts {
		if it, ok := part.(*engine.InlineText); ok {
			texts = append(texts, it)
		}
	}

	byWord := map[string]*engine.InlineText{}
	for _, it := range texts {
		byWord[it.Text] = it
	}

	wantPath(t, byWord["plain"].ASTNode.Path(), ast.Path{ast.TagParagraph})
	wantPath(t, byWord["bold"].ASTNode.Path(), ast.Path{ast.TagParagraph, ast.TagStrong})
	wantPath(t, byWord["and"].ASTNode.Path(), ast.Path{ast.TagParagraph, ast.TagStrong, ast.TagEmphasis})
	wantPath(t, byWord["italic"].ASTNode.Path(), ast.Path{ast.TagParagraph, ast.TagStrong, ast.TagEmphasis})
	wantPath(t, byWord["a"].ASTNode.Path(), ast.Path{ast.TagParagraph, ast.TagLink})
	wantPath(t, byWord["link"].ASTNode.Path(), ast.Path{ast.TagParagraph, ast.TagLink})
	wantPath(t, byWord["code"].ASTNode.Path(), ast.Path{ast.TagParagraph, ast.TagCodeSpan})

	if got := byWord["a"].ASTNode.Destination; got != "https://example.com" {
		t.Errorf("link Destination = %q, want %q", got, "https://example.com")
	}
	if byWord["a"].ASTNode != byWord["link"].ASTNode {
		t.Error("both words of the link should share the same ast.Node")
	}
}

func TestASTStrikethroughPath(t *testing.T) {
	doc := Parse([]byte("~~gone~~"))
	stack := doc.root
	wrapper := stack.Blocks[0].(*engine.MarginBlock)
	para := wrapper.Block.(*engine.TextBlock)
	text := para.Parts[0].(*engine.InlineText)
	wantPath(t, text.ASTNode.Path(), ast.Path{ast.TagParagraph, ast.TagStrikethrough})
}

// TestASTLinkReferenceDefinitionIsInvisible checks that a reference-style
// link ([text][ref] plus a [ref]: url "title" definition line) resolves
// normally, and that the definition line itself - once an unhandled
// node kind that crashed compilation entirely - produces no block of
// its own.
func TestASTLinkReferenceDefinitionIsInvisible(t *testing.T) {
	doc := Parse([]byte("[a link][ref]\n\n[ref]: https://example.com \"title\"\n"))
	stack := doc.root
	if len(stack.Blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1 (the reference definition should produce no block)", len(stack.Blocks))
	}

	para := stack.Blocks[0].(*engine.MarginBlock).Block.(*engine.TextBlock)
	link := para.Parts[0].(*engine.InlineText).ASTNode
	if link.Tag != ast.TagLink {
		t.Fatalf("node.Tag = %v, want ast.TagLink", link.Tag)
	}
	if want := "https://example.com"; link.Destination != want {
		t.Errorf("Destination = %q, want %q", link.Destination, want)
	}
}

// TestASTHTMLCommentBlockIsInvisible checks that a block-level <!--
// comment --> - unlike other raw HTML, never visible in any Markdown
// renderer - produces no block of its own, rather than showing as
// ast.TagUnsupported.
func TestASTHTMLCommentBlockIsInvisible(t *testing.T) {
	doc := Parse([]byte("Before.\n\n<!-- ignore -->\n\nAfter.\n"))
	stack := doc.root
	if len(stack.Blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2 (the comment should produce no block)", len(stack.Blocks))
	}
	// "Before"/"." (and "After"/".") are two separate Inline items, not
	// one - the Typographer extension's trigger byte for "." splits the
	// text run even where it substitutes nothing, though InlineLayout.
	// Glued still renders/wraps them as a single unbreakable unit.
	for i, want := range [][]string{{"Before", "."}, {"After", "."}} {
		para := stack.Blocks[i].(*engine.MarginBlock).Block.(*engine.TextBlock)
		if got := textOf(t, para.Parts); !stringsEqual(got, want) {
			t.Errorf("blocks[%d] words = %v, want %v", i, got, want)
		}
	}
}

// TestASTHTMLCommentInlineIsInvisible is
// TestASTHTMLCommentBlockIsInvisible's inline counterpart: a <!--
// comment --> mid-paragraph is dropped rather than shown as
// ast.TagUnsupported text.
func TestASTHTMLCommentInlineIsInvisible(t *testing.T) {
	doc := Parse([]byte("before <!-- ignore --> after\n"))
	stack := doc.root
	para := stack.Blocks[0].(*engine.MarginBlock).Block.(*engine.TextBlock)

	for _, part := range para.Parts {
		if it, ok := part.(*engine.InlineText); ok && it.ASTNode.Tag == ast.TagUnsupported {
			t.Errorf("found a ast.TagUnsupported part (%q); the comment should be invisible", it.Text)
		}
	}
}

// TestASTUnsupportedHTMLBlockShowsSource checks that a raw HTML block -
// a real Markdown construct compile.go has no case for - renders as a
// ast.TagUnsupported code block showing its own source, rather than
// panicking and taking down the whole document.
func TestASTUnsupportedHTMLBlockShowsSource(t *testing.T) {
	doc := Parse([]byte("<div>\n  <p>raw</p>\n</div>\n"))
	stack := doc.root
	if len(stack.Blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(stack.Blocks))
	}

	cb := stack.Blocks[0].(*engine.MarginBlock).Block.(*engine.CodeBlock)
	if cb.ASTNode.Tag != ast.TagUnsupported {
		t.Errorf("Tag = %v, want ast.TagUnsupported", cb.ASTNode.Tag)
	}
	if len(cb.Lines) == 0 {
		t.Fatal("no lines rendered for the unsupported HTML block")
	}
	if first := cb.Lines[0][0].(*engine.InlineText).Text; !strings.Contains(first, "<div>") {
		t.Errorf("first line = %q, want it to contain the raw source", first)
	}
}

// TestASTUnsupportedInlineHTMLShowsSource is
// TestASTUnsupportedHTMLBlockShowsSource's inline counterpart: raw
// inline HTML mid-paragraph renders as ast.TagUnsupported text carrying its
// own source, spliced into the surrounding paragraph rather than
// panicking.
func TestASTUnsupportedInlineHTMLShowsSource(t *testing.T) {
	// <span> and </span> are each their own RawHTML node - CommonMark
	// doesn't pair inline HTML tags - so two separate ast.TagUnsupported
	// runs are expected, one per tag.
	doc := Parse([]byte("before <span>x</span> after\n"))
	stack := doc.root
	para := stack.Blocks[0].(*engine.MarginBlock).Block.(*engine.TextBlock)

	var unsupported []string
	for _, part := range para.Parts {
		if it, ok := part.(*engine.InlineText); ok && it.ASTNode.Tag == ast.TagUnsupported {
			unsupported = append(unsupported, it.Text)
		}
	}
	if !slices.Contains(unsupported, "<span>") {
		t.Errorf("unsupported inline runs = %q, want one of them to be the raw %q", unsupported, "<span>")
	}
}
