package main

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
)

// Box is drawn via the package-level DrawBox, not by calling drawContents
// directly, so that every Box gets the same off-screen skip for free
// regardless of where it sits in the tree. drawContents holds only the
// type-specific drawing logic.
type Box interface {
	Bounds() image.Rectangle
	drawContents(dst *ebiten.Image, x, y int)
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

	boundsComputed bool
	bounds         image.Rectangle
	advance        int

	spaceComputed bool
	spaceWidth    int
}

var _ InlineBox = (*TextBox)(nil)

// Text/Face never change after construction, and a TextBox is always
// rebuilt from scratch (never mutated) whenever the source Block tree is
// re-laid out, so this measurement is valid for the entire lifetime of
// the instance: compute it once, on first use.
func (b *TextBox) BoundsAndAdvance() (image.Rectangle, int) {
	if !b.boundsComputed {
		bounds, advance := font.BoundString(b.Face, b.Text)
		metrics := b.Face.Metrics()
		b.bounds = image.Rect(
			bounds.Min.X.Floor(),
			-metrics.Ascent.Ceil(),
			bounds.Max.X.Ceil(),
			metrics.Descent.Ceil(),
		)
		b.advance = advance.Ceil()
		b.boundsComputed = true
	}
	return b.bounds, b.advance
}

func (b *TextBox) SpaceWidth() int {
	if !b.spaceComputed {
		adv, _ := b.Face.GlyphAdvance(' ')
		b.spaceWidth = adv.Ceil()
		b.spaceComputed = true
	}
	return b.spaceWidth
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

	boundsComputed bool
	bounds         image.Rectangle
	advance        int
}

var _ Box = (*LineBox)(nil)

// Same reasoning as TextBox: parts/space are fixed at construction and a
// LineBox is never reused across a re-layout, so this is safe to compute
// once and reuse for the instance's whole life.
func (b *LineBox) BoundsAndAdvance() (image.Rectangle, int) {
	if !b.boundsComputed {
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
		b.bounds = bounds
		b.advance = advance
		b.boundsComputed = true
	}
	return b.bounds, b.advance
}

func (b *LineBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	return bounds.Sub(bounds.Min)
}

type StackBox struct {
	boxes []Box

	boundsComputed bool
	bounds         image.Rectangle
}

func (b *StackBox) Bounds() image.Rectangle {
	if !b.boundsComputed {
		var bounds image.Rectangle
		for _, box := range b.boxes {
			bounds = bounds.Union(box.Bounds().Add(image.Pt(0, bounds.Max.Y)))
		}
		b.bounds = bounds
		b.boundsComputed = true
	}
	return b.bounds
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
