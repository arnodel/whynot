package whynot

import (
	"image"
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
	for i := range b.slots {
		box := b.boxAt(i)
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

// DrawFrom draws starting at c, so that c's position lands at (x, y) on
// dst - unlike DrawBox, it never calls Bounds() on the whole tree first,
// and never resolves or draws slots before c.index. Intended for View to
// call at the scroll cursor.
func (b *StackBox) DrawFrom(dst Canvas, c stackCursor, x, y int) {
	if c.index < 0 || c.index >= len(b.slots) {
		return
	}
	viewport := dst.Bounds()
	y -= int(c.offset)
	for i := c.index; i < len(b.slots); i++ {
		box := b.boxAt(i)
		childBounds := box.Bounds()
		if childBounds.Add(image.Pt(x, y)).Min.Y > viewport.Max.Y {
			break
		}
		DrawBox(box, dst, x, y)
		y += childBounds.Max.Y
	}
}

func (b *ContainerBox) drawContents(dst Canvas, x, y int) {
	DrawBox(b.inner, dst, x+b.innerPos.X, y+b.innerPos.Y)
}
