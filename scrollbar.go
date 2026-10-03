package whynot

import (
	"image"
	"image/color"
	"time"

	"github.com/arnodel/whynot/canvas"
)

// vscrollbar is the state of a View's own vertical scrollbar (see
// WithScrollbar): drawn by the View along its right edge, styled by its
// StyleSheet, and hovered and dragged through a Controller.
type vscrollbar struct {
	enabled  bool
	hovered  bool // the pointer is on the bar
	dragging bool
	// grab is where the thumb was grabbed, as a fraction of its length,
	// while dragging: the thumb's length can change during a drag.
	grab float64

	// The bar shows until revealUntil (on the clock of View.Layout's
	// now), fading out at the end.
	revealUntil time.Duration
}

// WithScrollbar shows the View's own vertical scrollbar, drawn along its
// right edge as its StyleSheet says, whenever the document is taller
// than the View. A Controller lets the user drag it.
func WithScrollbar() ViewOption {
	return func(v *View) { v.vbar.enabled = true }
}

// SetScrollbar shows or hides the View's own vertical scrollbar (see
// WithScrollbar).
func (v *View) SetScrollbar(on bool) {
	v.vbar.enabled = on
}

// revealScrollbar shows the vertical scrollbar from now, fading out after
// a while: the document just scrolled.
func (v *View) revealScrollbar() {
	v.vbar.revealUntil = v.ctx.Time + hscrollRevealHold + hscrollRevealFade
}

// scrollbarBar is the scrollbar's geometry at the View's scale.
func (v *View) scrollbarBar() barGeometry {
	return scaledBar(v.ctx.Styles.ScrollbarGeometry(), v.ctx.Scale)
}

// scrollbarThumb is the vertical scrollbar's thumb, in the View's
// coordinates, or ok=false if there's none: the scrollbar is off, or the
// whole document fits.
func (v *View) scrollbarThumb() (r image.Rectangle, ok bool) {
	if !v.vbar.enabled {
		return image.Rectangle{}, false
	}
	doc := v.DocumentBounds()
	visible := v.VisibleViewBounds(image.Pt(v.width, v.height))
	if doc.Dy() == 0 || visible.Dy() >= doc.Dy() {
		return image.Rectangle{}, false
	}
	bar := v.scrollbarBar()
	track := v.height
	length := max(visible.Dy()*track/doc.Dy(), bar.minThumb)
	y := min(visible.Min.Y*track/doc.Dy(), track-length)
	x := v.width - bar.inset - bar.thickness
	return image.Rect(x, y, x+bar.thickness, y+length), true
}

// scrollbarZone is the strip along the View's right edge that counts as
// its scrollbar for the pointer: the thumb's track, with a little slack.
func (v *View) scrollbarZone() image.Rectangle {
	bar := v.scrollbarBar()
	return image.Rect(v.width-bar.thickness-2*bar.inset, 0, v.width, v.height)
}

// onScrollbar reports whether p, in the View's coordinates, is on the
// scrollbar.
func (v *View) onScrollbar(p image.Point) bool {
	_, ok := v.scrollbarThumb()
	return ok && p.In(v.scrollbarZone())
}

// scrollbarOpacity is how visible the vertical scrollbar is at now, from
// 0 (hidden) to 1.
func (v *View) scrollbarOpacity(now time.Duration) float64 {
	switch {
	case v.scrollbarBar().alwaysVisible, v.vbar.dragging, v.vbar.hovered:
		return 1
	case now >= v.vbar.revealUntil:
		return 0
	}
	return min(1, float64(v.vbar.revealUntil-now)/float64(hscrollRevealFade))
}

// scrollbarAnimating reports whether the vertical scrollbar is fading.
func (v *View) scrollbarAnimating(now time.Duration) bool {
	return v.vbar.enabled && now < v.vbar.revealUntil
}

// drawScrollbar draws the vertical scrollbar onto dst, the View's top-left
// corner at (x, y).
func (v *View) drawScrollbar(dst canvas.Canvas, x, y int) {
	thumb, ok := v.scrollbarThumb()
	if !ok {
		return
	}
	opacity := v.scrollbarOpacity(v.ctx.Time)
	if opacity <= 0 {
		return
	}
	pressed := v.vbar.dragging
	c := color.NRGBAModel.Convert(v.ctx.Styles.ScrollbarColor(v.vbar.hovered || pressed, pressed)).(color.NRGBA)
	c.A = uint8(float64(c.A) * opacity)
	thumb = thumb.Add(image.Pt(x, y))
	dst.DrawRect(thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), c)
}

// beginScrollbarDrag starts dragging the scrollbar from p, in the View's
// coordinates: a press on the thumb grabs it where pressed, one elsewhere
// on the track grabs its middle there, so it jumps under the pointer.
func (v *View) beginScrollbarDrag(p image.Point) {
	thumb, ok := v.scrollbarThumb()
	if !ok {
		return
	}
	v.vbar.dragging, v.vbar.hovered = true, true
	if p.Y >= thumb.Min.Y && p.Y < thumb.Max.Y {
		v.vbar.grab = float64(p.Y-thumb.Min.Y) / float64(thumb.Dy())
	} else {
		v.vbar.grab = 0.5
	}
	v.dragScrollbarTo(p.Y)
}

// dragScrollbarTo moves the dragged thumb to follow the pointer's y, in
// the View's coordinates.
func (v *View) dragScrollbarTo(y int) {
	thumb, ok := v.scrollbarThumb()
	if !ok || v.height <= 0 {
		return
	}
	top := float64(y) - v.vbar.grab*float64(thumb.Dy())
	v.ScrollToRatio(top / float64(v.height))
}

// endScrollbarDrag ends a drag: the scrollbar then fades out like after
// any other scroll, unless the pointer stays on it.
func (v *View) endScrollbarDrag() {
	v.vbar.dragging = false
	v.revealScrollbar()
}
