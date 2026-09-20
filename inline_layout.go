package whynot

import (
	"image"
	"image/color"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
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
	// DrawInline's now - see BlockLayout.drawContents's identical parameter.
	DrawInline(dst Canvas, x, y int, now time.Duration) int
	HitTest(p image.Point, x, y int) (hit Hit, offset image.Point, nextX int)
	// PendingImages - see BlockLayout's identical method.
	PendingImages() []string
}

type TextBox struct {
	Text  string
	Face  font.Face
	Color color.Color

	// StrikeThickness is the strikethrough line's thickness in pixels; 0
	// means no strikethrough.
	StrikeThickness int

	// LineHeight is the multiplier applied to Face's natural Ascent+Descent
	// to get this box's vertical extent (StyleSheet.LineHeight) - the
	// font's own metrics don't reliably encode comfortable line spacing
	// (see BoundsAndAdvance), so this is what actually separates
	// consecutive wrapped lines within a paragraph. The zero value (an
	// unset TextBox, e.g. built directly by a test rather than through
	// InlineText.GetInlineLayout) falls back to the pre-LineHeight
	// behavior - bare Ascent+Descent, no extra space.
	LineHeight float64

	// pending is set only when this TextBox is standing in for an image
	// that hasn't settled yet (InlineImage.GetInlineLayout's "loading"
	// or fallback text) - nil for every ordinary text box.
	pending []string

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
		// A font's own Metrics().Height (its recommended line-to-line
		// spacing) doesn't reliably exceed bare Ascent+Descent - many real
		// fonts, including the bundled Go fonts, report an essentially
		// zero line gap - so it can't be trusted to give comfortable
		// reading spacing on its own. LineHeight (StyleSheet-driven, see
		// TextBox.LineHeight) is what actually adds it: the extra space is
		// split evenly above and below the font's natural ascent/descent
		// band, the same half-leading distribution CSS's line-height
		// uses, so glyphs stay vertically centered in their taller line
		// box rather than crowding its top. LineHeight <= 1 (including
		// the zero value an unset TextBox has) adds nothing, reproducing
		// the plain Ascent+Descent bounds from before this field existed.
		natural := metrics.Ascent + metrics.Descent
		extra := fixed.Int26_6(float64(natural)*b.LineHeight) - natural
		if extra < 0 {
			extra = 0
		}
		halfExtra := extra / 2
		top := metrics.Ascent + halfExtra
		bottom := metrics.Descent + (extra - halfExtra)
		b.bounds = image.Rect(
			bounds.Min.X.Floor(),
			-top.Ceil(),
			bounds.Max.X.Ceil(),
			bottom.Ceil(),
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

func (b *TextBox) DrawInline(dst Canvas, x, y int, now time.Duration) int {
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

func (b *TextBox) PendingImages() []string {
	return b.pending
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

func (b *ListItemMarkerBox) DrawInline(dst Canvas, x, y int, now time.Duration) int {
	_, advance := b.Marker.BoundsAndAdvance()
	space := b.Marker.SpaceWidth()
	b.Marker.DrawInline(dst, x-advance-space, y, now)
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

func (b *ListItemMarkerBox) PendingImages() []string {
	return b.Marker.PendingImages()
}

// CheckboxBox is a GFM task list item's checkbox marker, drawn with plain
// filled rects (Canvas.DrawRect) rather than a font glyph - TaskCheckbox's
// fallback for a face that doesn't have the ballot-box glyphs (☐/☑; most
// fonts, including the bundled Go fonts, don't). An outlined square for
// unchecked, the same outline with a filled square inside for checked -
// deliberately not a baked icon image: this way it needs no embedded
// asset, scales naturally from the resolved face's own metrics (already
// DPI/zoom-scaled, like every face SelectFace returns - so it tracks font
// size and zoom together without a separate StyleSheet dimensional
// constant), and always matches the resolved text color instead of being
// a fixed-color raster that could clash with a custom StyleSheet.
type CheckboxBox struct {
	checked    bool
	size       int
	thickness  int
	spaceWidth int
	color      color.Color
	source     Inline
}

var _ InlineLayout = (*CheckboxBox)(nil)

// newCheckboxBox sizes the box as a fraction of face's own (already
// scaled) Ascent - CapHeight/XHeight would give a tighter fit, but some
// fonts report them as 0 or even negative, so Ascent (always populated)
// is the reliable choice.
func newCheckboxBox(checked bool, face font.Face, color color.Color, source Inline) *CheckboxBox {
	ascent := face.Metrics().Ascent.Ceil()
	size := int(float64(ascent) * 0.72)
	if size < 2 {
		size = 2
	}
	thickness := size / 8
	if thickness < 1 {
		thickness = 1
	}
	spaceAdvance, _ := face.GlyphAdvance(' ')
	return &CheckboxBox{
		checked:    checked,
		size:       size,
		thickness:  thickness,
		spaceWidth: spaceAdvance.Ceil(),
		color:      color,
		source:     source,
	}
}

func (b *CheckboxBox) Source() Source {
	return b.source
}

// BoundsAndAdvance places the box entirely above the baseline (Min.Y =
// -size, Max.Y = 0, matching how a same-sized glyph like □ itself sits),
// so it lines up with surrounding text the same way TaskCheckbox's glyph
// path does.
func (b *CheckboxBox) BoundsAndAdvance() (image.Rectangle, int) {
	return image.Rect(0, -b.size, b.size, 0), b.size
}

func (b *CheckboxBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	return bounds
}

func (b *CheckboxBox) SpaceWidth() int {
	return b.spaceWidth
}

func (b *CheckboxBox) DrawInline(dst Canvas, x, y int, now time.Duration) int {
	top := y - b.size
	t := b.thickness
	dst.DrawRect(x, top, b.size, t, b.color)          // top edge
	dst.DrawRect(x, top+b.size-t, b.size, t, b.color) // bottom edge
	dst.DrawRect(x, top, t, b.size, b.color)          // left edge
	dst.DrawRect(x+b.size-t, top, t, b.size, b.color) // right edge
	if b.checked {
		pad := b.size / 4
		if inner := b.size - 2*pad; inner > 0 {
			dst.DrawRect(x+pad, top+pad, inner, inner, b.color)
		}
	}
	return x + b.size
}

func (b *CheckboxBox) HitTest(p image.Point, x, y int) (Hit, image.Point, int) {
	bounds, advance := b.BoundsAndAdvance()
	if p.In(bounds.Add(image.Pt(x, y))) {
		return b, image.Pt(x, y), x + advance
	}
	return nil, image.Point{}, x + advance
}

func (b *CheckboxBox) PendingImages() []string {
	return nil
}

type ImageBox struct {
	// img is the already-decoded image ImageCache.Load returned -
	// carried forward from InlineImage.GetInlineLayout so DrawInline
	// can pass it straight to Canvas.DrawImage, which never fetches or
	// decodes anything itself. nil while the image is still pending but
	// its dimensions are already known - bounds is still the correct,
	// final (scaled) size, so DrawInline draws a placeholder rect
	// instead, and nothing needs to reflow once img is filled in later.
	// anim is set instead of img for an animated GIF - exactly one of
	// the two is non-nil on a settled ImageBox.
	img              image.Image
	anim             *AnimatedImage
	bounds           image.Rectangle
	placeholderColor color.Color

	// pending is the one resolved src this box is still waiting on,
	// while img == nil - see PendingImages.
	pending []string

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

func (b *ImageBox) DrawInline(dst Canvas, x, y int, now time.Duration) int {
	switch {
	case b.anim != nil:
		dst.DrawImage(b.anim.CurrentFrame(now), x, y, b.bounds.Dx(), b.bounds.Dy())
	case b.img != nil:
		dst.DrawImage(b.img, x, y, b.bounds.Dx(), b.bounds.Dy())
	default:
		dst.DrawRect(x, y, b.bounds.Dx(), b.bounds.Dy(), b.placeholderColor)
	}
	return x + b.bounds.Dx()
}

func (b *ImageBox) HitTest(p image.Point, x, y int) (Hit, image.Point, int) {
	if p.In(b.bounds.Add(image.Pt(x, y))) {
		return b, image.Pt(x, y), x + b.bounds.Dx()
	}
	return nil, image.Point{}, x + b.bounds.Dx()
}

func (b *ImageBox) PendingImages() []string {
	return b.pending
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
