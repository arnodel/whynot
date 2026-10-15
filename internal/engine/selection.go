package engine

import (
	"bytes"
	"strings"

	"github.com/arnodel/whynot/internal/ast"
)

// Selection works on a tree of a document's Blocks, as a reader sees them:
// leaves hold text (a paragraph, a heading, a list item's text, a table
// cell, a code block), and containers hold leaves or other containers (the
// document, a list, a list item, a blockquote, a table and its rows).
// Wrappers that don't change what's selected, such as a MarginBlock, are
// seen through.
//
// A selection runs between two Positions. Within a leaf, it's a range of
// characters; across leaves, it's the run of children of the innermost
// container holding both ends, from the child holding one end to the
// child holding the other, each taken whole (see SelTree.Normalize).

// SelNode is a node of a SelTree.
type SelNode struct {
	// Block is the node's Block, nil for a table row.
	Block  Block
	Parent *SelNode
	// Index is the node's index in Parent.Kids.
	Index int
	Kids  []*SelNode

	// For a leaf, items are its inline items, a code block's line by line
	// (see lineStarts), and marker a list item's marker. A leaf without
	// items, such as a thematic break or a diagram, can only be selected
	// whole.
	leaf       bool
	items      []Inline
	lineStarts []int // a code block's, the index of each line's first item
	marker     Inline

	// table is set for a table and its rows; row is a row's index, the
	// header's being 0.
	table *TableBlock
	row   int
	// tag is the semantic tag of the node's block, if it has one: lists
	// and list items are told apart by it.
	tag ast.Tag
}

// SelTree is the selection tree of a document's top-level blocks.
type SelTree struct {
	Root *SelNode
	// where maps each item of a leaf to its leaf and index, and leaves
	// each leaf's Block to its node.
	where  map[Inline]Position
	leaves map[Block]*SelNode
}

// A Position is a place in a leaf's text: before the byte at Offset in
// the text of its item at Item, or after it, at its end. In a leaf
// without items, Offset 0 is its start and 1 its end.
type Position struct {
	Leaf   *SelNode
	Item   int
	Offset int
}

// NewSelTree builds the selection tree of blocks.
func NewSelTree(blocks []Block) *SelTree {
	t := &SelTree{where: map[Inline]Position{}, leaves: map[Block]*SelNode{}}
	t.Root = &SelNode{}
	for _, b := range blocks {
		t.add(t.Root, b)
	}
	return t
}

// add adds b's node under parent, unless it shows nothing.
func (t *SelTree) add(parent *SelNode, b Block) {
	n := &SelNode{Block: b, Parent: parent, Index: len(parent.Kids)}
	if node := b.Node(); node != nil {
		n.tag = node.Tag
	}
	t.fill(n, b)
	if !n.leaf && len(n.Kids) == 0 {
		return
	}
	parent.Kids = append(parent.Kids, n)
	if n.leaf {
		t.leaves[b] = n
	}
	for i, item := range n.items {
		t.where[item] = Position{Leaf: n, Item: i}
	}
}

// fill makes n the node of b: a leaf, or a container of b's children.
func (t *SelTree) fill(n *SelNode, b Block) {
	switch b := b.(type) {
	case *MarginBlock:
		if n.tag == 0 && b.MarginNode != nil {
			n.tag = b.MarginNode.Tag
		}
		t.fill(n, b.Block)
	case *StackBlock:
		if len(b.Blocks) == 1 {
			t.fill(n, b.Blocks[0])
			return
		}
		for _, child := range b.Blocks {
			t.add(n, child)
		}
	case *BlockquoteBlock:
		n.tag = ast.TagBlockquote
		if s, ok := unwrap(b.Inner).(*StackBlock); ok {
			for _, child := range s.Blocks {
				t.add(n, child)
			}
		} else {
			t.add(n, b.Inner)
		}
	case *TableBlock:
		n.table, n.tag = b, ast.TagTable
		for i, cells := range append([][]TableCell{b.Header}, b.Rows...) {
			row := &SelNode{Parent: n, Index: i, table: b, row: i}
			for _, cell := range cells {
				t.add(row, cell.Content)
			}
			n.Kids = append(n.Kids, row)
		}
	case *TextBlock:
		n.leaf, n.items = true, b.Parts
	case *ListItemHeadBlock:
		n.leaf, n.items, n.marker = true, b.Parts, b.Marker
	case *CodeBlock:
		n.leaf = true
		for _, line := range b.Lines {
			n.lineStarts = append(n.lineStarts, len(n.items))
			n.items = append(n.items, line...)
		}
	default:
		// A thematic break, a diagram: selected whole.
		n.leaf = true
	}
}

// unwrap returns b without the MarginBlocks around it.
func unwrap(b Block) Block {
	for {
		m, ok := b.(*MarginBlock)
		if !ok {
			return b
		}
		b = m.Block
	}
}

// PositionOf returns the Position before the byte at offset in item.
func (t *SelTree) PositionOf(item Inline, offset int) (Position, bool) {
	p, ok := t.where[item]
	p.Offset = offset
	return p, ok
}

// Carry returns p, a Position in another tree of the same document, in
// this one, if its leaf is still there: a growing document's last blocks
// are replaced as more arrives.
func (t *SelTree) Carry(p Position) (Position, bool) {
	if p.Leaf == nil {
		return Position{}, false
	}
	leaf, ok := t.leaves[p.Leaf.Block]
	if !ok {
		return Position{}, false
	}
	p.Leaf = leaf
	return p, true
}

// Start and End return the first and last Positions of the tree.
func (t *SelTree) Start() (Position, bool) {
	leaf := first(t.Root)
	return Position{Leaf: leaf}, leaf != nil
}

func (t *SelTree) End() (Position, bool) {
	leaf := last(t.Root)
	if leaf == nil {
		return Position{}, false
	}
	return leaf.end(), true
}

func first(n *SelNode) *SelNode {
	for n != nil && !n.leaf {
		if len(n.Kids) == 0 {
			return nil
		}
		n = n.Kids[0]
	}
	return n
}

func last(n *SelNode) *SelNode {
	for n != nil && !n.leaf {
		if len(n.Kids) == 0 {
			return nil
		}
		n = n.Kids[len(n.Kids)-1]
	}
	return n
}

// end returns the Position at the end of a leaf.
func (n *SelNode) end() Position {
	if len(n.items) == 0 {
		return Position{Leaf: n, Offset: 1}
	}
	i := len(n.items) - 1
	return Position{Leaf: n, Item: i, Offset: len(itemText(n.items[i]))}
}

// Word returns the Positions at the start and end of p's item: a word,
// or a whole leaf without items.
func (p Position) Word() (start, end Position) {
	if len(p.Leaf.items) == 0 {
		return Position{Leaf: p.Leaf}, p.Leaf.end()
	}
	return Position{Leaf: p.Leaf, Item: p.Item}, Position{Leaf: p.Leaf, Item: p.Item, Offset: len(itemText(p.Leaf.items[p.Item]))}
}

// Block returns the Positions at the start and end of p's leaf.
func (p Position) Block() (start, end Position) {
	return Position{Leaf: p.Leaf}, p.Leaf.end()
}

// path returns the indices from the root down to n.
func (n *SelNode) path() []int {
	var p []int
	for ; n.Parent != nil; n = n.Parent {
		p = append(p, n.Index)
	}
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
	return p
}

// Compare returns -1, 0 or 1 as a is before, at or after b.
func Compare(a, b Position) int {
	if a.Leaf != b.Leaf {
		pa, pb := a.Leaf.path(), b.Leaf.path()
		for i := 0; i < len(pa) && i < len(pb); i++ {
			if pa[i] != pb[i] {
				return cmp(pa[i], pb[i])
			}
		}
		return cmp(len(pa), len(pb))
	}
	if a.Item != b.Item {
		return cmp(a.Item, b.Item)
	}
	return cmp(a.Offset, b.Offset)
}

func cmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// A Span is a normalized selection: a range of characters in a leaf, From
// to To, if Within is set, or else the run of Container's Kids from First
// to Last, each whole.
type Span struct {
	Within      *SelNode
	From, To    Position
	Container   *SelNode
	First, Last int
}

// Normalize returns the Span of the selection between a and b, and
// whether it selects anything.
func Normalize(a, b Position) (Span, bool) {
	if Compare(a, b) > 0 {
		a, b = b, a
	}
	if a.Leaf == b.Leaf {
		return Span{Within: a.Leaf, From: a, To: b}, Compare(a, b) != 0
	}
	ancestors := map[*SelNode]*SelNode{} // ancestor to its child towards a
	for child, n := a.Leaf, a.Leaf.Parent; n != nil; child, n = n, n.Parent {
		ancestors[n] = child
	}
	child := b.Leaf
	for n := b.Leaf.Parent; n != nil; child, n = n, n.Parent {
		if towardsA, ok := ancestors[n]; ok {
			return Span{Container: n, First: towardsA.Index, Last: child.Index}, true
		}
	}
	return Span{}, false
}

// NormalizeWhole is Normalize for a selection that has left the
// document, carried on into another one: it's whole blocks even with both
// ends in one leaf, which it then selects whole.
func NormalizeWhole(a, b Position) (Span, bool) {
	if a.Leaf != nil && a.Leaf == b.Leaf {
		return Span{Container: a.Leaf.Parent, First: a.Leaf.Index, Last: a.Leaf.Index}, true
	}
	return Normalize(a, b)
}

// Contains reports whether the Span selects the leaf n whole, or, within
// it, which bytes of its item at index: from start to end.
func (s Span) Contains(n *SelNode, item int, textLen int) (start, end int, ok bool) {
	if s.Within != nil {
		if n != s.Within || item < s.From.Item || item > s.To.Item {
			return 0, 0, false
		}
		start, end = 0, textLen
		if item == s.From.Item {
			start = s.From.Offset
		}
		if item == s.To.Item {
			end = s.To.Offset
		}
		return start, end, start < end
	}
	// Whole: n is under one of the Container's Kids in the run.
	for m := n; m.Parent != nil; m = m.Parent {
		if m.Parent == s.Container {
			return 0, textLen, m.Index >= s.First && m.Index <= s.Last
		}
	}
	return 0, 0, false
}

// itemText returns an item's text as selection sees it.
func itemText(item Inline) string {
	switch item := item.(type) {
	case *InlineText:
		return item.Text
	case *InlineImage:
		return item.Alt
	}
	return ""
}

// Text returns the Span's plain text: within a leaf, the exact range;
// across leaves, each whole, separated by blank lines, list items by line
// breaks and keeping their markers, and table cells by tabs.
func (s Span) Text() string {
	if s.Within != nil {
		return leafText(s.Within, s.From, s.To)
	}
	var parts []string
	for _, kid := range s.Container.Kids[s.First : s.Last+1] {
		parts = append(parts, nodeText(kid))
	}
	return strings.Join(parts, separator(s.Container))
}

// separator returns what separates the plain text of n's Kids.
func separator(n *SelNode) string {
	switch {
	case n.table != nil && n.Block == nil:
		return "\t" // a row's cells
	case n.table != nil, n.tag == ast.TagList:
		return "\n"
	}
	return "\n\n"
}

// nodeText returns the plain text of n, whole.
func nodeText(n *SelNode) string {
	if n.leaf {
		text := leafText(n, Position{Leaf: n}, n.end())
		if n.marker != nil {
			text = markerText(n.marker) + " " + text
		}
		return text
	}
	var parts []string
	for _, kid := range n.Kids {
		parts = append(parts, nodeText(kid))
	}
	return strings.Join(parts, separator(n))
}

// markerText returns a list item marker's plain text.
func markerText(m Inline) string {
	switch m := m.(type) {
	case *InlineText:
		return m.Text
	case *TaskCheckbox:
		if m.Checked {
			return "[x]"
		}
		return "[ ]"
	}
	return ""
}

// leafText returns the plain text of a leaf from a to b: words separated
// as they show, a hard line break as a newline, and a code block's lines
// on lines of their own.
func leafText(n *SelNode, a, b Position) string {
	var sb strings.Builder
	line := 0
	for i := a.Item; i <= b.Item && i < len(n.items); i++ {
		item := n.items[i]
		text := itemText(item)
		from, to := 0, len(text)
		if i == a.Item {
			from = min(a.Offset, to)
		}
		if i == b.Item {
			to = min(b.Offset, to)
		}
		if i > a.Item {
			switch {
			case n.lineStarts != nil:
				for line+1 < len(n.lineStarts) && n.lineStarts[line+1] <= i {
					line++
					if !strings.HasSuffix(sb.String(), "\n") {
						sb.WriteByte('\n')
					}
				}
			case isLineBreak(item):
				sb.WriteByte('\n')
			case !glued(item) && sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n"):
				sb.WriteByte(' ')
			}
		}
		if from < to {
			sb.WriteString(text[from:to])
		}
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func isLineBreak(item Inline) bool {
	_, ok := item.(*LineBreak)
	return ok
}

func glued(item Inline) bool {
	switch item := item.(type) {
	case *InlineText:
		return item.Glued
	case *InlineImage:
		return item.Glued
	}
	return false
}

// Markdown returns the Span's Markdown, from source, the document's:
// whole blocks as their source; table rows, or a slice of a row's cells,
// as a table with its header; and within a leaf, the range widened to the
// edges of the outermost inline elements it cuts.
func (s Span) Markdown(source []byte) string {
	if s.Within != nil {
		return leafMarkdown(s.Within, s.From, s.To, source)
	}
	c := s.Container
	switch {
	case c.table != nil && c.Block == nil:
		return tableMarkdown(c.table, []int{c.row}, s.First, s.Last, source)
	case c.table != nil:
		rows := []int{}
		for i := max(s.First, 1); i <= s.Last; i++ {
			rows = append(rows, i)
		}
		return tableMarkdown(c.table, rows, 0, len(c.table.Header)-1, source)
	}
	first, last := c.Kids[s.First], c.Kids[s.Last]
	r1, ok1 := nodeSource(first)
	r2, ok2 := nodeSource(last)
	if !ok1 || !ok2 {
		return s.Text()
	}
	return string(source[lineStart(source, r1.Start):r2.End])
}

// nodeSource returns where n is in the source.
func nodeSource(n *SelNode) (ast.Range, bool) {
	if n.Block != nil {
		if node := blockNode(n.Block); node != nil && node.Source != (ast.Range{}) {
			return node.Source, true
		}
	}
	if len(n.Kids) == 0 {
		return ast.Range{}, false
	}
	a, ok1 := nodeSource(n.Kids[0])
	b, ok2 := nodeSource(n.Kids[len(n.Kids)-1])
	return ast.Range{Start: a.Start, End: b.End}, ok1 && ok2
}

// blockNode returns b's node, or, for a wrapper without one of its own,
// its content's.
func blockNode(b Block) *ast.Node {
	for b != nil {
		if m, ok := b.(*MarginBlock); ok {
			if m.MarginNode != nil && m.MarginNode.Source != (ast.Range{}) {
				return m.MarginNode
			}
			b = m.Block
			continue
		}
		return b.Node()
	}
	return nil
}

// lineStart returns the start of the line holding offset i.
func lineStart(source []byte, i int) int {
	return bytes.LastIndexByte(source[:i], '\n') + 1
}

// lineEnd returns the end of the line holding offset i, before its newline.
func lineEnd(source []byte, i int) int {
	if j := bytes.IndexByte(source[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(source)
}

// tableMarkdown returns a table of t's header and the given rows (indices
// in the header-first rows), with the columns from first to last.
func tableMarkdown(t *TableBlock, rows []int, first, last int, source []byte) string {
	all := append([][]TableCell{t.Header}, t.Rows...)
	cellText := func(c TableCell) string {
		if c.Content.ASTNode != nil && c.Content.ASTNode.Source != (ast.Range{}) {
			r := c.Content.ASTNode.Source
			return string(source[r.Start:r.End])
		}
		return leafTextOf(c.Content)
	}
	// The separator row's cells, from the source: they hold the columns'
	// alignment.
	seps := make([]string, len(t.Header))
	for i := range seps {
		seps[i] = "---"
	}
	if len(t.Header) > 0 && t.Header[0].Content.ASTNode != nil && t.Header[0].Content.ASTNode.Source != (ast.Range{}) {
		end := lineEnd(source, t.Header[0].Content.ASTNode.Source.Start)
		if end < len(source) {
			line := strings.Trim(strings.TrimSpace(string(source[end+1:lineEnd(source, end+1)])), "|")
			if cells := strings.Split(line, "|"); len(cells) == len(seps) {
				for i, c := range cells {
					seps[i] = strings.TrimSpace(c)
				}
			}
		}
	}
	row := func(cells []string) string { return "| " + strings.Join(cells, " | ") + " |" }
	var lines []string
	pick := func(r []TableCell, f func(TableCell) string) []string {
		var out []string
		for i := first; i <= last && i < len(r); i++ {
			out = append(out, f(r[i]))
		}
		return out
	}
	lines = append(lines, row(pick(all[0], cellText)), row(seps[first:min(last+1, len(seps))]))
	for _, i := range rows {
		if i > 0 && i < len(all) {
			lines = append(lines, row(pick(all[i], cellText)))
		}
	}
	return strings.Join(lines, "\n")
}

// leafTextOf returns a TextBlock's plain text.
func leafTextOf(b *TextBlock) string {
	n := &SelNode{leaf: true, items: b.Parts}
	return leafText(n, Position{Leaf: n}, n.end())
}

// leafMarkdown returns the Markdown of a leaf from a to b: the source
// between them, widened to the edges of the outermost inline elements
// either end falls inside.
func leafMarkdown(n *SelNode, a, b Position, source []byte) string {
	start, ok1 := sourceAt(n, a, true)
	end, ok2 := sourceAt(n, b, false)
	if !ok1 || !ok2 || end < start {
		return leafText(n, a, b)
	}
	return string(source[start:end])
}

// sourceAt returns where p is in the source, widened to the start (or
// end, if !start) of the outermost inline element of n it's inside.
func sourceAt(n *SelNode, p Position, start bool) (int, bool) {
	if p.Item >= len(n.items) {
		return 0, false
	}
	item := n.items[p.Item]
	w, ok := item.(*InlineText)
	var r ast.Range
	switch {
	case ok && w.Source != (ast.Range{}):
		r = w.Source
	case item.Node() != nil && item.Node().Source != (ast.Range{}):
		r = item.Node().Source
	default:
		return 0, false
	}
	// A word that isn't exactly its source (an escape, an entity) is taken
	// whole, unless p is at its edge.
	var at int
	switch {
	case ok && w.SourceExact:
		at = r.Start + p.Offset
	case start && p.Offset < len(itemText(item)), !start && p.Offset == 0:
		at = r.Start
	default:
		at = r.End
	}
	// The outermost inline element p is inside: the ancestor of the
	// item's node just below the leaf's.
	leafNode := blockNode(n.Block)
	var outer *ast.Node
	for m := item.Node(); m != nil && m != leafNode; m = m.Parent {
		if m.Source != (ast.Range{}) && isInlineTag(m.Tag) {
			outer = m
		}
	}
	if outer != nil {
		if start {
			at = min(at, outer.Source.Start)
		} else {
			at = max(at, outer.Source.End)
		}
	}
	return at, true
}

func isInlineTag(tag ast.Tag) bool {
	switch tag {
	case ast.TagLink, ast.TagCodeSpan, ast.TagEmphasis, ast.TagStrong, ast.TagStrikethrough, ast.TagImage:
		return true
	}
	return false
}
