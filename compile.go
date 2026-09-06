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

func (c *MarkdownCompiler) CompileNode(node gmast.Node) Block {
	if node.Kind() == gmast.KindDocument {
		return c.CompileDocument(node)
	}
	if _, ok := node.(gmast.BlockNode); ok {
		return c.CompileBlock(node)
	}
	return nil
}

func (c *MarkdownCompiler) CompileDocument(node gmast.Node) Block {
	var blocks []Block
	child := node.FirstChild()
	for child != nil {
		blocks = append(blocks, c.CompileNode(child))
		child = child.NextSibling()
	}
	return &StackBlock{blocks: blocks}
}

func (c *MarkdownCompiler) CompileBlock(node gmast.Node) Block {
	switch node.Kind() {
	case gmast.KindParagraph:
		var items []Inline
		child := node.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, inlineStyle{size: c.paragraphStyle.Size, color: color.White})
			child = child.NextSibling()
		}
		return &MarginBlock{Block: &TextBlock{parts: items}, margins: c.paragraphStyle.Margins}
	case gmast.KindHeading:
		var items []Inline
		partStyle := c.headingStyles[node.(*gmast.Heading).Level-1]
		child := node.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, inlineStyle{baseLevel: 2, size: partStyle.Size, color: color.White})
			child = child.NextSibling()
		}
		return &MarginBlock{Block: &TextBlock{parts: items}, margins: partStyle.Margins}
	case gmast.KindList:
		list := node.(*gmast.List)
		var items []Block
		var index = 1
		child := node.FirstChild()
		for child != nil {
			items = append(items, c.CompileListItem(child, index, list.Marker, list.IsTight))
			child = child.NextSibling()
			index++
		}
		return &MarginBlock{Block: &StackBlock{blocks: items}, margins: c.listStyle.Margins}
	case gmast.KindCodeBlock:
		cb := node.(*gmast.CodeBlock)
		segs := cb.Value.Segments()
		items := make([]Inline, len(segs))
		for i, seg := range segs {
			items[i] = &InlineText{
				text:  string(seg.Bytes(c.source)),
				style: c.codeBlockStyle.TextStyle,
				color: c.codeColor,
			}
		}
		return &MarginBlock{Block: &CodeBlock{lines: items}, margins: c.codeBlockStyle.Margins}
	case gmast.KindThematicBreak:
		return &MarginBlock{
			Block:   &ThematicBreakBlock{color: c.thematicBreakColor},
			margins: c.thematicBreakMargins,
		}
	case gmast.KindBlockquote:
		var items []Block
		child := node.FirstChild()
		for child != nil {
			items = append(items, c.CompileNode(child))
			child = child.NextSibling()
		}
		return &MarginBlock{
			Block:   &BlockquoteBlock{inner: wrapBlocks(items), barColor: c.blockquoteBarColor},
			margins: c.blockquoteMargins,
		}
	case extast.KindTable:
		return c.CompileTable(node)
	}
	panic("Unsupported block")
}

func (c *MarkdownCompiler) CompileListItem(node gmast.Node, index int, marker byte, tight bool) Block {
	if !tight {
		log.Panicf("Unsupported: loose list item")
	}

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
	// after it - most commonly a nested List - stacks below as trailing
	// block content. An item with no leading paragraph (e.g. one that
	// opens directly with a nested list) leaves items empty, so the
	// marker ends up on a line of its own - GetBox already does the
	// right thing for that with no special-casing needed.
	next := node.FirstChild()
	if next != nil && next.Kind() == gmast.KindParagraph {
		child := next.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, inlineStyle{size: c.listItemStyle.Size, color: color.White})
			child = child.NextSibling()
		}
		next = next.NextSibling()
	}

	blocks := []Block{&ListItemHeadBlock{
		marker: &InlineText{text: markerString, color: color.White, style: c.listItemStyle.TextStyle},
		parts:  items,
	}}
	var trailingBlocks []Block
	for ; next != nil; next = next.NextSibling() {
		trailingBlocks = append(trailingBlocks, c.CompileNode(next))
	}
	if len(trailingBlocks) > 0 {
		blocks = append(blocks, wrapBlocks(trailingBlocks))
	}

	// The whole item - head plus any trailing content - is a StackBlock
	// carrying the item's real margins (crucially Left, for indentation).
	// Not wrapBlocks: that returns a single block unwrapped when there's
	// only one, which would lose these margins for the common
	// no-trailing-content case (ListItemHeadBlock's own Margins() is
	// always zero).
	return &MarginBlock{Block: &StackBlock{blocks: blocks}, margins: c.listItemStyle.Margins}
}

// CompileTable compiles a Table node. The header is mandatory (GFM
// requires it); the body is not - a table can legitimately have zero
// data rows, in which case Table has no TableBody child at all.
func (c *MarkdownCompiler) CompileTable(node gmast.Node) Block {
	headerNode := node.FirstChild()
	header := c.CompileTableRow(headerNode)

	var rows [][]tableCell
	if bodyNode := headerNode.NextSibling(); bodyNode != nil {
		for row := bodyNode.FirstChild(); row != nil; row = row.NextSibling() {
			rows = append(rows, c.CompileTableRow(row))
		}
	}

	return &MarginBlock{
		Block:   &TableBlock{header: header, rows: rows, frameColor: c.tableFrameColor},
		margins: c.tableMargins,
	}
}

// CompileTableRow compiles the cells of a TableHeader or a TableRow -
// both have TableCell children directly, no intermediate node, so one
// method handles both despite the different AST kinds.
func (c *MarkdownCompiler) CompileTableRow(node gmast.Node) []tableCell {
	var cells []tableCell
	for cellNode := node.FirstChild(); cellNode != nil; cellNode = cellNode.NextSibling() {
		tc := cellNode.(*extast.TableCell)
		var parts []Inline
		for child := tc.FirstChild(); child != nil; child = child.NextSibling() {
			parts = c.AppendInlineNode(parts, child, inlineStyle{size: c.tableCellStyle.Size, color: color.White})
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
// straight through to its children.
type inlineStyle struct {
	baseLevel int
	size      float64
	color     color.Color
	strike    bool
}

func (c *MarkdownCompiler) AppendInlineNode(items []Inline, node gmast.Node, st inlineStyle) []Inline {
	switch node.Kind() {
	case gmast.KindText:
		t := node.(*gmast.Text)
		return appendString(items, t.Value.Value(c.source), getStyle(st.baseLevel, st.size), st.color, st.strike)
	case gmast.KindEmphasis:
		child := node.FirstChild()
		childStyle := st
		childStyle.baseLevel++
		for child != nil {
			items = c.AppendInlineNode(items, child, childStyle)
			child = child.NextSibling()
		}
		return items
	case gmast.KindStrong:
		child := node.FirstChild()
		childStyle := st
		childStyle.baseLevel += 2
		for child != nil {
			items = c.AppendInlineNode(items, child, childStyle)
			child = child.NextSibling()
		}
		return items
	case gmast.KindCodeSpan:
		style := getStyle(st.baseLevel, st.size)
		style.Family = Monospace
		cs := node.(*gmast.CodeSpan)
		return appendString(items, cs.Value.Value(c.source), style, c.codeColor, st.strike)
	case gmast.KindImage:
		imgNode := node.(*gmast.Image)
		return append(items, &InlineImage{
			src:   imgNode.Destination.Value(c.source),
			title: imgNode.Title.Value(c.source),
		})
	case gmast.KindLink:
		child := node.FirstChild()
		childStyle := st
		childStyle.color = c.linkColor
		for child != nil {
			items = c.AppendInlineNode(items, child, childStyle)
			child = child.NextSibling()
		}
		return items
	case gmast.KindAutoLink:
		al := node.(*gmast.AutoLink)
		return appendString(items, al.Label.Value(c.source), getStyle(st.baseLevel, st.size), c.linkColor, st.strike)
	case extast.KindStrikethrough:
		child := node.FirstChild()
		childStyle := st
		childStyle.strike = true
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

func appendString(items []Inline, s string, style TextStyle, color color.Color, strike bool) []Inline {
	textParts := strings.Fields(s)
	for _, part := range textParts {
		items = append(items, &InlineText{
			text:   part,
			color:  color,
			style:  style,
			strike: strike,
		})
	}
	return items
}
