package whynot

import (
	"image"
	"image/color"

	"golang.org/x/image/font"
)

// InlineLayout's HitTest mirrors DrawInline's own calling convention exactly
// - same (x, y), same "next x" return - so LineBox.HitTest can drive the
// identical accumulation loop drawContents does, just calling HitTest
// instead of DrawInline at each step.
type InlineLayout interface {
	BoundsAndAdvance() (image.Rectangle, int)
	Bounds() image.Rectangle
	Source() Source
	SpaceWidth() int
	DrawInline(dst Canvas, x, y int) int
	HitTest(p image.Point, x, y int) (hit Hit, offset image.Point, nextX int)
}

type TextBox struct {
	Text  string
	Face  font.Face
	Color color.Color

	// StrikeThickness is the strikethrough line's thickness in pixels; 0
	// means no strikethrough.
	StrikeThickness int

	// source is the InlineText this TextBox was built from - see Source.
	source Inline

	boundsComputed bool
	bounds         image.Rectangle
	advance        int

	spaceComputed bool
	spaceWidth    int
}

var _ InlineLayout = (*TextBox)(nil)

func (b *TextBox) Source() Source {
	return b.source
}

func (b *TextBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	return bounds
}

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

func (b *TextBox) DrawInline(dst Canvas, x, y int) int {
	_, advance := b.BoundsAndAdvance()
	dst.DrawText(b.Text, b.Face, x, y, b.Color)
	if b.StrikeThickness > 0 {
		// Halfway up the x-height lands the line through the vertical
		// middle of lowercase letters - the standard strike position.
		// DrawRect's y is the bar's top edge, so shift up by half the
		// thickness to center the bar on that position rather than
		// drawing it entirely below.
		xHeight := b.Face.Metrics().XHeight.Ceil()
		dst.DrawRect(x, y-xHeight/2-b.StrikeThickness/2, advance, b.StrikeThickness, b.Color)
	}
	return x + advance
}

func (b *TextBox) HitTest(p image.Point, x, y int) (Hit, image.Point, int) {
	bounds, advance := b.BoundsAndAdvance()
	if p.In(bounds.Add(image.Pt(x, y))) {
		return b, image.Pt(x, y), x + advance
	}
	return nil, image.Point{}, x + advance
}

type ListItemMarkerBox struct {
	Marker InlineLayout
}

var _ InlineLayout = (*ListItemMarkerBox)(nil)

func (b *ListItemMarkerBox) BoundsAndAdvance() (image.Rectangle, int) {
	bounds, _ := b.Marker.BoundsAndAdvance()
	return image.Rect(0, bounds.Min.Y, 0, bounds.Max.Y), 0
}

func (b *ListItemMarkerBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	return bounds
}

func (b *ListItemMarkerBox) Source() Source {
	return b.Marker.Source()
}

func (b *ListItemMarkerBox) SpaceWidth() int {
	return b.Marker.SpaceWidth()
}

func (b *ListItemMarkerBox) DrawInline(dst Canvas, x, y int) int {
	_, advance := b.Marker.BoundsAndAdvance()
	space := b.Marker.SpaceWidth()
	b.Marker.DrawInline(dst, x-advance-space, y)
	return x - space
}

// HitTest mirrors DrawInline exactly: the marker is drawn hanging off to
// the left of x (DrawInline's x-advance-space), not at x itself, so a
// naive check against BoundsAndAdvance's own (zero-width) bounds would
// never match a click on the visible marker glyph.
func (b *ListItemMarkerBox) HitTest(p image.Point, x, y int) (Hit, image.Point, int) {
	_, advance := b.Marker.BoundsAndAdvance()
	space := b.Marker.SpaceWidth()
	if hit, offset, _ := b.Marker.HitTest(p, x-advance-space, y); hit != nil {
		return hit, offset, x - space
	}
	return nil, image.Point{}, x - space
}

type ImageBox struct {
	src    string
	bounds image.Rectangle

	// source is the InlineImage this ImageBox was built from - see Source.
	source Inline
}

var _ InlineLayout = (*ImageBox)(nil)

func (b *ImageBox) Source() Source {
	return b.source
}

func (b *ImageBox) BoundsAndAdvance() (image.Rectangle, int) {
	return b.bounds, b.bounds.Dx()
}

func (b *ImageBox) Bounds() image.Rectangle {
	return b.bounds
}

func (b *ImageBox) SpaceWidth() int {
	return 0
}

func (b *ImageBox) DrawInline(dst Canvas, x, y int) int {
	dst.DrawImage(b.src, x, y)
	return x + b.bounds.Dx()
}

func (b *ImageBox) HitTest(p image.Point, x, y int) (Hit, image.Point, int) {
	if p.In(b.bounds.Add(image.Pt(x, y))) {
		return b, image.Pt(x, y), x + b.bounds.Dx()
	}
	return nil, image.Point{}, x + b.bounds.Dx()
}

func splitBoxes(boxes []InlineLayout, width int) (int, image.Rectangle) {
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
		// bounds.Dx(), not Max.X: a later word's own bounds can pull
		// Min.X away from 0 (e.g. left-side bearing), so Max.X alone
		// isn't the line's true width - comparing it directly against
		// width makes the wrap constraint tighter than intended, by
		// however far Min.X has drifted.
		if bounds.Dx() > width {
			return i + 1, bounds
		}
		advance += boxAdvance
	}
	return len(boxes), bounds
}
