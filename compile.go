package whynot

import (
	"fmt"
	"github.com/arnodel/whynot/internal/styling"
	"log"
	"strings"
	"unicode"

	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	extast "github.com/yuin/goldmark/v2/extension/ast"
	"github.com/yuin/goldmark/v2/parser"
)

// Parse compiles Markdown source into a Document ready to render with a
// View. Parse only builds structure: all appearance (fonts, colors,
// margins, ...) comes from the View's StyleSheet, so the same Document
// can be shown in different styles without parsing it again.
func Parse(source []byte, opts ...ParseOption) *Document {
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
	c := compiler{source: source}
	for _, opt := range opts {
		opt(&c)
	}
	// A nil parent styling.Node is what marks a block as top-level.
	return &Document{
		root:       &StackBlock{blocks: c.compileBlocks(node.FirstChild(), nil)},
		headings:   c.headings,
		soleImages: c.soleImages,
	}
}

// compileBlocks compiles first and its following siblings under parent,
// dropping any that compile to nothing.
func (c *compiler) compileBlocks(first gmast.Node, parent *styling.Node) []Block {
	var blocks []Block
	for node := first; node != nil; node = node.NextSibling() {
		if _, ok := node.(gmast.BlockNode); !ok {
			continue
		}
		if block := c.compileBlock(node, parent); block != nil {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// compileInlines compiles node's children as a fresh run of inline
// content - a paragraph, heading, list item head or table cell.
func (c *compiler) compileInlines(node gmast.Node, astNode *styling.Node) []Inline {
	c.pendingSpace = true
	return c.appendChildren(nil, node, astNode)
}

// codeBlockTabExpansion is what a literal tab in a code block's source is
// replaced with - see the KindCodeBlock case below.
const codeBlockTabExpansion = "    "

func (c *compiler) compileBlock(node gmast.Node, parent *styling.Node) Block {
	switch node.Kind() {
	case gmast.KindParagraph:
		return c.compileTextBlock(node, parent.AddChild(styling.TagParagraph), parent == nil)
	case gmast.KindHeading:
		astNode := parent.AddChild(headingTag(node.(*gmast.Heading).Level))
		if attr, ok := node.Attribute("id"); ok {
			astNode.ID = attr.Value(c.source)
		}
		return c.compileTextBlock(node, astNode, parent == nil)
	case gmast.KindList:
		list := node.(*gmast.List)
		astNode := parent.AddChild(styling.TagList)
		var items []Block
		index := 0
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			items = append(items, c.compileListItem(child, list, index, astNode))
			index++
		}
		return &MarginBlock{Block: &StackBlock{blocks: items}, node: astNode}
	case gmast.KindCodeBlock:
		astNode := parent.AddChild(styling.TagCodeBlock)
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

		language, _ := cb.Language(c.source)
		lines := c.codeBlockLines(astNode, language, rawLines)
		if plugin := c.pluginFor(language); plugin != nil {
			// rawLines already carry their own trailing newlines (see
			// highlightLines' identical join) - joining with "" avoids
			// doubling them up.
			code := strings.Join(rawLines, "")
			fallback := &CodeBlock{lines: lines, node: astNode}
			return &MarginBlock{Block: NewDiagramBlock(astNode, plugin.Image(language, code), fallback), node: astNode}
		}
		return &MarginBlock{Block: &CodeBlock{lines: lines, node: astNode}, node: astNode}
	case gmast.KindThematicBreak:
		astNode := parent.AddChild(styling.TagThematicBreak)
		return &MarginBlock{
			Block: &ThematicBreakBlock{node: astNode},
			node:  astNode,
		}
	case gmast.KindBlockquote:
		astNode := parent.AddChild(styling.TagBlockquote)
		return &MarginBlock{
			Block: &BlockquoteBlock{inner: wrapBlocks(c.compileBlocks(node.FirstChild(), astNode)), node: astNode},
			node:  astNode,
		}
	case extast.KindTable:
		return c.compileTable(node, parent)
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

// compileTextBlock compiles a paragraph or heading, whose styling.Node the
// caller has already created. A top-level one is also recorded in the
// Document: a heading as a TOCEntry, and one whose only content is an
// image as a soleImages entry.
func (c *compiler) compileTextBlock(node gmast.Node, astNode *styling.Node, topLevel bool) Block {
	items := c.compileInlines(node, astNode)
	block := &MarginBlock{Block: &TextBlock{parts: items, node: astNode}, node: astNode}

	if !topLevel {
		return block
	}
	if astNode.Tag >= styling.TagHeading1 && astNode.Tag <= styling.TagHeading6 {
		c.headings = append(c.headings, TOCEntry{
			ID:    astNode.ID,
			Level: int(astNode.Tag-styling.TagHeading1) + 1,
			Text:  plainText(items),
		})
	}
	if len(items) == 1 {
		if img, ok := items[0].(*InlineImage); ok {
			if c.soleImages == nil {
				c.soleImages = make(map[Block]string)
			}
			c.soleImages[block] = img.src
		}
	}
	return block
}

// plainText joins items' words with single spaces; anything but an
// *InlineText (e.g. an image) contributes nothing.
func plainText(items []Inline) string {
	var words []string
	for _, item := range items {
		if it, ok := item.(*InlineText); ok {
			words = append(words, it.text)
		}
	}
	return strings.Join(words, " ")
}

// compileUnsupportedBlock handles any block-level Markdown construct
// whynot doesn't have a case for above. Rather than taking down the
// whole document, it logs a warning and renders the construct as a
// code block in StyleSheet.UnsupportedColor, showing its raw source
// where possible so the gap is visible rather than silently dropped.
func (c *compiler) compileUnsupportedBlock(node gmast.Node, parent *styling.Node) Block {
	astNode := parent.AddChild(styling.TagUnsupported)
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

// headingTag maps a heading level (1-6) to its styling.Tag - safe because
// styling.TagHeading1..TagHeading6 are declared consecutively.
func headingTag(level int) styling.Tag {
	return styling.TagHeading1 + styling.Tag(level-1)
}

// compileListItem compiles the item at index (0-based) in list.
func (c *compiler) compileListItem(node gmast.Node, list *gmast.List, index int, parent *styling.Node) Block {
	itemNode := parent.AddChild(styling.TagListItem)
	var marker Inline
	if status, ok := extension.TaskStatusOf(node); ok {
		marker = &TaskCheckbox{checked: status == extension.TaskStatusCompleted, node: itemNode}
	} else {
		marker = &InlineText{text: listMarker(list, index), node: itemNode}
	}

	// A leading Paragraph is the item's own text, flowed with the marker
	// hanging off its first line (see ListItemHeadBlock.GetBlockLayout).
	// Anything after it - a nested List, or (in a loose list) further
	// paragraphs - stacks below as trailing block content. An item with no
	// leading paragraph has no parts, so the marker ends up on a line of
	// its own.
	var parts []Inline
	next := node.FirstChild()
	if next != nil && next.Kind() == gmast.KindParagraph {
		parts = c.compileInlines(next, itemNode)
		next = next.NextSibling()
	}

	head := Block(&ListItemHeadBlock{
		marker: marker,
		parts:  parts,
		node:   itemNode,
	})
	if !list.IsTight {
		// Loose items get real paragraph spacing on their own leading text
		// too, not a tight head's zero margins. This also grows the gap
		// between items with no separate constant: StackBlock.Margins()
		// reports its first child's margin, so it collapses outward
		// through the item's own MarginBlock and the list's own
		// StackBlock, like any sibling gap.
		//
		// A dedicated styling.TagParagraph child, not itemNode itself: the head's
		// own margins should resolve as a paragraph's (matching what this
		// looked like before StyleSheet), distinct from itemNode's tag,
		// which the marker/parts' own inline styling still uses.
		head = &MarginBlock{Block: head, node: itemNode.AddChild(styling.TagParagraph)}
	}
	blocks := []Block{head}

	if trailingBlocks := c.compileBlocks(next, itemNode); len(trailingBlocks) > 0 {
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

// listMarker is the marker text for the item at index (0-based) in list:
// the bullet character itself, or the item's number (counting from the
// list's own start number) followed by the list's delimiter.
func listMarker(list *gmast.List, index int) string {
	if list.IsOrdered() {
		return fmt.Sprintf("%d%c", list.Start+index, list.Marker)
	}
	return string(list.Marker)
}

// compileTable compiles a Table node. The header is mandatory (GFM
// requires it); the body is not - a table can legitimately have zero
// data rows, in which case Table has no TableBody child at all.
func (c *compiler) compileTable(node gmast.Node, parent *styling.Node) Block {
	astNode := parent.AddChild(styling.TagTable)
	headerNode := node.FirstChild()
	header := c.compileTableRow(headerNode, astNode)

	var rows [][]tableCell
	if bodyNode := headerNode.NextSibling(); bodyNode != nil {
		for row := bodyNode.FirstChild(); row != nil; row = row.NextSibling() {
			rows = append(rows, c.compileTableRow(row, astNode))
		}
	}

	return &MarginBlock{
		Block: &TableBlock{header: header, rows: rows, node: astNode},
		node:  astNode,
	}
}

// compileTableRow compiles the cells of a TableHeader or a TableRow -
// both have TableCell children directly, no intermediate node, so one
// method handles both despite the different AST kinds.
func (c *compiler) compileTableRow(node gmast.Node, parent *styling.Node) []tableCell {
	var cells []tableCell
	for cellNode := node.FirstChild(); cellNode != nil; cellNode = cellNode.NextSibling() {
		tc := cellNode.(*extast.TableCell)
		cellNode := parent.AddChild(styling.TagTableCell)
		cells = append(cells, tableCell{
			content:   &TextBlock{parts: c.compileInlines(tc, cellNode), node: cellNode},
			alignment: tableCellAlignment(tc.Alignment),
		})
	}
	return cells
}

// spanTags maps each inline construct that only wraps other inline
// content to the styling.Tag it applies to that content.
var spanTags = map[gmast.NodeKind]styling.Tag{
	gmast.KindEmphasis:       styling.TagEmphasis,
	gmast.KindStrong:         styling.TagStrong,
	extast.KindStrikethrough: styling.TagStrikethrough,
}

// appendChildren appends node's inline children, under astNode.
func (c *compiler) appendChildren(items []Inline, node gmast.Node, astNode *styling.Node) []Inline {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		items = c.appendInline(items, child, astNode)
	}
	return items
}

// appendInline walks an inline subtree, appending each leaf as an
// Inline. astNode is node's parent in the styling.Node tree - extended only by
// the constructs that get their own styling.Tag, and threaded straight through
// everywhere else. Appearance (font, color, strike) is never resolved
// here - each produced Inline just carries the styling.Node it was created
// under, resolved later by RenderingContext against a StyleSheet.
func (c *compiler) appendInline(items []Inline, node gmast.Node, astNode *styling.Node) []Inline {
	if tag, ok := spanTags[node.Kind()]; ok {
		return c.appendChildren(items, node, astNode.AddChild(tag))
	}
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
	case gmast.KindCodeSpan:
		cs := node.(*gmast.CodeSpan)
		childNode := astNode.AddChild(styling.TagCodeSpan)
		return c.appendString(items, cs.Value.Value(c.source), childNode)
	case gmast.KindImage:
		imgNode := node.(*gmast.Image)
		imageNode := astNode.AddChild(styling.TagImage)
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
			// (every resize/zoom/reload), and styling.Node.AddChild
			// isn't idempotent, so mutating the tree there would
			// grow a new child every time instead of reusing one.
			fallbackNode: imageNode.AddChild(styling.TagUnsupported),
			glued:        glued,
		})
	case gmast.KindLink:
		linkNode := astNode.AddChild(styling.TagLink)
		linkNode.Destination = node.(*gmast.Link).Destination.Value(c.source)
		return c.appendChildren(items, node, linkNode)
	case gmast.KindAutoLink:
		al := node.(*gmast.AutoLink)
		childNode := astNode.AddChild(styling.TagLink)
		childNode.Destination = al.Destination.Value(c.source)
		return c.appendString(items, al.Label.Value(c.source), childNode)
	default:
		return c.appendUnsupportedInline(items, node, astNode)
	}
}

// appendUnsupportedInline is appendInline's fallback for any inline
// Markdown construct whynot doesn't have a case for - the inline
// counterpart to compileUnsupportedBlock. Inline content can't hold a
// block-level box, so the raw source (where available) is spliced into
// the surrounding paragraph as ordinary words, styled in
// StyleSheet.UnsupportedColor via styling.TagUnsupported.
func (c *compiler) appendUnsupportedInline(items []Inline, node gmast.Node, astNode *styling.Node) []Inline {
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
	return c.appendString(items, text, astNode.AddChild(styling.TagUnsupported))
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
// compiler.pendingSpace's own doc comment for why that carry is
// needed at all (a whitespace-only Text node between two non-text
// siblings, e.g. "**a** *b*", produces zero items of its own here but
// still needs to un-glue whatever comes next).
func (c *compiler) appendString(items []Inline, s string, node *styling.Node) []Inline {
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
