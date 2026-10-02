package engine

import (
	"math"
	"sort"

	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/styling"
)

// Source is the common ground between Block and Inline: something with
// a semantic identity in the compiled ast.Node tree. Leaf boxes expose
// it so a hit-test result traces back to its origin - and since the
// concrete value is the real Block/Inline, a caller can type-assert
// further for anything beyond the node itself.
type Source interface {
	Node() *ast.Node
}

type Block interface {
	Source
	GetBlockLayout(ctx Context, width int) BlockLayout
	Margins(ctx Context) styling.Margins
}

// Marginer is anything that reports its own logical (unscaled) Margins -
// every Block satisfies it, but it's kept narrow so Context.
// ScaledMargins doesn't need the rest of the Block interface.
type Marginer interface {
	Margins(ctx Context) styling.Margins
}

// WithoutMargins satisfies Block's Margins() with a zero value, for a Block
// that has no margins of its own. Margins can still be added around it by
// wrapping it in a MarginBlock, as the compiler does for most blocks.
type WithoutMargins struct{}

func (WithoutMargins) Margins(ctx Context) styling.Margins {
	return styling.Margins{}
}

// MarginBlock adds margins to an existing Block, collapsing Top/Bottom with
// whatever it already reports. Left/Right are just MarginBlock's own value:
// nothing it wraps ever reports a nonzero Left/Right to collapse with -
// StackBlock, the only Block with any derived margins, only ever derives
// Top/Bottom from its children.
//
// Its own margins are resolved from ctx.Styles via node - unscaled,
// matching Block.Margins' convention (StackBlock.GetBlockLayout, the one real
// caller, scales the result via ctx.ScaledMargins).
type MarginBlock struct {
	Block
	MarginNode *ast.Node
}

func (b *MarginBlock) Margins(ctx Context) styling.Margins {
	inner := b.Block.Margins(ctx)
	own := ctx.Styles.Margins(b.MarginNode)
	return styling.Margins{
		Top:    math.Max(inner.Top, own.Top),
		Bottom: math.Max(inner.Bottom, own.Bottom),
		Left:   own.Left,
		Right:  own.Right,
	}
}

// Node delegates to the wrapped Block rather than returning b.node: a
// loose list item's head wraps a ListItemHeadBlock in a MarginBlock
// keyed to a synthetic ast.TagParagraph node (purely so its margins resolve
// like a paragraph's) - the wrapped content's ast.TagListItem is the more
// correct identity for hit-testing.
func (b *MarginBlock) Node() *ast.Node {
	return b.Block.Node()
}

// ThematicBreakBlock is a horizontal rule (`---`). Unlike the other Block
// types it has no inline content to lay out - just a color; its vertical
// spacing comes from whatever MarginBlock wraps it.
type ThematicBreakBlock struct {
	WithoutMargins
	ASTNode *ast.Node
}

var _ Block = (*ThematicBreakBlock)(nil)

func (b *ThematicBreakBlock) Node() *ast.Node {
	return b.ASTNode
}

func (b *ThematicBreakBlock) GetBlockLayout(ctx Context, width int) BlockLayout {
	return &RuleBox{
		width:     width,
		thickness: int(ctx.ScaledThematicBreakThickness(b.ASTNode)),
		color:     ctx.Styles.BorderColor(b.ASTNode),
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
	Inner   Block
	ASTNode *ast.Node
}

var _ Block = (*BlockquoteBlock)(nil)

func (b *BlockquoteBlock) Node() *ast.Node {
	return b.ASTNode
}

func (b *BlockquoteBlock) GetBlockLayout(ctx Context, width int) BlockLayout {
	geom := ctx.ScaledBlockquoteGeometry(b.ASTNode)
	indent := int(geom.Indent)
	return &BlockquoteBox{
		width:    width,
		Indent:   indent,
		barWidth: int(geom.BarWidth),
		barColor: ctx.Styles.BorderColor(b.ASTNode),
		Inner:    b.Inner.GetBlockLayout(ctx, width-indent),
		source:   b,
	}
}

type CodeBlock struct {
	WithoutMargins
	// Lines is one slice per visual line, each holding that line's spans -
	// usually a single element (one InlineText for the whole line), but
	// more than one when a Highlighter has split the line into classified
	// tokens (see highlightLines).
	Lines   [][]Inline
	ASTNode *ast.Node
}

var _ Block = (*CodeBlock)(nil)

func (b *CodeBlock) Node() *ast.Node {
	return b.ASTNode
}

func (b *CodeBlock) GetBlockLayout(ctx Context, width int) BlockLayout {
	lineBoxes := make([]BlockLayout, len(b.Lines))
	for i, parts := range b.Lines {
		lineBoxes[i] = NewLineBox(inlineLayouts(ctx, width, parts), true)
	}
	// Lines never wrap, so a long one scrolls sideways instead.
	return scrollIfWider(ctx, b, &StackBox{Slots: preResolvedSlots(lineBoxes), source: b}, width)
}

type TextBlock struct {
	WithoutMargins
	Parts   []Inline
	ASTNode *ast.Node
}

var _ Block = (*TextBlock)(nil)

func (b *TextBlock) Node() *ast.Node {
	return b.ASTNode
}

func (b *TextBlock) GetBlockLayout(ctx Context, width int) BlockLayout {
	return &StackBox{Slots: preResolvedSlots(wrapLines(inlineLayouts(ctx, width, b.Parts), width)), source: b}
}

// inlineLayouts lays out each of parts at wrap width width.
func inlineLayouts(ctx Context, width int, parts []Inline) []InlineLayout {
	boxes := make([]InlineLayout, len(parts))
	for i, part := range parts {
		boxes[i] = part.GetInlineLayout(ctx, width)
	}
	return boxes
}

// ListItemHeadBlock is a list item's own paragraph text, flowed with the
// marker hanging off the first line - see GetBlockLayout. It always reports zero
// margins: a list item's indentation and item-to-item spacing belong to
// the StackBlock compileListItem wraps it in (along with any trailing
// content, e.g. a nested list), not to the head on its own - it has no
// business claiming indentation whether or not there's a trailing part.
type ListItemHeadBlock struct {
	WithoutMargins
	Marker  Inline
	Parts   []Inline
	ASTNode *ast.Node
}

var _ Block = (*ListItemHeadBlock)(nil)

func (b *ListItemHeadBlock) Node() *ast.Node {
	return b.ASTNode
}

func (b *ListItemHeadBlock) GetBlockLayout(ctx Context, width int) BlockLayout {
	marker := &ListItemMarkerBox{Marker: b.Marker.GetInlineLayout(ctx, width)}
	boxes := append([]InlineLayout{marker}, inlineLayouts(ctx, width, b.Parts)...)
	return &StackBox{Slots: preResolvedSlots(wrapLines(boxes, width)), source: b}
}

type CellAlignment int

const (
	AlignNone CellAlignment = iota
	AlignLeft
	AlignRight
	AlignCenter
)

type TableCell struct {
	Content   *TextBlock
	Alignment CellAlignment
}

// TableBlock is a GFM table. header and each row in rows hold one
// tableCell per column.
type TableBlock struct {
	WithoutMargins
	Header  []TableCell
	Rows    [][]TableCell
	ASTNode *ast.Node
}

var _ Block = (*TableBlock)(nil)

func (b *TableBlock) Node() *ast.Node {
	return b.ASTNode
}

// NaturalWidthMeasure is an effectively-unbounded width passed to a
// cell's GetBlockLayout purely to measure its natural (unwrapped) width via the
// resulting BlockLayout's Bounds() - large enough that no realistic cell content
// would ever wrap against it.
const NaturalWidthMeasure = 1 << 20

// ResolveColumnWidths decides each column's final width from its natural
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
func ResolveColumnWidths(natural []int, available int) []int {
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

func (b *TableBlock) GetBlockLayout(ctx Context, width int) BlockLayout {
	geom := ctx.ScaledTableGeometry(b.ASTNode)
	frameThickness := int(geom.FrameThickness)
	columnGap := int(geom.ColumnGap)
	rowGap := int(geom.RowGap)
	headerGap := int(geom.HeaderGap)
	columnRuleThickness := max(1, int(geom.ColumnRuleThickness))

	numCols := len(b.Header)
	rows := append([][]TableCell{b.Header}, b.Rows...)

	natural := make([]int, numCols)
	for _, row := range rows {
		for c, cell := range row {
			if w := cell.Content.GetBlockLayout(ctx, NaturalWidthMeasure).Bounds().Dx(); w > natural[c] {
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
	columnWidths := ResolveColumnWidths(natural, available)

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
			contentBox := cell.Content.GetBlockLayout(ctx, columnWidths[c])
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
			switch cell.Alignment {
			case AlignCenter:
				xOffset = (effectiveWidths[c] - contentWidth) / 2
			case AlignRight:
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

	// Columns shrink to fit width, but a cell holding a word too long to
	// wrap can still make the table wider: it scrolls sideways then.
	return scrollIfWider(ctx, b, &TableBox{
		columnOffsets:       columnOffsets,
		rowOffsets:          rowOffsets,
		frameThickness:      frameThickness,
		columnGap:           columnGap,
		columnRuleThickness: columnRuleThickness,
		frameColor:          ctx.Styles.BorderColor(b.ASTNode),
		cells:               cells,
		source:              b,
	}, width)
}

type StackBlock struct {
	Blocks []Block
}

var _ Block = (*StackBlock)(nil)

// Node always returns nil: a StackBlock aggregates other Blocks, each
// with their own identity, so it has none of its own - hit-testing that
// reaches a bare StackBlock should already have recursed into whichever
// child slot actually matched.
func (b *StackBlock) Node() *ast.Node {
	return nil
}

// Margins reports Top/Bottom as its first/last child's own margin - the
// same collapsing GetBlockLayout applies between siblings, extended to its own
// edges. Left/Right are zero: as a stack of blocks arranged vertically,
// StackBlock has no notion of a horizontal edge to derive from a child -
// only whatever wraps it (see MarginBlock) has a real Left/Right.
func (b *StackBlock) Margins(ctx Context) styling.Margins {
	if len(b.Blocks) == 0 {
		return styling.Margins{}
	}
	return styling.Margins{
		Top:    b.Blocks[0].Margins(ctx).Top,
		Bottom: b.Blocks[len(b.Blocks)-1].Margins(ctx).Bottom,
	}
}

// GetBlockLayout builds the slot skeleton only - gap sizes from Margins(), which is
// cheap (no text measurement) - deferring each block's own GetBlockLayout call to
// StackBox.boxAt, on first access to that slot. A resize only needs to
// re-derive gap sizes and widths up front; the expensive part (actually
// laying out each block's content) only happens for slots something later
// asks for, e.g. those near a scroll anchor.
func (b *StackBlock) GetBlockLayout(ctx Context, width int) BlockLayout {
	return b.StackLayout(&ctx, width)
}

// StackLayout is GetBlockLayout with its concrete result type, laying out
// slots against *ctx when they're first needed (see StackBox.ctx).
func (b *StackBlock) StackLayout(ctx *Context, width int) *StackBox {
	slots := make([]StackSlot, 0, len(b.Blocks))
	bottomMargin := 0
	for i, block := range b.Blocks {
		margins := ctx.ScaledMargins(block)
		if i > 0 {
			gap := max(bottomMargin, int(margins.Top))
			if gap > 0 {
				slots = append(slots, StackSlot{Box: NewEmptyBox(width, gap)})
			}
		}
		leftMargin := int(margins.Left)
		rightMargin := int(margins.Right)
		slots = append(slots, StackSlot{
			Block:      block,
			Width:      width - leftMargin - rightMargin,
			leftMargin: leftMargin,
			wrap:       leftMargin > 0 || rightMargin > 0,
		})
		bottomMargin = int(margins.Bottom)
	}
	return &StackBox{Slots: slots, Ctx: ctx, Width: width}
}
