package main

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

type RenderingContext struct {
	Scale float64
	FaceSelector
}

func (c RenderingContext) ScaleMargins(m Margins) Margins {
	m.Left *= c.Scale
	m.Right *= c.Scale
	m.Top *= c.Scale
	m.Bottom *= c.Scale
	return m
}

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

func (t *InlineText) GetInlineBox(ctx RenderingContext) InlineBox {
	face, err := ctx.SelectFace(t.style)
	if err != nil {
		panic(err)
	}
	return &TextBox{
		Text:  t.text,
		Face:  face,
		Color: t.color,
	}
}

type InlineImage struct {
	image *ebiten.Image
	title string
	src   string
}

var _ Inline = (*InlineImage)(nil)

func (i *InlineImage) GetInlineBox(ctx RenderingContext) InlineBox {
	return &ImageBox{
		image: i.image,
	}
}

type CodeBlock struct {
	margins Margins
	lines   []Inline
	space   int
}

var _ Block = (*CodeBlock)(nil)

func (b *CodeBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	height := 0
	for _, line := range b.lines {
		box := line.GetInlineBox(ctx)
		bounds, _ := box.BoundsAndAdvance()
		height += bounds.Dy()
	}
	return image.Rect(0, 0, width, height)
}

func (b *CodeBlock) GetBox(ctx RenderingContext, width int) Box {
	lineBoxes := make([]Box, len(b.lines))
	for i, line := range b.lines {
		lineBoxes[i] = &LineBox{[]InlineBox{line.GetInlineBox(ctx)}, b.space}
	}
	return &StackBox{boxes: lineBoxes}
}

func (b *CodeBlock) Margins() Margins {
	return b.margins
}

type TextBlock struct {
	margins Margins
	parts   []Inline
	space   int
}

var _ Block = (*TextBlock)(nil)

func (b *TextBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	height := 0
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(ctx)
	}
	for len(boxes) > 0 {
		i, lineBounds := splitBoxes(boxes, width)
		height += lineBounds.Dy()
		boxes = boxes[i:]
	}
	return image.Rect(0, 0, width, height)
}

func (b *TextBlock) GetBox(ctx RenderingContext, width int) Box {
	lines := []Box{}
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(ctx)
	}
	for len(boxes) > 0 {
		i, _ := splitBoxes(boxes, width)
		lines = append(lines, &LineBox{boxes[:i], b.space})
		boxes = boxes[i:]
	}
	return &StackBox{boxes: lines}
}

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

func (b *ListItemBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	height := 0
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(ctx)
	}
	for len(boxes) > 0 {
		i, lineBounds := splitBoxes(boxes, width)
		height += lineBounds.Dy()
		boxes = boxes[i:]
	}
	return image.Rect(0, 0, width, height)
}

func (b *ListItemBlock) GetBox(ctx RenderingContext, width int) Box {
	lines := []Box{}
	boxes := make([]InlineBox, len(b.parts)+1)

	boxes[0] = &ListItemMarkerBox{
		Marker: b.marker.GetInlineBox(ctx),
	}
	for i, part := range b.parts {
		boxes[i+1] = part.GetInlineBox(ctx)
	}
	for len(boxes) > 0 {
		i, _ := splitBoxes(boxes, width)
		lines = append(lines, &LineBox{boxes[:i], b.space})
		boxes = boxes[i:]
	}
	return &StackBox{boxes: lines}
}

func (b *ListItemBlock) Margins() Margins {
	return b.margins
}

type StackBlock struct {
	blocks  []Block
	margins Margins
}

var _ Block = (*StackBlock)(nil)

func (b *StackBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	return image.Rectangle{}
}

func (b *StackBlock) GetBox(ctx RenderingContext, width int) Box {
	boxes := make([]Box, 0, len(b.blocks))
	bottomMargin := 0
	for i, block := range b.blocks {
		margins := ctx.ScaleMargins(block.Margins())
		if i > 0 {
			gap := maxInt(bottomMargin, int(margins.Top))
			if gap > 0 {
				boxes = append(boxes, NewEmptyBox(width, gap))
			}
		}
		if margins.Left > 0 || margins.Right > 0 {
			box := block.GetBox(ctx, width-int(margins.Left+margins.Right))
			boxes = append(boxes, NewContainerBox(box, width, box.Bounds().Dy(), int(margins.Left), 0))
		} else {
			boxes = append(boxes, block.GetBox(ctx, width))
		}
		bottomMargin = int(margins.Bottom)
	}
	return &StackBox{boxes: boxes}
}

func (b *StackBlock) Margins() Margins {
	return Margins{
		Top:    math.Max(b.blocks[0].Margins().Top, b.margins.Top),
		Bottom: math.Max(b.blocks[len(b.blocks)-1].Margins().Bottom, b.margins.Bottom),
	}
}
