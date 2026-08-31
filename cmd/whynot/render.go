package main

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

func (b *TextBox) DrawInline(dst *ebiten.Image, x, y int) int {
	bounds, advance := b.BoundsAndAdvance()
	// drawRect(dst, bounds.Add(image.Pt(x, y)), color.Gray{Y: 128})
	_ = bounds
	text.Draw(dst, b.Text, b.Face, x, y, b.Color)
	return x + advance
}

func (b *ListItemMarkerBox) DrawInline(dst *ebiten.Image, x, y int) int {
	_, advance := b.Marker.BoundsAndAdvance()
	space := b.Marker.SpaceWidth()
	b.Marker.DrawInline(dst, x-advance-space, y)
	return x - space
}

func (b *ImageBox) DrawInline(dst *ebiten.Image, x, y int) int {
	geoM := ebiten.GeoM{}
	geoM.Translate(float64(x), float64(y))
	dst.DrawImage(b.image, &ebiten.DrawImageOptions{GeoM: geoM})
	return b.image.Bounds().Dx()
}

// DrawBox is the sole entry point for drawing a Box: it skips drawContents
// entirely when box's bounds don't overlap dst, so every Box gets that for
// free regardless of who's calling it or where it sits in the tree.
func DrawBox(box Box, dst *ebiten.Image, x, y int) {
	if !box.Bounds().Add(image.Pt(x, y)).Overlaps(dst.Bounds()) {
		return
	}
	box.drawContents(dst, x, y)
}

func (b *LineBox) drawContents(dst *ebiten.Image, x, y int) {
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

func (b *StackBox) drawContents(dst *ebiten.Image, x, y int) {
	for _, box := range b.boxes {
		DrawBox(box, dst, x, y)
		y += box.Bounds().Max.Y
	}
}

func (b *EmptyBox) drawContents(dst *ebiten.Image, x, y int) {
}

func (b *ContainerBox) drawContents(dst *ebiten.Image, x, y int) {
	DrawBox(b.inner, dst, x+b.innerPos.X, y+b.innerPos.Y)
}

func drawRect(dst *ebiten.Image, rect image.Rectangle, clr color.Color) {
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Min.X), float64(rect.Max.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Min.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Max.Y), float64(rect.Max.X), float64(rect.Max.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Max.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Max.Y), clr)
}
