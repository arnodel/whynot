package whynot

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

type Margins struct {
	Top, Bottom, Left, Right float64
}

type Block interface {
	GetBounds(ctx RenderingContext, width int) image.Rectangle
	GetBox(ctx RenderingContext, width int) Box
	Margins() Margins
}

type Inline interface {
	GetInlineBox(RenderingContext) InlineBox
}

type InlineText struct {
	text  string
	style TextStyle
	color color.Color
}

var _ Inline = (*InlineText)(nil)

type InlineImage struct {
	image *ebiten.Image
	title string
	src   string
}

var _ Inline = (*InlineImage)(nil)

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

type ListItemBlock struct {
	marker  Inline
	margins Margins
	parts   []Inline
	space   int
}

var _ Block = (*ListItemBlock)(nil)

func (b *ListItemBlock) Margins() Margins {
	return b.margins
}

type StackBlock struct {
	blocks  []Block
	margins Margins
}

var _ Block = (*StackBlock)(nil)

func (b *StackBlock) Margins() Margins {
	return Margins{
		Top:    math.Max(b.blocks[0].Margins().Top, b.margins.Top),
		Bottom: math.Max(b.blocks[len(b.blocks)-1].Margins().Bottom, b.margins.Bottom),
	}
}
