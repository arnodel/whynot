package main

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
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

type ListItemMarkerBox struct {
	Marker InlineBox
}

var _ InlineBox = (*ListItemMarkerBox)(nil)

func (b *ListItemMarkerBox) BoundsAndAdvance() (image.Rectangle, int) {
	bounds, _ := b.Marker.BoundsAndAdvance()
	return image.Rect(0, bounds.Min.Y, 0, bounds.Max.Y), 0
}

func (b *ListItemMarkerBox) SpaceWidth() int {
	return b.Marker.SpaceWidth()
}

type ImageBox struct {
	image *ebiten.Image
}

var _ InlineBox = (*ImageBox)(nil)

func (b *ImageBox) BoundsAndAdvance() (image.Rectangle, int) {
	bounds := b.image.Bounds()
	return bounds, bounds.Dx()
}

func (b *ImageBox) SpaceWidth() int {
	return 0
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
