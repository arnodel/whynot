package main

import (
	"image"
	"image/color"
)

type Margins struct {
	Top, Bottom, Left, Right int
}

type Block interface {
	GetBounds(faceSelector FaceSelector, width int) image.Rectangle
	GetBox(faceSelector FaceSelector, width int) Box
	Margins() Margins
}

type Inline interface {
	GetInlineBox(FaceSelector) InlineBox
}

type InlineText struct {
	text  string
	style TextStyle
	color color.Color
}

var _ Inline = (*InlineText)(nil)

func (t *InlineText) GetInlineBox(faceSelector FaceSelector) InlineBox {
	face, err := faceSelector.SelectFace(t.style)
	if err != nil {
		panic(err)
	}
	return &TextBox{
		Text:  t.text,
		Face:  face,
		Color: t.color,
	}
}

type TextBlock struct {
	margins Margins
	parts   []Inline
	space   int
}

var _ Block = (*TextBlock)(nil)

func (b *TextBlock) GetBounds(faceSelector FaceSelector, width int) image.Rectangle {
	height := 0
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(faceSelector)
	}
	for len(boxes) > 0 {
		i, lineBounds := splitBoxes(boxes, width)
		height += lineBounds.Dy()
		boxes = boxes[i:]
	}
	return image.Rect(0, 0, width, height)
}

func (b *TextBlock) GetBox(faceSelector FaceSelector, width int) Box {
	lines := []Box{}
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(faceSelector)
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
	indent  int
	marker  Inline
	margins Margins
	parts   []Inline
	space   int
}

var _ Block = (*ListItemBlock)(nil)

func (b *ListItemBlock) GetBounds(faceSelector FaceSelector, width int) image.Rectangle {
	height := 0
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(faceSelector)
	}
	for len(boxes) > 0 {
		i, lineBounds := splitBoxes(boxes, width)
		height += lineBounds.Dy()
		boxes = boxes[i:]
	}
	return image.Rect(0, 0, width, height)
}

func (b *ListItemBlock) GetBox(faceSelector FaceSelector, width int) Box {
	lines := []Box{}
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(faceSelector)
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

func (b *StackBlock) GetBounds(faceSelecctor FaceSelector, width int) image.Rectangle {
	return image.Rectangle{}
}

func (b *StackBlock) GetBox(faceSelector FaceSelector, width int) Box {
	boxes := make([]Box, 0, len(b.blocks))
	bottomMargin := 0
	for i, block := range b.blocks {
		margins := block.Margins()
		if i > 0 {
			gap := maxInt(bottomMargin, margins.Top)
			if gap > 0 {
				boxes = append(boxes, NewEmptyBox(width, gap))
			}
		}
		if margins.Left > 0 || margins.Right > 0 {
			box := block.GetBox(faceSelector, width-margins.Left-margins.Right)
			boxes = append(boxes, NewContainerBox(box, width, box.Bounds().Dy(), margins.Left, 0))
		} else {
			boxes = append(boxes, block.GetBox(faceSelector, width))
		}
		bottomMargin = margins.Bottom
	}
	return &StackBox{boxes: boxes}
}

func (b *StackBlock) Margins() Margins {
	return Margins{
		Top:    maxInt(b.blocks[0].Margins().Top, b.margins.Top),
		Bottom: maxInt(b.blocks[len(b.blocks)-1].Margins().Bottom, b.margins.Bottom),
	}
}
