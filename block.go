package whynot

import (
	"image/color"
	"math"
)

type Margins struct {
	Top, Bottom, Left, Right float64
}

type Block interface {
	GetBox(ctx RenderingContext, width int) Box
	Margins() Margins
}

type Inline interface {
	GetInlineBox(RenderingContext) InlineBox
}

type InlineText struct {
	text   string
	style  TextStyle
	color  color.Color
	strike bool
}

var _ Inline = (*InlineText)(nil)

type InlineImage struct {
	src   string
	title string
}

var _ Inline = (*InlineImage)(nil)

// ThematicBreakBlock is a horizontal rule (`---`). Unlike the other Block
// types it has no inline content to lay out - just a color and the margins
// that give it its vertical spacing.
type ThematicBreakBlock struct {
	margins Margins
	color   color.Color
}

var _ Block = (*ThematicBreakBlock)(nil)

func (b *ThematicBreakBlock) Margins() Margins {
	return b.margins
}

// BlockquoteBlock is a quoted group of ordinary blocks (`> ...`). Unlike
// list-item indentation, which relies on the parent StackBlock's generic
// left-margin wrapping, a blockquote positions its own content and draws
// its own left-edge bar - self-contained, so nested blockquotes (each
// level's bar drawn independently) just work without the parent needing
// to know anything about it. inner is the quoted content as a single
// Block - already a StackBlock if there was more than one, resolved once
// at compile time rather than rebuilt on every GetBox call.
type BlockquoteBlock struct {
	inner    Block
	margins  Margins
	barColor color.Color
}

var _ Block = (*BlockquoteBlock)(nil)

func (b *BlockquoteBlock) Margins() Margins {
	return b.margins
}

type CodeBlock struct {
	margins Margins
	lines   []Inline
	space   int
}

var _ Block = (*CodeBlock)(nil)

func (b *CodeBlock) Margins() Margins {
	return b.margins
}

type TextBlock struct {
	margins Margins
	parts   []Inline
	space   int
}

var _ Block = (*TextBlock)(nil)

func (b *TextBlock) Margins() Margins {
	return b.margins
}

// ListItemHeadBlock is a list item's own paragraph text, flowed with the
// marker hanging off the first line - see GetBox. It always reports zero
// margins: a list item's indentation and item-to-item spacing belong to
// the StackBlock CompileListItem wraps it in (along with any trailing
// content, e.g. a nested list), not to the head on its own - it has no
// business claiming indentation whether or not there's a trailing part.
type ListItemHeadBlock struct {
	marker Inline
	parts  []Inline
	space  int
}

var _ Block = (*ListItemHeadBlock)(nil)

func (b *ListItemHeadBlock) Margins() Margins {
	return Margins{}
}

type StackBlock struct {
	blocks  []Block
	margins Margins
}

var _ Block = (*StackBlock)(nil)

// Margins combines Top/Bottom with its first/last child's, the same
// adjacent-margin collapsing StackBlock.GetBox applies between any two
// blocks. Left/Right aren't a collapsing concept the way Top/Bottom are -
// they're just "how far do I sit from my container's edge," a property
// of this block alone - so they're b.margins' own value, not derived
// from children.
func (b *StackBlock) Margins() Margins {
	return Margins{
		Top:    math.Max(b.blocks[0].Margins().Top, b.margins.Top),
		Bottom: math.Max(b.blocks[len(b.blocks)-1].Margins().Bottom, b.margins.Bottom),
		Left:   b.margins.Left,
		Right:  b.margins.Right,
	}
}
