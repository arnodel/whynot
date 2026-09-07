package whynot

import (
	"fmt"
	"image/color"
	"log"
	"strings"

	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	extast "github.com/yuin/goldmark/v2/extension/ast"
	"github.com/yuin/goldmark/v2/parser"
	"golang.org/x/image/font"
)

// Parse compiles Markdown source into a Block tree ready for layout via
// Block.GetBox.
func Parse(source []byte) Block {
	p := parser.New(parser.WithExtensions(extension.TaskListItemParser, extension.StrikethroughParser, extension.TableParser))
	node := p.Parse(source)
	compiler := MarkdownCompiler{
		source: source,
		paragraphStyle: partStyle{
			TextStyle: TextStyle{Size: 16},
			Margins:   Margins{Top: 10, Bottom: 10},
		},
		listItemStyle: partStyle{
			TextStyle: TextStyle{Size: 16},
			Margins:   Margins{Top: 5, Bottom: 5, Left: 40},
		},
		listStyle: partStyle{
			Margins: Margins{Top: 10, Bottom: 10},
		},
		headingStyles: [6]partStyle{
			{
				TextStyle:   TextStyle{Size: 40, Weight: font.WeightBold, Family: SmallCaps},
				Margins:     Margins{Top: 30, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 36, Weight: font.WeightBold},
				Margins:     Margins{Top: 26, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 32, Weight: font.WeightBold},
				Margins:     Margins{Top: 22, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 28, Weight: font.WeightBold},
				Margins:     Margins{Top: 18, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 24, Weight: font.WeightBold},
				Margins:     Margins{Top: 14, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 20, Weight: font.WeightBold},
				Margins:     Margins{Top: 10, Bottom: 10},
				LevelOffset: 2,
			},
		},
		codeBlockStyle: partStyle{
			TextStyle: TextStyle{Size: 16, Family: Monospace},
			Margins:   Margins{Top: 20, Bottom: 20, Left: 20},
		},
		codeColor:            color.RGBA{0xFF, 0xFF, 0x80, 0xFF},
		thematicBreakMargins: Margins{Top: 20, Bottom: 20},
		thematicBreakColor:   color.RGBA{0x80, 0x80, 0x80, 0xFF},
		linkColor:            color.RGBA{0x66, 0xB2, 0xFF, 0xFF},
		blockquoteMargins:    Margins{Top: 10, Bottom: 10},
		blockquoteBarColor:   color.RGBA{0x80, 0x80, 0x80, 0xFF},
		tableCellStyle:       partStyle{TextStyle: TextStyle{Size: 16}},
		tableMargins:         Margins{Top: 10, Bottom: 10},
		tableFrameColor:      color.RGBA{0x80, 0x80, 0x80, 0xFF},
	}
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
		blocks = append(blocks, c.CompileNode(child, nil))
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
			items = c.AppendInlineNode(items, child, inlineStyle{size: c.paragraphStyle.Size, color: color.White, astNode: astNode})
			child = child.NextSibling()
		}
		return &MarginBlock{Block: &TextBlock{parts: items}, node: astNode}
	case gmast.KindHeading:
		level := node.(*gmast.Heading).Level
		partStyle := c.headingStyles[level-1]
		astNode := parent.AddChild(headingTag(level))
		var items []Inline
		child := node.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, inlineStyle{baseLevel: 2, size: partStyle.Size, color: color.White, astNode: astNode})
			child = child.NextSibling()
		}
		return &MarginBlock{Block: &TextBlock{parts: items}, node: astNode}
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
			items[i] = &InlineText{
				text:  text,
				style: c.codeBlockStyle.TextStyle,
				color: c.codeColor,
				node:  astNode,
			}
		}
		return &MarginBlock{Block: &CodeBlock{lines: items}, node: astNode}
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
			items = append(items, c.CompileNode(child, astNode))
			child = child.NextSibling()
		}
		return &MarginBlock{
			Block: &BlockquoteBlock{inner: wrapBlocks(items), node: astNode},
			node:  astNode,
		}
	case extast.KindTable:
		return c.CompileTable(node, parent)
	}
	panic("Unsupported block")
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
	// hanging off its first line (see ListItemHeadBlock.GetBox). Anything
	// after it - a nested List, or (in a loose list) further paragraphs -
	// stacks below as trailing block content, each already compiled with
	// its own real margins by CompileNode. An item with no leading
	// paragraph leaves items empty, so the marker ends up on a line of
	// its own - GetBox already handles that with no special-casing.
	next := node.FirstChild()
	if next != nil && next.Kind() == gmast.KindParagraph {
		child := next.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, inlineStyle{size: c.listItemStyle.Size, color: color.White, astNode: itemNode})
			child = child.NextSibling()
		}
		next = next.NextSibling()
	}

	head := Block(&ListItemHeadBlock{
		marker: &InlineText{text: markerString, color: color.White, style: c.listItemStyle.TextStyle, node: itemNode},
		parts:  items,
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
		trailingBlocks = append(trailingBlocks, c.CompileNode(next, itemNode))
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
			parts = c.AppendInlineNode(parts, child, inlineStyle{size: c.tableCellStyle.Size, color: color.White, astNode: cellASTNode})
		}
		cells = append(cells, tableCell{
			content:   &TextBlock{parts: parts},
			alignment: tableCellAlignment(tc.Alignment),
		})
	}
	return cells
}

// inlineStyle is the styling state threaded down as AppendInlineNode walks
// an inline subtree - it only ever changes at the node that introduces a
// new value (Emphasis/Strong bump baseLevel, Link overrides color,
// Strikethrough sets strike); every other node passes its received value
// straight through to its children. astNode is the same idea applied to
// the ASTNode tree: the current node's ancestry, updated only by the
// inline constructs that get their own ASTTag (Emphasis, Strong, Link,
// Strikethrough), and threaded straight through everywhere else - not
// found on the produced Block/Inline tree, since that stays flat, but
// carried alongside it during compilation.
type inlineStyle struct {
	baseLevel int
	size      float64
	color     color.Color
	strike    bool
	astNode   *ASTNode
}

func (c *MarkdownCompiler) AppendInlineNode(items []Inline, node gmast.Node, st inlineStyle) []Inline {
	switch node.Kind() {
	case gmast.KindText:
		t := node.(*gmast.Text)
		return appendString(items, t.Value.Value(c.source), getStyle(st.baseLevel, st.size), st.color, st.strike, st.astNode)
	case gmast.KindEmphasis:
		child := node.FirstChild()
		childStyle := st
		childStyle.baseLevel++
		childStyle.astNode = st.astNode.AddChild(TagEmphasis)
		for child != nil {
			items = c.AppendInlineNode(items, child, childStyle)
			child = child.NextSibling()
		}
		return items
	case gmast.KindStrong:
		child := node.FirstChild()
		childStyle := st
		childStyle.baseLevel += 2
		childStyle.astNode = st.astNode.AddChild(TagStrong)
		for child != nil {
			items = c.AppendInlineNode(items, child, childStyle)
			child = child.NextSibling()
		}
		return items
	case gmast.KindCodeSpan:
		style := getStyle(st.baseLevel, st.size)
		style.Family = Monospace
		cs := node.(*gmast.CodeSpan)
		astNode := st.astNode.AddChild(TagCodeSpan)
		return appendString(items, cs.Value.Value(c.source), style, c.codeColor, st.strike, astNode)
	case gmast.KindImage:
		imgNode := node.(*gmast.Image)
		return append(items, &InlineImage{
			src:   imgNode.Destination.Value(c.source),
			title: imgNode.Title.Value(c.source),
			node:  st.astNode,
		})
	case gmast.KindLink:
		child := node.FirstChild()
		childStyle := st
		childStyle.color = c.linkColor
		childStyle.astNode = st.astNode.AddChild(TagLink)
		for child != nil {
			items = c.AppendInlineNode(items, child, childStyle)
			child = child.NextSibling()
		}
		return items
	case gmast.KindAutoLink:
		al := node.(*gmast.AutoLink)
		astNode := st.astNode.AddChild(TagLink)
		return appendString(items, al.Label.Value(c.source), getStyle(st.baseLevel, st.size), c.linkColor, st.strike, astNode)
	case extast.KindStrikethrough:
		child := node.FirstChild()
		childStyle := st
		childStyle.strike = true
		childStyle.astNode = st.astNode.AddChild(TagStrikethrough)
		for child != nil {
			items = c.AppendInlineNode(items, child, childStyle)
			child = child.NextSibling()
		}
		return items
	default:
		log.Panicf("Unsupported node kind %s", node.Kind())
	}
	return nil
}

var levelToStyles = [4]TextStyle{
	{0, font.StyleNormal, font.WeightNormal, Proportional},
	{0, font.StyleItalic, font.WeightNormal, Proportional},
	{0, font.StyleNormal, font.WeightBold, Proportional},
	{0, font.StyleItalic, font.WeightBold, Proportional},
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

func getStyle(level int, size float64) TextStyle {
	textStyle := levelToStyles[level%len(levelToStyles)]
	textStyle.Size = size
	return textStyle
}

func appendString(items []Inline, s string, style TextStyle, color color.Color, strike bool, node *ASTNode) []Inline {
	textParts := strings.Fields(s)
	for _, part := range textParts {
		items = append(items, &InlineText{
			text:   part,
			color:  color,
			style:  style,
			strike: strike,
			node:   node,
		})
	}
	return items
}
