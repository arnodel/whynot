package markdown

import (
	"fmt"
	"log"
	"strings"

	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	extast "github.com/yuin/goldmark/v2/extension/ast"

	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
)

// compileBlock compiles a block node under parent (nil at the top level),
// or returns nil for one that shows nothing.
func (c *compiler) compileBlock(node gmast.Node, parent *ast.Node) engine.Block {
	switch node.Kind() {
	case gmast.KindParagraph:
		return c.compileTextBlock(node, parent.AddChild(ast.TagParagraph), parent == nil)
	case gmast.KindHeading:
		astNode := parent.AddChild(headingTag(node.(*gmast.Heading).Level))
		if attr, ok := node.Attribute("id"); ok {
			astNode.ID = attr.Value(c.source)
		}
		return c.compileTextBlock(node, astNode, parent == nil)
	case gmast.KindList:
		list := node.(*gmast.List)
		astNode := parent.AddChild(ast.TagList)
		var items []engine.Block
		index := 0
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			items = append(items, c.compileListItem(child, list, index, astNode))
			index++
		}
		return &engine.MarginBlock{Block: &engine.StackBlock{Blocks: items}, MarginNode: astNode}
	case gmast.KindCodeBlock:
		astNode := parent.AddChild(ast.TagCodeBlock)
		cb := node.(*gmast.CodeBlock)
		segs := cb.Value.Segments()
		rawLines := make([]string, len(segs))
		for i, seg := range segs {
			// Code keeps its tabs, and most fonts have no glyph for one:
			// expand them so indentation shows as indentation.
			rawLines[i] = strings.ReplaceAll(string(seg.Bytes(c.source)), "\t", codeBlockTabExpansion)
		}

		language, _ := cb.Language(c.source)
		block := c.codeBlock(astNode, language, rawLines, c.pluginsFor(language))
		return &engine.MarginBlock{Block: block, MarginNode: astNode}
	case gmast.KindThematicBreak:
		astNode := parent.AddChild(ast.TagThematicBreak)
		return &engine.MarginBlock{
			Block:      &engine.ThematicBreakBlock{ASTNode: astNode},
			MarginNode: astNode,
		}
	case gmast.KindBlockquote:
		astNode := parent.AddChild(ast.TagBlockquote)
		return &engine.MarginBlock{
			Block:      &engine.BlockquoteBlock{Inner: wrapBlocks(c.compileBlocks(node.FirstChild(), astNode)), ASTNode: astNode},
			MarginNode: astNode,
		}
	case extast.KindTable:
		return c.compileTable(node, parent)
	case gmast.KindLinkReferenceDefinition:
		// A `[foo]: /url` definition: goldmark has already used it to
		// resolve links, and it isn't shown.
		return nil
	case gmast.KindHTMLBlock:
		if node.(*gmast.HTMLBlock).HTMLBlockKind == gmast.HTMLBlockKind2 {
			// A <!-- comment -->: never shown.
			return nil
		}
	}
	return c.compileUnsupportedBlock(node, parent)
}

// compileTextBlock compiles a paragraph or heading, whose ast.Node the
// caller has already created. A top-level one is also recorded in the
// Result: a heading as a Heading, and one whose only content is an
// image as a SoleImages entry.
func (c *compiler) compileTextBlock(node gmast.Node, astNode *ast.Node, topLevel bool) engine.Block {
	items := c.compileInlines(node, astNode)
	block := &engine.MarginBlock{Block: &engine.TextBlock{Parts: items, ASTNode: astNode}, MarginNode: astNode}

	if !topLevel {
		return block
	}
	if astNode.Tag >= ast.TagHeading1 && astNode.Tag <= ast.TagHeading6 {
		c.headings = append(c.headings, Heading{
			ID:    astNode.ID,
			Level: int(astNode.Tag-ast.TagHeading1) + 1,
			Text:  plainText(items),
		})
	}
	if len(items) == 1 {
		if img, ok := items[0].(*engine.InlineImage); ok && img.Image != nil {
			if c.soleImages == nil {
				c.soleImages = make(map[engine.Block]fetch.Source)
			}
			c.soleImages[block] = img.Image
		}
	}
	return block
}

// plainText joins items' words with single spaces; anything but an
// *InlineText (e.g. an image) contributes nothing.
func plainText(items []engine.Inline) string {
	var words []string
	for _, item := range items {
		if it, ok := item.(*engine.InlineText); ok {
			words = append(words, it.Text)
		}
	}
	return strings.Join(words, " ")
}

// compileUnsupportedBlock shows a block construct the compiler has no
// case for as a code block of its raw source (where available), styled
// as unsupported, and logs it: the gap stays visible rather than
// silently dropped.
func (c *compiler) compileUnsupportedBlock(node gmast.Node, parent *ast.Node) engine.Block {
	astNode := parent.AddChild(ast.TagUnsupported)
	log.Printf("whynot: unsupported %s block, showing its source instead", node.Kind())

	var items []engine.Inline
	if html, ok := node.(*gmast.HTMLBlock); ok {
		for _, seg := range html.Value.Segments() {
			items = append(items, &engine.InlineText{Text: string(seg.Bytes(c.source)), ASTNode: astNode})
		}
	}
	if len(items) == 0 {
		items = []engine.Inline{&engine.InlineText{Text: fmt.Sprintf("(unsupported: %s)", node.Kind()), ASTNode: astNode}}
	}
	lines := make([][]engine.Inline, len(items))
	for i, item := range items {
		lines[i] = []engine.Inline{item}
	}
	return &engine.MarginBlock{Block: &engine.CodeBlock{Lines: lines, ASTNode: astNode}, MarginNode: astNode}
}

// headingTag maps a heading level (1-6) to its ast.Tag - safe because
// ast.TagHeading1..TagHeading6 are declared consecutively.
func headingTag(level int) ast.Tag {
	return ast.TagHeading1 + ast.Tag(level-1)
}

// compileListItem compiles the item at index (0-based) in list.
func (c *compiler) compileListItem(node gmast.Node, list *gmast.List, index int, parent *ast.Node) engine.Block {
	itemNode := parent.AddChild(ast.TagListItem)
	var marker engine.Inline
	if status, ok := extension.TaskStatusOf(node); ok {
		marker = &engine.TaskCheckbox{Checked: status == extension.TaskStatusCompleted, ASTNode: itemNode}
	} else {
		marker = &engine.InlineText{Text: listMarker(list, index), ASTNode: itemNode}
	}

	// A leading paragraph is the item's own text, with the marker hanging
	// off its first line. Anything after it (a nested list, further
	// paragraphs) stacks below. Without a leading paragraph, the marker
	// is on a line of its own.
	var parts []engine.Inline
	next := node.FirstChild()
	if next != nil && next.Kind() == gmast.KindParagraph {
		parts = c.compileInlines(next, itemNode)
		next = next.NextSibling()
	}

	head := engine.Block(&engine.ListItemHeadBlock{
		Marker:  marker,
		Parts:   parts,
		ASTNode: itemNode,
	})
	if !list.IsTight {
		// A loose item's text gets paragraph spacing. That also spaces the
		// items apart: a StackBlock's margins are its first child's, so
		// they collapse outward like any sibling gap. The margins come from
		// a paragraph node of its own, while the marker and text keep
		// itemNode for their styling.
		head = &engine.MarginBlock{Block: head, MarginNode: itemNode.AddChild(ast.TagParagraph)}
	}
	blocks := []engine.Block{head}

	if trailingBlocks := c.compileBlocks(next, itemNode); len(trailingBlocks) > 0 {
		blocks = append(blocks, wrapBlocks(trailingBlocks))
	}

	// Always a StackBlock carrying the item's margins (notably Left, the
	// indentation): wrapBlocks would return a lone head unwrapped, and
	// lose them.
	return &engine.MarginBlock{Block: &engine.StackBlock{Blocks: blocks}, MarginNode: itemNode}
}

// listMarker is the marker text for the item at index (0-based) in list:
// the bullet character itself, or the item's number (counting from the
// list's own start number) followed by the list's delimiter.
func listMarker(list *gmast.List, index int) string {
	if list.IsOrdered() {
		return fmt.Sprintf("%d%c", list.Start+index, list.Marker)
	}
	return string(list.Marker)
}

// compileTable compiles a table. It always has a header, but may have no
// body: a table with no data rows has no TableBody child.
func (c *compiler) compileTable(node gmast.Node, parent *ast.Node) engine.Block {
	astNode := parent.AddChild(ast.TagTable)
	headerNode := node.FirstChild()
	header := c.compileTableRow(headerNode, astNode)

	var rows [][]engine.TableCell
	if bodyNode := headerNode.NextSibling(); bodyNode != nil {
		for row := bodyNode.FirstChild(); row != nil; row = row.NextSibling() {
			rows = append(rows, c.compileTableRow(row, astNode))
		}
	}

	return &engine.MarginBlock{
		Block:      &engine.TableBlock{Header: header, Rows: rows, ASTNode: astNode},
		MarginNode: astNode,
	}
}

// compileTableRow compiles the cells of a TableHeader or a TableRow,
// which both have TableCell children.
func (c *compiler) compileTableRow(node gmast.Node, parent *ast.Node) []engine.TableCell {
	var cells []engine.TableCell
	for cellNode := node.FirstChild(); cellNode != nil; cellNode = cellNode.NextSibling() {
		tc := cellNode.(*extast.TableCell)
		cellNode := parent.AddChild(ast.TagTableCell)
		cells = append(cells, engine.TableCell{
			Content:   &engine.TextBlock{Parts: c.compileInlines(tc, cellNode), ASTNode: cellNode},
			Alignment: tableCellAlignment(tc.Alignment),
		})
	}
	return cells
}

// tableCellAlignment converts goldmark's alignment to the engine's.
func tableCellAlignment(a extast.Alignment) engine.CellAlignment {
	switch a {
	case extast.AlignLeft:
		return engine.AlignLeft
	case extast.AlignRight:
		return engine.AlignRight
	case extast.AlignCenter:
		return engine.AlignCenter
	default:
		return engine.AlignNone
	}
}

// wrapBlocks returns a lone block as is, and several in a StackBlock.
func wrapBlocks(blocks []engine.Block) engine.Block {
	if len(blocks) == 1 {
		return blocks[0]
	}
	return &engine.StackBlock{Blocks: blocks}
}
