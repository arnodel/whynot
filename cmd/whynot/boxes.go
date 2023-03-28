package main

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
)

type Box interface {
	Bounds() image.Rectangle
	Draw(dst *ebiten.Image, x, y int)
}

type InlineBox interface {
	BoundsAndAdvance() (image.Rectangle, int)
	SpaceWidth() int
	DrawInline(dst *ebiten.Image, x, y int) int
}

type TextBox struct {
	Text  string
	Face  font.Face
	Color color.Color
}

var _ InlineBox = (*TextBox)(nil)

func (b *TextBox) BoundsAndAdvance() (image.Rectangle, int) {
	bounds, advance := font.BoundString(b.Face, b.Text)
	metrics := b.Face.Metrics()
	return image.Rect(
		bounds.Min.X.Floor(),
		-metrics.Ascent.Ceil(),
		bounds.Max.X.Ceil(),
		metrics.Descent.Ceil(),
	), advance.Ceil()
}

func (b *TextBox) SpaceWidth() int {
	adv, _ := b.Face.GlyphAdvance(' ')
	return adv.Ceil()
}

func (b *TextBox) DrawInline(dst *ebiten.Image, x, y int) int {
	bounds, advance := b.BoundsAndAdvance()
	// drawRect(dst, bounds.Add(image.Pt(x, y)), color.Gray{Y: 128})
	_ = bounds
	text.Draw(dst, b.Text, b.Face, x, y, b.Color)
	return x + advance
}

type LineBox struct {
	parts []InlineBox
	space int
}

var _ Box = (*LineBox)(nil)

func (b *LineBox) BoundsAndAdvance() (image.Rectangle, int) {
	bounds, advance := b.parts[0].BoundsAndAdvance()
	left := bounds.Min.X
	if left < 0 {
		bounds = bounds.Add(image.Pt(-left, 0))
		advance -= left
	}
	for _, box := range b.parts[1:] {
		advance += b.space
		boxBounds, boxAdvance := box.BoundsAndAdvance()
		bounds = bounds.Union(boxBounds.Add(image.Pt(advance, 0)))
		advance += boxAdvance
	}
	return bounds, advance
}

func (b *LineBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	return bounds.Sub(bounds.Min)
}

func (b *LineBox) Draw(dst *ebiten.Image, x, y int) {
	lineBounds, _ := b.BoundsAndAdvance()
	y -= lineBounds.Min.Y

	bounds, _ := b.parts[0].BoundsAndAdvance()
	left := bounds.Min.X
	if left < 0 {
		x -= left
	}
	prevSpace := b.parts[0].SpaceWidth()

	x = b.parts[0].DrawInline(dst, x, y)
	for _, box := range b.parts[1:] {
		space := box.SpaceWidth()
		x = box.DrawInline(dst, x+maxInt(prevSpace, space), y)
		prevSpace = space
	}
}

type StackBox struct {
	boxes []Box
}

func (b *StackBox) Bounds() image.Rectangle {
	var bounds image.Rectangle
	for _, box := range b.boxes {
		bounds = bounds.Union(box.Bounds().Add(image.Pt(0, bounds.Max.Y)))
	}
	return bounds
}

func (b *StackBox) Draw(dst *ebiten.Image, x, y int) {
	for _, box := range b.boxes {
		box.Draw(dst, x, y)
		y += box.Bounds().Max.Y
	}
}

func splitBoxes(boxes []InlineBox, width int) (int, image.Rectangle) {
	if len(boxes) == 0 {
		return 0, image.Rectangle{}
	}
	bounds, advance := boxes[0].BoundsAndAdvance()
	left := bounds.Min.X
	if left < 0 {
		bounds = bounds.Add(image.Pt(-left, 0))
		advance -= left
	}
	prevSpace := boxes[0].SpaceWidth()
	for i, box := range boxes[1:] {
		boxBounds, boxAdvance := box.BoundsAndAdvance()

		space := box.SpaceWidth()
		advance += maxInt(space, prevSpace)
		prevSpace = space

		movedBoxBounds := boxBounds.Add(image.Pt(advance, 0))
		bounds = bounds.Union(movedBoxBounds)
		if bounds.Max.X > width {
			return i + 1, bounds
		}
		advance += boxAdvance
	}
	return len(boxes), bounds
}

type EmptyBox struct {
	bounds image.Rectangle
}

func NewEmptyBox(w, h int) *EmptyBox {
	return &EmptyBox{
		bounds: image.Rect(0, 0, w, h),
	}
}

func (b *EmptyBox) Bounds() image.Rectangle {
	return b.bounds
}

func (b *EmptyBox) Draw(dst *ebiten.Image, x, y int) {
}

type ContainerBox struct {
	bounds   image.Rectangle
	innerPos image.Point
	inner    Box
}

func NewContainerBox(inner Box, w, h, x, y int) *ContainerBox {
	return &ContainerBox{
		bounds:   image.Rect(0, 0, w, h),
		innerPos: image.Pt(x, y),
		inner:    inner,
	}
}

func (b *ContainerBox) Bounds() image.Rectangle {
	return b.bounds
}

func (b *ContainerBox) Draw(dst *ebiten.Image, x, y int) {
	b.inner.Draw(dst, x+b.innerPos.X, y+b.innerPos.Y)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func drawRect(dst *ebiten.Image, rect image.Rectangle, clr color.Color) {
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Min.X), float64(rect.Max.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Min.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Max.Y), float64(rect.Max.X), float64(rect.Max.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Max.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Max.Y), clr)
}
