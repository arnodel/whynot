package whynot

import (
	"math"
	"sort"
)

type Margins struct {
	Top, Bottom, Left, Right float64
}

// Source is the common ground between Block and Inline: something with
// a semantic identity in the compiled ASTNode tree. Leaf boxes expose
// it so a hit-test result traces back to its origin - and since the
// concrete value is the real Block/Inline, a caller can type-assert
// further for anything beyond the node itself.
type Source interface {
	Node() *ASTNode
}

type Block interface {
	Source
	GetBlockLayout(ctx RenderingContext, width int) BlockLayout
	Margins(ctx RenderingContext) Margins
}

// Marginer is anything that reports its own logical (unscaled) Margins -
// every Block satisfies it, but it's kept narrow so RenderingContext.
// ScaledMargins doesn't need the rest of the Block interface.
type Marginer interface {
	Margins(ctx RenderingContext) Margins
}

// WithoutMargins satisfies Block's Margins() with a zero value, for content
// that never gets margins of its own (a list item's head, a table cell's
// content).
type WithoutMargins struct{}

func (WithoutMargins) Margins(ctx RenderingContext) Margins {
	return Margins{}
}

// MarginBlock adds margins to an existing Block, collapsing Top/Bottom with
// whatever it already reports. Left/Right are just MarginBlock's own value:
// nothing it wraps ever reports a nonzero Left/Right to collapse with -
// StackBlock, the only Block with any derived margins, only ever derives
// Top/Bottom from its children.
//
// Its own margins are resolved from ctx.StyleSheet via node - unscaled,
// matching Block.Margins' convention (StackBlock.GetBlockLayout, the one real
// caller, scales the result via ctx.ScaledMargins).
type MarginBlock struct {
	Block
	node *ASTNode
}

func (b *MarginBlock) Margins(ctx RenderingContext) Margins {
	inner := b.Block.Margins(ctx)
	own := ctx.StyleSheet.Margins(b.node)
	return Margins{
		Top:    math.Max(inner.Top, own.Top),
		Bottom: math.Max(inner.Bottom, own.Bottom),
		Left:   own.Left,
		Right:  own.Right,
	}
}

// Node delegates to the wrapped Block rather than returning b.node: a
// loose list item's head wraps a ListItemHeadBlock in a MarginBlock
// keyed to a synthetic TagParagraph node (purely so its margins resolve
// like a paragraph's) - the wrapped content's TagListItem is the more
// correct identity for hit-testing.
func (b *MarginBlock) Node() *ASTNode {
	return b.Block.Node()
}

// ThematicBreakBlock is a horizontal rule (`---`). Unlike the other Block
// types it has no inline content to lay out - just a color; its vertical
// spacing comes from whatever MarginBlock wraps it.
type ThematicBreakBlock struct {
	WithoutMargins
	node *ASTNode
}

var _ Block = (*ThematicBreakBlock)(nil)

func (b *ThematicBreakBlock) Node() *ASTNode {
	return b.node
}

func (b *ThematicBreakBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	return &RuleBox{
		width:     width,
		thickness: int(ctx.ScaledThematicBreakThickness(b.node)),
		color:     ctx.StyleSheet.BorderColor(b.node),
		source:    b,
	}
}

// BlockquoteBlock is a quoted group of ordinary blocks (`> ...`). Unlike
// list-item indentation, which relies on the parent StackBlock's generic
// left-margin wrapping, a blockquote positions its own content and draws
// its own left-edge bar - self-contained, so nested blockquotes (each
// level's bar drawn independently) just work without the parent needing
// to know anything about it. inner is the quoted content as a single
// Block - already a StackBlock if there was more than one, resolved once
// at compile time rather than rebuilt on every GetBlockLayout call.
type BlockquoteBlock struct {
	WithoutMargins
	inner Block
	node  *ASTNode
}

var _ Block = (*BlockquoteBlock)(nil)

func (b *BlockquoteBlock) Node() *ASTNode {
	return b.node
}

func (b *BlockquoteBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	geom := ctx.ScaledBlockquoteGeometry(b.node)
	indent := int(geom.Indent)
	return &BlockquoteBox{
		width:    width,
		indent:   indent,
		barWidth: int(geom.BarWidth),
		barColor: ctx.StyleSheet.BorderColor(b.node),
		inner:    b.inner.GetBlockLayout(ctx, width-indent),
		source:   b,
	}
}

type CodeBlock struct {
	WithoutMargins
	// lines is one slice per visual line, each holding that line's spans -
	// usually a single element (one InlineText for the whole line), but
	// more than one when a Highlighter has split the line into classified
	// tokens (see highlightLines).
	lines [][]Inline
	node  *ASTNode
}

var _ Block = (*CodeBlock)(nil)

func (b *CodeBlock) Node() *ASTNode {
	return b.node
}

func (b *CodeBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	lineBoxes := make([]BlockLayout, len(b.lines))
	for i, parts := range b.lines {
		boxes := make([]InlineLayout, len(parts))
		for j, part := range parts {
			boxes[j] = part.GetInlineLayout(ctx, width)
		}
		// Glue: true - a code line's parts are already-contiguous
		// substrings of the source line (see highlightLines), any
		// whitespace between tokens already included verbatim in a
		// plain-class span, unlike TextBlock's word-split parts which
		// need LineBox's normal inter-word gap reconstructed.
		lineBoxes[i] = &LineBox{parts: boxes, Glue: true}
	}
	return &StackBox{slots: preResolvedSlots(lineBoxes), source: b}
}

type TextBlock struct {
	WithoutMargins
	parts []Inline
	node  *ASTNode
}

var _ Block = (*TextBlock)(nil)

func (b *TextBlock) Node() *ASTNode {
	return b.node
}

func (b *TextBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	lines := []BlockLayout{}
	boxes := make([]InlineLayout, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineLayout(ctx, width)
	}
	for len(boxes) > 0 {
		i, _ := splitBoxes(boxes, width)
		lines = append(lines, &LineBox{parts: boxes[:i]})
		boxes = boxes[i:]
	}
	return &StackBox{slots: preResolvedSlots(lines), source: b}
}

// ListItemHeadBlock is a list item's own paragraph text, flowed with the
// marker hanging off the first line - see GetBlockLayout. It always reports zero
// margins: a list item's indentation and item-to-item spacing belong to
// the StackBlock CompileListItem wraps it in (along with any trailing
// content, e.g. a nested list), not to the head on its own - it has no
// business claiming indentation whether or not there's a trailing part.
type ListItemHeadBlock struct {
	WithoutMargins
	marker Inline
	parts  []Inline
	node   *ASTNode
}

var _ Block = (*ListItemHeadBlock)(nil)

func (b *ListItemHeadBlock) Node() *ASTNode {
	return b.node
}

func (b *ListItemHeadBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	lines := []BlockLayout{}
	boxes := make([]InlineLayout, len(b.parts)+1)

	boxes[0] = &ListItemMarkerBox{
		Marker: b.marker.GetInlineLayout(ctx, width),
	}
	for i, part := range b.parts {
		boxes[i+1] = part.GetInlineLayout(ctx, width)
	}
	for len(boxes) > 0 {
		i, _ := splitBoxes(boxes, width)
		lines = append(lines, &LineBox{parts: boxes[:i]})
		boxes = boxes[i:]
	}
	return &StackBox{slots: preResolvedSlots(lines), source: b}
}

type cellAlignment int

const (
	alignNone cellAlignment = iota
	alignLeft
	alignRight
	alignCenter
)

type tableCell struct {
	content   *TextBlock
	alignment cellAlignment
}

// TableBlock is a GFM table. header and each row in rows hold one
// tableCell per column.
type TableBlock struct {
	WithoutMargins
	header []tableCell
	rows   [][]tableCell
	node   *ASTNode
}

var _ Block = (*TableBlock)(nil)

func (b *TableBlock) Node() *ASTNode {
	return b.node
}

// naturalWidthMeasure is an effectively-unbounded width passed to a
// cell's GetBlockLayout purely to measure its natural (unwrapped) width via the
// resulting BlockLayout's Bounds() - large enough that no realistic cell content
// would ever wrap against it.
const naturalWidthMeasure = 1 << 20

// resolveColumnWidths decides each column's final width from its natural
// (unwrapped) width, given the total width available for all columns
// combined (gaps and the frame are the caller's concern, not this
// function's). Deliberately isolated from measuring natural widths and
// from using the result, so alternative column-width policies can be
// swapped in later without touching either.
//
// Columns split into "narrow" (kept at natural width) and "wide" (shrunk,
// sharing the leftover space proportionally to natural width). The split
// point is the largest K such that the combined natural width of the K
// narrowest columns is less than what a single wide column would get if
// the remaining N-K columns split the rest equally:
//
//	sum(narrowest K) < available/(N-K+1)
//
// Checked as sum*(N-K+1) < available (exact with integers, and avoids
// assuming the condition is monotonic in K - it isn't, in general, since
// the sum grows and the divisor shrinks as K grows).
func resolveColumnWidths(natural []int, available int) []int {
	n := len(natural)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return natural[order[i]] < natural[order[j]] })

	prefixSum := make([]int, n+1)
	for i, idx := range order {
		prefixSum[i+1] = prefixSum[i] + natural[idx]
	}

	k := 0
	for candidate := 0; candidate <= n; candidate++ {
		if prefixSum[candidate]*(n-candidate+1) < available {
			k = candidate
		}
	}

	result := make([]int, n)
	for i := 0; i < k; i++ {
		result[order[i]] = natural[order[i]]
	}
	if k < n {
		wideNatural := prefixSum[n] - prefixSum[k]
		remaining := available - prefixSum[k]
		if remaining < 0 {
			remaining = 0
		}
		for i := k; i < n; i++ {
			idx := order[i]
			if wideNatural > 0 {
				result[idx] = remaining * natural[idx] / wideNatural
			}
		}
	}
	return result
}

func (b *TableBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	geom := ctx.ScaledTableGeometry(b.node)
	frameThickness := int(geom.FrameThickness)
	columnGap := int(geom.ColumnGap)
	rowGap := int(geom.RowGap)
	headerGap := int(geom.HeaderGap)
	columnRuleThickness := maxInt(1, int(geom.ColumnRuleThickness))

	numCols := len(b.header)
	rows := append([][]tableCell{b.header}, b.rows...)

	natural := make([]int, numCols)
	for _, row := range rows {
		for c, cell := range row {
			if w := cell.content.GetBlockLayout(ctx, naturalWidthMeasure).Bounds().Dx(); w > natural[c] {
				natural[c] = w
			}
		}
	}

	// A column rule sits centered in the columnGap between two columns,
	// so the whitespace it leaves on either side is only columnGap/2 -
	// half against the frame would double that rhythm at the table's own
	// edges, which reads as a lopsided margin. So edgeGap (columnGap/2)
	// pads each side against the frame instead, matching a rule's own
	// half-gap; total gap budget is edgeGap*2 + (numCols-1)*columnGap,
	// which is exactly numCols*columnGap.
	edgeGap := columnGap / 2
	available := width - 2*frameThickness - numCols*columnGap
	columnWidths := resolveColumnWidths(natural, available)

	// columnWidths is the wrap constraint given to each cell, not
	// necessarily what it actually renders at: word-wrapped text rarely
	// uses every last pixel of the width it's given, so a wrapped cell's
	// real Bounds() is often narrower than the column it wrapped to.
	// Track each column's effective width - the widest a cell in it
	// actually rendered - and use that for offsets/alignment instead of
	// the looser allocated width, so a wide-but-wrapped column doesn't
	// leave a bigger gap before the next one than it needs to.
	rawCells := make([][]BlockLayout, len(rows))
	rowHeights := make([]int, len(rows))
	effectiveWidths := make([]int, numCols)
	for r, row := range rows {
		rowBoxes := make([]BlockLayout, numCols)
		rowHeight := 0
		for c, cell := range row {
			contentBox := cell.content.GetBlockLayout(ctx, columnWidths[c])
			rowBoxes[c] = contentBox
			if h := contentBox.Bounds().Dy(); h > rowHeight {
				rowHeight = h
			}
			if w := contentBox.Bounds().Dx(); w > effectiveWidths[c] {
				effectiveWidths[c] = w
			}
		}
		rawCells[r] = rowBoxes
		rowHeights[r] = rowHeight
	}

	columnOffsets := make([]int, numCols+1)
	columnOffsets[0] = frameThickness + edgeGap
	for c := range numCols {
		gap := columnGap
		if c == numCols-1 {
			gap = edgeGap
		}
		columnOffsets[c+1] = columnOffsets[c] + effectiveWidths[c] + gap
		if c == numCols-1 {
			columnOffsets[c+1] += frameThickness
		}
	}

	cells := make([][]BlockLayout, len(rows))
	for r, row := range rows {
		rowCells := make([]BlockLayout, numCols)
		for c, cell := range row {
			contentBox := rawCells[r][c]
			contentWidth := contentBox.Bounds().Dx()
			xOffset := 0
			switch cell.alignment {
			case alignCenter:
				xOffset = (effectiveWidths[c] - contentWidth) / 2
			case alignRight:
				xOffset = effectiveWidths[c] - contentWidth
			}
			rowCells[c] = NewContainerBox(contentBox, effectiveWidths[c], rowHeights[r], xOffset, 0)
		}
		cells[r] = rowCells
	}

	// Same reasoning as columnOffsets: rowGap pads the top and bottom
	// against the frame too, not just between rows. The header->body
	// transition keeps its own smaller headerGap either way.
	rowOffsets := make([]int, len(rows)+1)
	rowOffsets[0] = frameThickness + rowGap
	for r, h := range rowHeights {
		rowOffsets[r+1] = rowOffsets[r] + h
		switch {
		case r == len(rowHeights)-1:
			rowOffsets[r+1] += rowGap + frameThickness
		case r == 0:
			rowOffsets[r+1] += headerGap
		default:
			rowOffsets[r+1] += rowGap
		}
	}

	return &TableBox{
		columnOffsets:       columnOffsets,
		rowOffsets:          rowOffsets,
		frameThickness:      frameThickness,
		columnGap:           columnGap,
		columnRuleThickness: columnRuleThickness,
		frameColor:          ctx.StyleSheet.BorderColor(b.node),
		cells:               cells,
		source:              b,
	}
}

type StackBlock struct {
	blocks []Block
}

var _ Block = (*StackBlock)(nil)

// Node always returns nil: a StackBlock aggregates other Blocks, each
// with their own identity, so it has none of its own - hit-testing that
// reaches a bare StackBlock should already have recursed into whichever
// child slot actually matched.
func (b *StackBlock) Node() *ASTNode {
	return nil
}

// Margins reports Top/Bottom as its first/last child's own margin - the
// same collapsing GetBlockLayout applies between siblings, extended to its own
// edges. Left/Right are zero: as a stack of blocks arranged vertically,
// StackBlock has no notion of a horizontal edge to derive from a child -
// only whatever wraps it (see MarginBlock) has a real Left/Right.
func (b *StackBlock) Margins(ctx RenderingContext) Margins {
	if len(b.blocks) == 0 {
		return Margins{}
	}
	return Margins{
		Top:    b.blocks[0].Margins(ctx).Top,
		Bottom: b.blocks[len(b.blocks)-1].Margins(ctx).Bottom,
	}
}

// GetBlockLayout builds the slot skeleton only - gap sizes from Margins(), which is
// cheap (no text measurement) - deferring each block's own GetBlockLayout call to
// StackBox.boxAt, on first access to that slot. A resize only needs to
// re-derive gap sizes and widths up front; the expensive part (actually
// laying out each block's content) only happens for slots something later
// asks for, e.g. those near a scroll anchor.
func (b *StackBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	slots := make([]stackSlot, 0, len(b.blocks))
	bottomMargin := 0
	for i, block := range b.blocks {
		margins := ctx.ScaledMargins(block)
		if i > 0 {
			gap := maxInt(bottomMargin, int(margins.Top))
			if gap > 0 {
				slots = append(slots, stackSlot{box: NewEmptyBox(width, gap)})
			}
		}
		leftMargin := int(margins.Left)
		rightMargin := int(margins.Right)
		slots = append(slots, stackSlot{
			block:      block,
			width:      width - leftMargin - rightMargin,
			leftMargin: leftMargin,
			wrap:       leftMargin > 0 || rightMargin > 0,
		})
		bottomMargin = int(margins.Bottom)
	}
	return &StackBox{slots: slots, ctx: ctx, width: width}
}
