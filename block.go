package whynot

import (
	"math"
)

type Margins struct {
	Top, Bottom, Left, Right float64
}

type Block interface {
	GetBox(ctx RenderingContext, width int) Box
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
// matching Block.Margins' convention (StackBlock.GetBox, the one real
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

type Inline interface {
	GetInlineBox(RenderingContext) InlineBox
}

type InlineText struct {
	text string
	node *ASTNode
}

var _ Inline = (*InlineText)(nil)

type InlineImage struct {
	src   string
	title string
	node  *ASTNode
}

var _ Inline = (*InlineImage)(nil)

// ThematicBreakBlock is a horizontal rule (`---`). Unlike the other Block
// types it has no inline content to lay out - just a color; its vertical
// spacing comes from whatever MarginBlock wraps it.
type ThematicBreakBlock struct {
	WithoutMargins
	node *ASTNode
}

var _ Block = (*ThematicBreakBlock)(nil)

// BlockquoteBlock is a quoted group of ordinary blocks (`> ...`). Unlike
// list-item indentation, which relies on the parent StackBlock's generic
// left-margin wrapping, a blockquote positions its own content and draws
// its own left-edge bar - self-contained, so nested blockquotes (each
// level's bar drawn independently) just work without the parent needing
// to know anything about it. inner is the quoted content as a single
// Block - already a StackBlock if there was more than one, resolved once
// at compile time rather than rebuilt on every GetBox call.
type BlockquoteBlock struct {
	WithoutMargins
	inner Block
	node  *ASTNode
}

var _ Block = (*BlockquoteBlock)(nil)

type CodeBlock struct {
	WithoutMargins
	lines []Inline
}

var _ Block = (*CodeBlock)(nil)

type TextBlock struct {
	WithoutMargins
	parts []Inline
}

var _ Block = (*TextBlock)(nil)

// ListItemHeadBlock is a list item's own paragraph text, flowed with the
// marker hanging off the first line - see GetBox. It always reports zero
// margins: a list item's indentation and item-to-item spacing belong to
// the StackBlock CompileListItem wraps it in (along with any trailing
// content, e.g. a nested list), not to the head on its own - it has no
// business claiming indentation whether or not there's a trailing part.
type ListItemHeadBlock struct {
	WithoutMargins
	marker Inline
	parts  []Inline
}

var _ Block = (*ListItemHeadBlock)(nil)

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

type StackBlock struct {
	blocks []Block
}

var _ Block = (*StackBlock)(nil)

// Margins reports Top/Bottom as its first/last child's own margin - the
// same collapsing GetBox applies between siblings, extended to its own
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
