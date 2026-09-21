package whynot

import (
	"fmt"
	"log"
	"strings"
	"unicode"

	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	extast "github.com/yuin/goldmark/v2/extension/ast"
	"github.com/yuin/goldmark/v2/parser"
)

// Parse compiles Markdown source into a Block tree ready for layout via
// Block.GetBlockLayout. All appearance (fonts, colors, margins, ...) is resolved
// later, from RenderingContext.StyleSheet against each Block/Inline's own
// ASTNode - Parse itself only builds structure.
func Parse(source []byte, opts ...ParseOption) Block {
	p := parser.New(
		parser.WithExtensions(
			extension.TaskListItemParser,
			extension.StrikethroughParser,
			extension.TableParser,
			// Substitutes straight quotes/dashes/ellipsis for their
			// typographic equivalents ("x" -> "x", -- -> en dash, etc.) as
			// a plain Text node, same as any other inline text - its
			// default substitutions are HTML entities (e.g. "&rsquo;"),
			// but Text.Value's decoder resolves those the same way it
			// already resolves &nbsp;/&amp; in ordinary prose (see
			// TestParseResolvesEntitiesAndEscapes), so no extra config is
			// needed to get literal runes out of it.
			extension.TypographerParser,
		),
		parser.WithAutoHeadingID(),
	)
	node := p.Parse(source)
	compiler := MarkdownCompiler{source: source}
	for _, opt := range opts {
		opt(&compiler)
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
		c.pendingSpace = true
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
		c.pendingSpace = true
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
		rawLines := make([]string, len(segs))
		for i, seg := range segs {
			// A code block's content is verbatim source, tabs included -
			// unlike indentation elsewhere in the document, this isn't
			// tab-expanded before it reaches us. Most fonts have no glyph
			// for a raw tab, rendering it as a placeholder box instead of
			// whitespace, so expand it here to keep indentation looking
			// like indentation.
			rawLines[i] = strings.ReplaceAll(string(seg.Bytes(c.source)), "\t", codeBlockTabExpansion)
		}

		var lines [][]Inline
		if c.highlighter != nil {
			language, _ := cb.Language(c.source)
			lines = highlightLines(c.highlighter, astNode, language, rawLines)
		}
		if lines == nil {
			// No highlighter configured, or highlightLines bailed out on
			// a mismatched line count (a misbehaving Highlighter) - both
			// cases fall back to the same plain, one-InlineText-per-line
			// rendering.
			lines = make([][]Inline, len(rawLines))
			for i, text := range rawLines {
				lines[i] = []Inline{&InlineText{text: text, node: astNode}}
			}
		}
		return &MarginBlock{Block: &CodeBlock{lines: lines, node: astNode}, node: astNode}
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
	lines := make([][]Inline, len(items))
	for i, item := range items {
		lines[i] = []Inline{item}
	}
	return &MarginBlock{Block: &CodeBlock{lines: lines, node: astNode}, node: astNode}
}

// headingTag maps a heading level (1-6) to its ASTTag - safe because
// TagHeading1..TagHeading6 are declared consecutively.
func headingTag(level int) ASTTag {
	return TagHeading1 + ASTTag(level-1)
}

func (c *MarkdownCompiler) CompileListItem(node gmast.Node, index int, marker byte, tight bool, parent *ASTNode) Block {
	itemNode := parent.AddChild(TagListItem)
	var items []Inline
	var markerInline Inline
	if status, ok := extension.TaskStatusOf(node); ok {
		markerInline = &TaskCheckbox{checked: status == extension.TaskStatusCompleted, node: itemNode}
	} else {
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
		markerInline = &InlineText{text: markerString, node: itemNode}
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
		c.pendingSpace = true
		child := next.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, itemNode)
			child = child.NextSibling()
		}
		next = next.NextSibling()
	}

	head := Block(&ListItemHeadBlock{
		marker: markerInline,
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
		c.pendingSpace = true
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
		items = c.appendString(items, t.Value.Value(c.source), astNode)
		// The newline itself isn't in either Text node's Value - goldmark
		// represents a line break purely via this flag - so without this,
		// the next word would glue on with no space (e.g. "laid\nout" ->
		// "laidout"). No forced-break rendering exists yet, so both
		// kinds just become a space.
		if t.SoftLineBreak() || t.HardLineBreak() {
			c.pendingSpace = true
		}
		return items
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
		return c.appendString(items, cs.Value.Value(c.source), childNode)
	case gmast.KindImage:
		imgNode := node.(*gmast.Image)
		imageNode := astNode.AddChild(TagImage)
		glued := !c.pendingSpace
		c.pendingSpace = false
		return append(items, &InlineImage{
			src:   imgNode.Destination.Value(c.source),
			alt:   altText(imgNode, c.source),
			title: imgNode.Title.Value(c.source),
			node:  imageNode,
			// fallbackNode is precomputed once, here, rather than
			// on demand inside GetInlineLayout - a layout-time
			// "does the image load" check can run many times
			// (every resize/zoom/reload), and ASTNode.AddChild
			// isn't idempotent, so mutating the tree there would
			// grow a new child every time instead of reusing one.
			fallbackNode: imageNode.AddChild(TagUnsupported),
			glued:        glued,
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
		return c.appendString(items, al.Label.Value(c.source), childNode)
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
	return c.appendString(items, text, astNode.AddChild(TagUnsupported))
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

// altText flattens an image's child nodes - CommonMark allows arbitrary
// inline content in an image's alt-text description (`![a *b*](x.png)`
// is valid) - into a plain string, the same way HTML rendering flattens
// it into an <img alt="..."> attribute. whynot has nowhere to show
// formatted alt text either, so this recurses into any node kind
// generically rather than special-casing Emphasis/Strong/etc.
func altText(node gmast.Node, source []byte) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == gmast.KindText {
			b.WriteString(child.(*gmast.Text).Value.Value(source))
		} else {
			b.WriteString(altText(child, source))
		}
	}
	return b.String()
}

// nbsp is a non-breaking space (U+00A0) - what a literal NBSP character or
// an `&nbsp;` entity in the source both normalize to by the time goldmark
// hands us a Text node's Value (there's no AST-level distinction between
// them, or from an ordinary space, confirmed against goldmark v2 directly).
const nbsp = ' '

// appendString splits s into Inline items along the same lines a browser
// would collapse/wrap plain text: each maximal run of ordinary breakable
// whitespace becomes a gap between words (as strings.Fields did before),
// but unlike strings.Fields, a literal non-breaking space is never treated
// as that kind of gap - it becomes its own atomic word instead, so it
// can carry Glued (see InlineLayout.Glued) on both sides and end up
// visually spaced but never a line-break point.
//
// Each produced item's Glued reflects c.pendingSpace, the whitespace
// carried over from wherever the previous item (in this call or an
// earlier one, however many sibling nodes back) left off - see
// MarkdownCompiler.pendingSpace's own doc comment for why that carry is
// needed at all (a whitespace-only Text node between two non-text
// siblings, e.g. "**a** *b*", produces zero items of its own here but
// still needs to un-glue whatever comes next).
func (c *MarkdownCompiler) appendString(items []Inline, s string, node *ASTNode) []Inline {
	runes := []rune(s)
	for i := 0; i < len(runes); {
		switch r := runes[i]; {
		case r == nbsp:
			items = append(items, &InlineText{text: string(nbsp), node: node, glued: !c.pendingSpace})
			c.pendingSpace = false
			i++
		case unicode.IsSpace(r):
			c.pendingSpace = true
			i++
		default:
			start := i
			for i < len(runes) && runes[i] != nbsp && !unicode.IsSpace(runes[i]) {
				i++
			}
			items = append(items, &InlineText{text: string(runes[start:i]), node: node, glued: !c.pendingSpace})
			c.pendingSpace = false
		}
	}
	return items
}
