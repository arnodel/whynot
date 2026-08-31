package whynot

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
)

func (b *TextBox) DrawInline(dst Canvas, x, y int) int {
	_, advance := b.BoundsAndAdvance()
	dst.DrawText(b.Text, b.Face, x, y, b.Color)
	return x + advance
}

func (b *ListItemMarkerBox) DrawInline(dst Canvas, x, y int) int {
	_, advance := b.Marker.BoundsAndAdvance()
	space := b.Marker.SpaceWidth()
	b.Marker.DrawInline(dst, x-advance-space, y)
	return x - space
}

func (b *ImageBox) DrawInline(dst Canvas, x, y int) int {
	dst.DrawImage(b.src, x, y)
	return x + b.bounds.Dx()
}

// DrawBox is the sole entry point for drawing a Box: it skips drawContents
// entirely when box's bounds don't overlap dst, so every Box gets that for
// free regardless of who's calling it or where it sits in the tree.
func DrawBox(box Box, dst Canvas, x, y int) {
	if !box.Bounds().Add(image.Pt(x, y)).Overlaps(dst.Bounds()) {
		return
	}
	box.drawContents(dst, x, y)
}

func (b *LineBox) drawContents(dst Canvas, x, y int) {
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

func (b *StackBox) drawContents(dst Canvas, x, y int) {
	viewport := dst.Bounds()
	for _, box := range b.boxes {
		childBounds := box.Bounds()
		if childBounds.Add(image.Pt(x, y)).Min.Y > viewport.Max.Y {
			// This child, and every one after it, starts below the
			// viewport: nothing further down can be visible.
			break
		}
		DrawBox(box, dst, x, y)
		y += childBounds.Max.Y
	}
}

func (b *EmptyBox) drawContents(dst Canvas, x, y int) {
}

func (b *ContainerBox) drawContents(dst Canvas, x, y int) {
	DrawBox(b.inner, dst, x+b.innerPos.X, y+b.innerPos.Y)
}

// EbitenCanvas implements Canvas by drawing onto an *ebiten.Image. It's the
// only ebiten-specific piece left in the package - a placeholder for what
// should eventually move to its own package, kept here for now so the
// Canvas abstraction itself can be proven out first.
type EbitenCanvas struct {
	dst *ebiten.Image
}

func NewEbitenCanvas(dst *ebiten.Image) *EbitenCanvas {
	return &EbitenCanvas{dst: dst}
}

func (c *EbitenCanvas) Bounds() image.Rectangle {
	return c.dst.Bounds()
}

func (c *EbitenCanvas) DrawText(s string, face font.Face, x, y int, clr color.Color) {
	text.Draw(c.dst, s, face, x, y, clr)
}

func (c *EbitenCanvas) DrawImage(src string, x, y int) {
	img := loadImage(src)
	if img == nil {
		return
	}
	geoM := ebiten.GeoM{}
	geoM.Translate(float64(x), float64(y))
	c.dst.DrawImage(img, &ebiten.DrawImageOptions{GeoM: geoM})
}

// loadedImageCache is a stopgap, not where this belongs long-term: it wants
// to live as a field on whatever Canvas implementation eventually moves to
// its own package, not package-level state here. Kept simple for now since
// relocating it is a mechanical follow-up once that package exists.
var loadedImageCache = map[string]*ebiten.Image{}

func loadImage(src string) *ebiten.Image {
	if img, ok := loadedImageCache[src]; ok {
		return img
	}
	img, _, _ := ebitenutil.NewImageFromFile(src)
	loadedImageCache[src] = img
	return img
}

func drawRect(dst *ebiten.Image, rect image.Rectangle, clr color.Color) {
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Min.X), float64(rect.Max.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Min.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Min.X), float64(rect.Max.Y), float64(rect.Max.X), float64(rect.Max.Y), clr)
	ebitenutil.DrawLine(dst, float64(rect.Max.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Max.Y), clr)
}
