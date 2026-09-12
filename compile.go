package whynot

import (
	"fmt"
	"log"
	"strings"

	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	extast "github.com/yuin/goldmark/v2/extension/ast"
	"github.com/yuin/goldmark/v2/parser"
)

// Parse compiles Markdown source into a Block tree ready for layout via
// Block.GetBlockLayout. All appearance (fonts, colors, margins, ...) is resolved
// later, from RenderingContext.StyleSheet against each Block/Inline's own
// ASTNode - Parse itself only builds structure.
func Parse(source []byte) Block {
	p := parser.New(
		parser.WithExtensions(extension.TaskListItemParser, extension.StrikethroughParser, extension.TableParser),
		parser.WithAutoHeadingID(),
	)
	node := p.Parse(source)
	compiler := MarkdownCompiler{source: source}
	return compiler.CompileDocument(node)
}

func (c *MarkdownCompiler) CompileNode(node gmast.Node, parent *ASTNode) Block {
	if node.Kind() == gmast.KindDocument {
		return c.CompileDocument(node)
	}
	if _, ok := node.(gmast.BlockNode); ok {
		return c.CompileBlock(node, parent)
	}
	return nil
}

// CompileDocument compiles the document root. Always the top of the tree -
// gmast.KindDocument is never nested - so unlike CompileBlock/CompileListItem
// it needs no parent *ASTNode to thread through.
func (c *MarkdownCompiler) CompileDocument(node gmast.Node) Block {
	var blocks []Block
	child := node.FirstChild()
	for child != nil {
		if block := c.CompileNode(child, nil); block != nil {
			blocks = append(blocks, block)
		}
		child = child.NextSibling()
	}
	return &StackBlock{blocks: blocks}
}

// codeBlockTabExpansion is what a literal tab in a code block's source is
// replaced with - see the KindCodeBlock case below.
const codeBlockTabExpansion = "    "

func (c *MarkdownCompiler) CompileBlock(node gmast.Node, parent *ASTNode) Block {
	switch node.Kind() {
	case gmast.KindParagraph:
		astNode := parent.AddChild(TagParagraph)
		var items []Inline
		child := node.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, astNode)
			child = child.NextSibling()
		}
		return &MarginBlock{Block: &TextBlock{parts: items, node: astNode}, node: astNode}
	case gmast.KindHeading:
		level := node.(*gmast.Heading).Level
		astNode := parent.AddChild(headingTag(level))
		if attr, ok := node.Attribute("id"); ok {
			astNode.ID = attr.Value(c.source)
		}
		var items []Inline
		child := node.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, astNode)
			child = child.NextSibling()
		}
		return &MarginBlock{Block: &TextBlock{parts: items, node: astNode}, node: astNode}
	case gmast.KindList:
		list := node.(*gmast.List)
		astNode := parent.AddChild(TagList)
		var items []Block
		var index = 1
		child := node.FirstChild()
		for child != nil {
			items = append(items, c.CompileListItem(child, index, list.Marker, list.IsTight, astNode))
			child = child.NextSibling()
			index++
		}
		return &MarginBlock{Block: &StackBlock{blocks: items}, node: astNode}
	case gmast.KindCodeBlock:
		astNode := parent.AddChild(TagCodeBlock)
		cb := node.(*gmast.CodeBlock)
		segs := cb.Value.Segments()
		items := make([]Inline, len(segs))
		for i, seg := range segs {
			// A code block's content is verbatim source, tabs included -
			// unlike indentation elsewhere in the document, this isn't
			// tab-expanded before it reaches us. Most fonts have no glyph
			// for a raw tab, rendering it as a placeholder box instead of
			// whitespace, so expand it here to keep indentation looking
			// like indentation.
			text := strings.ReplaceAll(string(seg.Bytes(c.source)), "\t", codeBlockTabExpansion)
			items[i] = &InlineText{text: text, node: astNode}
		}
		return &MarginBlock{Block: &CodeBlock{lines: items, node: astNode}, node: astNode}
	case gmast.KindThematicBreak:
		astNode := parent.AddChild(TagThematicBreak)
		return &MarginBlock{
			Block: &ThematicBreakBlock{node: astNode},
			node:  astNode,
		}
	case gmast.KindBlockquote:
		astNode := parent.AddChild(TagBlockquote)
		var items []Block
		child := node.FirstChild()
		for child != nil {
			if block := c.CompileNode(child, astNode); block != nil {
				items = append(items, block)
			}
			child = child.NextSibling()
		}
		return &MarginBlock{
			Block: &BlockquoteBlock{inner: wrapBlocks(items), node: astNode},
			node:  astNode,
		}
	case extast.KindTable:
		return c.CompileTable(node, parent)
	case gmast.KindLinkReferenceDefinition:
		// A `[foo]: /url "title"` line - already consumed by goldmark to
		// resolve reference-style links elsewhere in the document (see
		// KindLink), and, like in any other Markdown renderer, invisible
		// in its own right.
		return nil
	case gmast.KindHTMLBlock:
		if node.(*gmast.HTMLBlock).HTMLBlockKind == gmast.HTMLBlockKind2 {
			// A <!-- comment -->, invisible in any Markdown renderer -
			// not "unsupported", never meant to be shown at all.
			return nil
		}
	}
	return c.compileUnsupportedBlock(node, parent)
}

// compileUnsupportedBlock handles any block-level Markdown construct
// whynot doesn't have a case for above. Rather than taking down the
// whole document, it logs a warning and renders the construct as a
// code block in StyleSheet.UnsupportedColor, showing its raw source
// where possible so the gap is visible rather than silently dropped.
func (c *MarkdownCompiler) compileUnsupportedBlock(node gmast.Node, parent *ASTNode) Block {
	astNode := parent.AddChild(TagUnsupported)
	log.Printf("whynot: unsupported %s block, showing its source instead", node.Kind())

	var items []Inline
	if html, ok := node.(*gmast.HTMLBlock); ok {
		for _, seg := range html.Value.Segments() {
			items = append(items, &InlineText{text: string(seg.Bytes(c.source)), node: astNode})
		}
	}
	if len(items) == 0 {
		items = []Inline{&InlineText{text: fmt.Sprintf("(unsupported: %s)", node.Kind()), node: astNode}}
	}
	return &MarginBlock{Block: &CodeBlock{lines: items, node: astNode}, node: astNode}
}

// headingTag maps a heading level (1-6) to its ASTTag - safe because
// TagHeading1..TagHeading6 are declared consecutively.
func headingTag(level int) ASTTag {
	return TagHeading1 + ASTTag(level-1)
}

func (c *MarkdownCompiler) CompileListItem(node gmast.Node, index int, marker byte, tight bool, parent *ASTNode) Block {
	itemNode := parent.AddChild(TagListItem)
	var items []Inline
	var markerString string
	switch marker {
	case '-', '+', '*':
		markerString = string(marker)
	case ')':
		markerString = fmt.Sprintf("%d)", index)
	case '.':
		markerString = fmt.Sprintf("%d.", index)
	default:
		panic("Unsupported marker")
	}
	if status, ok := extension.TaskStatusOf(node); ok {
		// Go's built-in font has no ballot-box/checkmark glyphs (☐ ☑ ✓
		// etc. all miss) - □/■ are covered and the same width, so the
		// list stays aligned either way.
		if status == extension.TaskStatusCompleted {
			markerString = "■"
		} else {
			markerString = "□"
		}
	}

	// A leading Paragraph is the item's own text, flowed with the marker
	// hanging off its first line (see ListItemHeadBlock.GetBlockLayout). Anything
	// after it - a nested List, or (in a loose list) further paragraphs -
	// stacks below as trailing block content, each already compiled with
	// its own real margins by CompileNode. An item with no leading
	// paragraph leaves items empty, so the marker ends up on a line of
	// its own - GetBlockLayout already handles that with no special-casing.
	next := node.FirstChild()
	if next != nil && next.Kind() == gmast.KindParagraph {
		child := next.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, itemNode)
			child = child.NextSibling()
		}
		next = next.NextSibling()
	}

	head := Block(&ListItemHeadBlock{
		marker: &InlineText{text: markerString, node: itemNode},
		parts:  items,
		node:   itemNode,
	})
	if !tight {
		// Loose items get real paragraph spacing on their own leading text
		// too, not a tight head's zero margins. This also grows the gap
		// between items with no separate constant: StackBlock.Margins()
		// reports its first child's margin, so it collapses outward
		// through the item's own MarginBlock and the list's own
		// StackBlock, like any sibling gap.
		//
		// A dedicated TagParagraph child, not itemNode itself: the head's
		// own margins should resolve as a paragraph's (matching what this
		// looked like before StyleSheet), distinct from itemNode's tag,
		// which the marker/parts' own inline styling still uses.
		head = &MarginBlock{Block: head, node: itemNode.AddChild(TagParagraph)}
	}
	blocks := []Block{head}

	var trailingBlocks []Block
	for ; next != nil; next = next.NextSibling() {
		if block := c.CompileNode(next, itemNode); block != nil {
			trailingBlocks = append(trailingBlocks, block)
		}
	}
	if len(trailingBlocks) > 0 {
		blocks = append(blocks, wrapBlocks(trailingBlocks))
	}

	// The whole item - head plus any trailing content - is a StackBlock
	// carrying the item's real margins (crucially Left, for indentation).
	// Not wrapBlocks: that returns a single block unwrapped when there's
	// only one, which would lose these margins for the common
	// no-trailing-content tight case (ListItemHeadBlock's own Margins() is
	// zero then).
	return &MarginBlock{Block: &StackBlock{blocks: blocks}, node: itemNode}
}

// CompileTable compiles a Table node. The header is mandatory (GFM
// requires it); the body is not - a table can legitimately have zero
// data rows, in which case Table has no TableBody child at all.
func (c *MarkdownCompiler) CompileTable(node gmast.Node, parent *ASTNode) Block {
	astNode := parent.AddChild(TagTable)
	headerNode := node.FirstChild()
	header := c.CompileTableRow(headerNode, astNode)

	var rows [][]tableCell
	if bodyNode := headerNode.NextSibling(); bodyNode != nil {
		for row := bodyNode.FirstChild(); row != nil; row = row.NextSibling() {
			rows = append(rows, c.CompileTableRow(row, astNode))
		}
	}

	return &MarginBlock{
		Block: &TableBlock{header: header, rows: rows, node: astNode},
		node:  astNode,
	}
}

// CompileTableRow compiles the cells of a TableHeader or a TableRow -
// both have TableCell children directly, no intermediate node, so one
// method handles both despite the different AST kinds.
func (c *MarkdownCompiler) CompileTableRow(node gmast.Node, parent *ASTNode) []tableCell {
	var cells []tableCell
	for cellNode := node.FirstChild(); cellNode != nil; cellNode = cellNode.NextSibling() {
		tc := cellNode.(*extast.TableCell)
		cellASTNode := parent.AddChild(TagTableCell)
		var parts []Inline
		for child := tc.FirstChild(); child != nil; child = child.NextSibling() {
			parts = c.AppendInlineNode(parts, child, cellASTNode)
		}
		cells = append(cells, tableCell{
			content:   &TextBlock{parts: parts, node: cellASTNode},
			alignment: tableCellAlignment(tc.Alignment),
		})
	}
	return cells
}

// AppendInlineNode walks an inline subtree, appending each leaf as an
// Inline. astNode is node's parent in the ASTNode tree - updated only by
// the constructs that get their own ASTTag (Emphasis, Strong, Link,
// CodeSpan, Strikethrough) via AddChild, and threaded straight through
// everywhere else. Appearance (font, color, strike) is never resolved
// here - each produced Inline just carries the ASTNode it was created
// under, resolved later by RenderingContext against a StyleSheet.
func (c *MarkdownCompiler) AppendInlineNode(items []Inline, node gmast.Node, astNode *ASTNode) []Inline {
	switch node.Kind() {
	case gmast.KindText:
		t := node.(*gmast.Text)
		return appendString(items, t.Value.Value(c.source), astNode)
	case gmast.KindEmphasis:
		child := node.FirstChild()
		childNode := astNode.AddChild(TagEmphasis)
		for child != nil {
			items = c.AppendInlineNode(items, child, childNode)
			child = child.NextSibling()
		}
		return items
	case gmast.KindStrong:
		child := node.FirstChild()
		childNode := astNode.AddChild(TagStrong)
		for child != nil {
			items = c.AppendInlineNode(items, child, childNode)
			child = child.NextSibling()
		}
		return items
	case gmast.KindCodeSpan:
		cs := node.(*gmast.CodeSpan)
		childNode := astNode.AddChild(TagCodeSpan)
		return appendString(items, cs.Value.Value(c.source), childNode)
	case gmast.KindImage:
		imgNode := node.(*gmast.Image)
		return append(items, &InlineImage{
			src:   imgNode.Destination.Value(c.source),
			title: imgNode.Title.Value(c.source),
			node:  astNode,
		})
	case gmast.KindLink:
		link := node.(*gmast.Link)
		child := node.FirstChild()
		childNode := astNode.AddChild(TagLink)
		childNode.Destination = link.Destination.Value(c.source)
		for child != nil {
			items = c.AppendInlineNode(items, child, childNode)
			child = child.NextSibling()
		}
		return items
	case gmast.KindAutoLink:
		al := node.(*gmast.AutoLink)
		childNode := astNode.AddChild(TagLink)
		childNode.Destination = al.Destination.Value(c.source)
		return appendString(items, al.Label.Value(c.source), childNode)
	case extast.KindStrikethrough:
		child := node.FirstChild()
		childNode := astNode.AddChild(TagStrikethrough)
		for child != nil {
			items = c.AppendInlineNode(items, child, childNode)
			child = child.NextSibling()
		}
		return items
	default:
		return c.appendUnsupportedInline(items, node, astNode)
	}
}

// appendUnsupportedInline is AppendInlineNode's fallback for any inline
// Markdown construct whynot doesn't have a case for - the inline
// counterpart to compileUnsupportedBlock. Inline content can't hold a
// block-level box, so the raw source (where available) is spliced into
// the surrounding paragraph as ordinary words, styled in
// StyleSheet.UnsupportedColor via TagUnsupported.
func (c *MarkdownCompiler) appendUnsupportedInline(items []Inline, node gmast.Node, astNode *ASTNode) []Inline {
	text := fmt.Sprintf("(unsupported: %s)", node.Kind())
	if raw, ok := node.(*gmast.RawHTML); ok {
		text = raw.Value.Value(c.source)
		if strings.HasPrefix(text, "<!--") {
			// A <!-- comment -->, invisible in any Markdown renderer -
			// not "unsupported", never meant to be shown at all. Unlike
			// HTMLBlockKind2, goldmark gives inline RawHTML no kind of
			// its own to check instead.
			return items
		}
	}
	log.Printf("whynot: unsupported %s inline content, showing its source instead", node.Kind())
	return appendString(items, text, astNode.AddChild(TagUnsupported))
}

// tableCellAlignment translates goldmark's own alignment enum to
// whynot's - see cellAlignment's doc comment for why they're kept
// distinct.
func tableCellAlignment(a extast.Alignment) cellAlignment {
	switch a {
	case extast.AlignLeft:
		return alignLeft
	case extast.AlignRight:
		return alignRight
	case extast.AlignCenter:
		return alignCenter
	default:
		return alignNone
	}
}

// wrapBlocks returns blocks[0] directly if there's exactly one, or a
// StackBlock of all of them otherwise - for a Block field that holds "one
// or more" blocks without unconditionally wrapping a single child.
func wrapBlocks(blocks []Block) Block {
	if len(blocks) == 1 {
		return blocks[0]
	}
	return &StackBlock{blocks: blocks}
}

func appendString(items []Inline, s string, node *ASTNode) []Inline {
	textParts := strings.Fields(s)
	for _, part := range textParts {
		items = append(items, &InlineText{text: part, node: node})
	}
	return items
}
