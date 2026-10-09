package engine

import (
	"image"
	"image/color"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/internal/imagecache"
)

// InlineLayout is one part of a line of inline content, placed by
// lineBuilder. DrawInline and HitTest take the pen position LineBox
// placed it at: x the start of its advance, y the baseline.
type InlineLayout interface {
	// BoundsAndAdvance returns the part's ink bounds relative to its pen
	// position, and how far it moves the pen.
	BoundsAndAdvance() (image.Rectangle, int)
	Bounds() image.Rectangle
	Source() Source
	SpaceWidth() int
	// Glued reports whether this item directly abuts the previous one in
	// its line with no source whitespace between them - e.g. a word right
	// next to a code span, or a non-breaking space's own neighbors. A
	// glued boundary gets no gap and is never a line-break point (see
	// lineBuilder.gap and wrapLines). Meaningless for an item that's
	// never anything but a line's first part (listItemMarkerBox,
	// checkboxBox) - never queried there.
	Glued() bool
	// DrawInline's now - see BlockLayout.drawContents's identical parameter.
	DrawInline(dst canvas.Canvas, x, y int, now time.Duration)
	HitTest(p image.Point, x, y int) (hit Hit, offset image.Point)
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

	// glued - see InlineLayout.Glued's doc comment. Set once at
	// construction (InlineText.GetInlineLayout), from the source
	// whitespace tracked at compile time (see compiler.pendingSpace) -
	// never mutated after.
	glued bool

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

func (b *TextBox) Glued() bool {
	return b.glued
}

func (b *TextBox) SpaceWidth() int {
	if !b.spaceComputed {
		adv, _ := b.Face.GlyphAdvance(' ')
		b.spaceWidth = adv.Ceil()
		b.spaceComputed = true
	}
	return b.spaceWidth
}

func (b *TextBox) DrawInline(dst canvas.Canvas, x, y int, now time.Duration) {
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
}

func (b *TextBox) HitTest(p image.Point, x, y int) (Hit, image.Point) {
	return hitIfInside(b, p, x, y)
}

func (b *TextBox) PendingImages() []string {
	return b.pending
}

// listItemMarkerBox hangs a list item's marker to the left of the line's
// start, so the item's text starts at the line's own x=0.
type listItemMarkerBox struct {
	Marker InlineLayout
}

var _ InlineLayout = (*listItemMarkerBox)(nil)

// BoundsAndAdvance reports no width, and an advance of minus the space
// width: that cancels the gap lineBuilder inserts before the next part,
// so the text after the marker starts exactly where the marker box is.
func (b *listItemMarkerBox) BoundsAndAdvance() (image.Rectangle, int) {
	bounds, _ := b.Marker.BoundsAndAdvance()
	return image.Rect(0, bounds.Min.Y, 0, bounds.Max.Y), -b.Marker.SpaceWidth()
}

func (b *listItemMarkerBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	return bounds
}

func (b *listItemMarkerBox) Source() Source {
	return b.Marker.Source()
}

func (b *listItemMarkerBox) SpaceWidth() int {
	return b.Marker.SpaceWidth()
}

// Glued is always false - a listItemMarkerBox is only ever a line's first
// part (ListItemHeadBlock.GetBlockLayout), so it's never queried by the
// gap/break logic that Glued exists for.
func (b *listItemMarkerBox) Glued() bool {
	return false
}

func (b *listItemMarkerBox) DrawInline(dst canvas.Canvas, x, y int, now time.Duration) {
	b.Marker.DrawInline(dst, b.markerX(x), y, now)
}

// HitTest checks the marker where DrawInline draws it, hanging to the left
// of x - not against this box's own zero-width bounds.
func (b *listItemMarkerBox) HitTest(p image.Point, x, y int) (Hit, image.Point) {
	return b.Marker.HitTest(p, b.markerX(x), y)
}

// markerX is where the marker is drawn for a box at x: one space width to
// the left of x, ending there.
func (b *listItemMarkerBox) markerX(x int) int {
	_, advance := b.Marker.BoundsAndAdvance()
	return x - advance - b.Marker.SpaceWidth()
}

func (b *listItemMarkerBox) PendingImages() []string {
	return b.Marker.PendingImages()
}

// checkboxBox is a GFM task list item's checkbox marker, drawn with plain
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
type checkboxBox struct {
	checked    bool
	size       int
	thickness  int
	spaceWidth int
	color      color.Color
	source     Inline
}

var _ InlineLayout = (*checkboxBox)(nil)

// newCheckboxBox sizes the box as a fraction of face's own (already
// scaled) Ascent - CapHeight/XHeight would give a tighter fit, but some
// fonts report them as 0 or even negative, so Ascent (always populated)
// is the reliable choice.
func newCheckboxBox(checked bool, face font.Face, color color.Color, source Inline) *checkboxBox {
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
	return &checkboxBox{
		checked:    checked,
		size:       size,
		thickness:  thickness,
		spaceWidth: spaceAdvance.Ceil(),
		color:      color,
		source:     source,
	}
}

func (b *checkboxBox) Source() Source {
	return b.source
}

// BoundsAndAdvance places the box entirely above the baseline (Min.Y =
// -size, Max.Y = 0, matching how a same-sized glyph like □ itself sits),
// so it lines up with surrounding text the same way TaskCheckbox's glyph
// path does.
func (b *checkboxBox) BoundsAndAdvance() (image.Rectangle, int) {
	return image.Rect(0, -b.size, b.size, 0), b.size
}

func (b *checkboxBox) Bounds() image.Rectangle {
	bounds, _ := b.BoundsAndAdvance()
	return bounds
}

func (b *checkboxBox) SpaceWidth() int {
	return b.spaceWidth
}

// Glued is always false - a checkboxBox only ever appears as a
// listItemMarkerBox's Marker (a list item's task checkbox), so like
// listItemMarkerBox itself it's never queried by the gap/break logic.
func (b *checkboxBox) Glued() bool {
	return false
}

func (b *checkboxBox) DrawInline(dst canvas.Canvas, x, y int, now time.Duration) {
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
}

func (b *checkboxBox) HitTest(p image.Point, x, y int) (Hit, image.Point) {
	return hitIfInside(b, p, x, y)
}

func (b *checkboxBox) PendingImages() []string {
	return nil
}

type ImageBox struct {
	// Image is the already-decoded image imagecache.Cache.Load returned -
	// carried forward from InlineImage.GetInlineLayout so DrawInline
	// can pass it straight to Canvas.DrawImage, which never fetches or
	// decodes anything itself. nil while the image is still pending but
	// its dimensions are already known - bounds is still the correct,
	// final (scaled) size, so DrawInline draws a placeholder rect
	// instead, and nothing needs to reflow once Image is filled in later.
	// anim is set instead of Image for an animated GIF - exactly one of
	// the two is non-nil on a settled ImageBox.
	Image            image.Image
	Animation        *imagecache.Animation
	Rect             image.Rectangle
	placeholderColor color.Color

	// Pending is the one resolved src this box is still waiting on,
	// while img == nil - see PendingImages.
	Pending []string

	// glued - see InlineLayout.Glued's doc comment and TextBox.glued.
	glued bool

	// source is the InlineImage this ImageBox was built from - see Source.
	source Inline
}

var _ InlineLayout = (*ImageBox)(nil)

func (b *ImageBox) Source() Source {
	return b.source
}

func (b *ImageBox) BoundsAndAdvance() (image.Rectangle, int) {
	return b.Rect, b.Rect.Dx()
}

func (b *ImageBox) Bounds() image.Rectangle {
	return b.Rect
}

func (b *ImageBox) SpaceWidth() int {
	return 0
}

func (b *ImageBox) Glued() bool {
	return b.glued
}

func (b *ImageBox) DrawInline(dst canvas.Canvas, x, y int, now time.Duration) {
	switch {
	case b.Animation != nil:
		dst.DrawImage(b.Animation.CurrentFrame(now), x, y, b.Rect.Dx(), b.Rect.Dy())
	case b.Image != nil:
		dst.DrawImage(b.Image, x, y, b.Rect.Dx(), b.Rect.Dy())
	default:
		dst.DrawRect(x, y, b.Rect.Dx(), b.Rect.Dy(), b.placeholderColor)
	}
}

func (b *ImageBox) HitTest(p image.Point, x, y int) (Hit, image.Point) {
	return hitIfInside(b, p, x, y)
}

func (b *ImageBox) PendingImages() []string {
	return b.Pending
}

// lineBreakBox is a LineBreak's layout: wrapLines ends the line at it, and
// it's never part of a line.
type lineBreakBox struct {
	source Inline
}

func (b *lineBreakBox) Source() Source                                            { return b.source }
func (b *lineBreakBox) BoundsAndAdvance() (image.Rectangle, int)                  { return image.Rectangle{}, 0 }
func (b *lineBreakBox) Bounds() image.Rectangle                                   { return image.Rectangle{} }
func (b *lineBreakBox) SpaceWidth() int                                           { return 0 }
func (b *lineBreakBox) Glued() bool                                               { return false }
func (b *lineBreakBox) DrawInline(dst canvas.Canvas, x, y int, now time.Duration) {}
func (b *lineBreakBox) HitTest(p image.Point, x, y int) (Hit, image.Point)        { return nil, image.Point{} }
func (b *lineBreakBox) PendingImages() []string                                   { return nil }

// hitIfInside is the HitTest of a leaf InlineLayout: a hit on the box
// itself if p falls within its bounds at pen position (x, y).
func hitIfInside(b InlineLayout, p image.Point, x, y int) (Hit, image.Point) {
	if p.In(b.Bounds().Add(image.Pt(x, y))) {
		return b, image.Pt(x, y)
	}
	return nil, image.Point{}
}
