package browser

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// How long a ViewStack's scrollbar stays after the stack last moved, then
// fades out over: as a View's does.
const (
	stackScrollbarHold = 600 * time.Millisecond
	stackScrollbarFade = 300 * time.Millisecond
)

// stackScrollbar is the state of a ViewStack's own scrollbar.
type stackScrollbar struct {
	// style is how it looks, nil if it's off (see ViewStack.SetScrollbar).
	style *simpletheme.Scrollbar

	hovered, dragging bool
	// grab is where the thumb is held while dragged, as a fraction of its
	// length.
	grab float64
	// revealUntil is when it's hidden again, unless hovered or dragged;
	// shown is the position it was last revealed at, now the time of the
	// last Draw.
	revealUntil, now time.Duration
	shown            stackPosition
}

// SetScrollbar shows the stack's own scrollbar along its right edge,
// looking as style says, or hides it for a nil style: for a program that
// draws no scrollbar of its own (see VisibleRange and ScrollToRatio).
func (s *ViewStack) SetScrollbar(style *simpletheme.Scrollbar) {
	s.bar.style = style
}

// Bounds returns where the stack is drawn on the canvas.
func (s *ViewStack) Bounds() image.Rectangle { return s.bounds }

// height returns an estimate of the height of v's document, from its
// VisibleRange: exact when its end is in view.
func height(v *whynot.View) float64 {
	start, end := v.VisibleRange()
	b := v.Bounds()
	if end >= 1 {
		below := float64(v.DocumentBounds(0).Max.Y - b.Min.Y)
		if start <= 0 {
			return below
		}
		return below / (1 - start)
	}
	if end <= start || b.Dy() <= 0 {
		return 0
	}
	return float64(b.Dy()) / (end - start)
}

// heights returns an estimate of the height of each child's document,
// and their total.
func (s *ViewStack) heights() (hs []float64, total float64) {
	hs = make([]float64, len(s.views))
	for i, v := range s.views {
		hs[i] = height(v)
		total += hs[i]
	}
	return hs, total
}

// VisibleRange returns the part of the stack's content in view, as
// fractions of its height, as a View's does: estimated, from its
// children's own estimates.
func (s *ViewStack) VisibleRange() (start, end float64) {
	if len(s.views) == 0 {
		return 0, 1
	}
	hs, total := s.heights()
	if total <= float64(s.bounds.Dy()) {
		return 0, 1
	}
	top := 0.0
	for i := range s.top {
		top += hs[i]
	}
	childStart, _ := s.views[s.top].VisibleRange()
	top += childStart * hs[s.top]
	return top / total, math.Min(1, (top+float64(s.bounds.Dy()))/total)
}

// ScrollToRatio puts the top of the stack at ratio (clamped to [0, 1]) of
// its content's estimated height, as a View's does.
func (s *ViewStack) ScrollToRatio(ratio float64) {
	if len(s.views) == 0 {
		return
	}
	hs, total := s.heights()
	y := math.Max(0, math.Min(1, ratio)) * total
	for i, h := range hs {
		if y < h || i == len(hs)-1 {
			s.top = i
			if h > 0 {
				s.views[i].ScrollToRatio(y / h)
			} else {
				toStart(s.views[i])
			}
			break
		}
		y -= h
	}
	s.clampEnd()
}

// ScrollbarColor returns the color of a scrollbar's thumb, as a View's
// StyleSheet gives it: hover while it's highlighted, pressed while it's
// dragged.
func (s *ViewStack) ScrollbarColor(hover, pressed bool) color.Color {
	if style := s.bar.style; style != nil {
		switch {
		case pressed && style.Pressed != nil:
			return style.Pressed
		case hover && style.Hover != nil:
			return style.Hover
		case style.Idle != nil:
			return style.Idle
		}
	}
	if len(s.views) == 0 {
		return color.Transparent
	}
	return s.views[0].ScrollbarColor(hover, pressed)
}

// barGeometry returns the scrollbar's thickness, inset from the edge and
// shortest thumb, in canvas pixels: the style's, or a View's defaults.
func (s *ViewStack) barGeometry() (thickness, inset, minThumb int) {
	t, i, m := 6.0, 2.0, 24.0
	if style := s.bar.style; style != nil {
		if style.Thickness > 0 {
			t = style.Thickness
		}
		if style.Inset > 0 {
			i = style.Inset
		}
		if style.MinThumbLength > 0 {
			m = style.MinThumbLength
		}
	}
	scale := s.scale()
	return int(t * scale), int(i * scale), int(m * scale)
}

// thumb returns the scrollbar's thumb, or ok false if there's none: the
// scrollbar is off, or everything fits.
func (s *ViewStack) thumb() (r image.Rectangle, ok bool) {
	if s.bar.style == nil {
		return image.Rectangle{}, false
	}
	start, end := s.VisibleRange()
	if end-start >= 1 {
		return image.Rectangle{}, false
	}
	thickness, inset, minThumb := s.barGeometry()
	track := s.bounds.Dy()
	length := max(int((end-start)*float64(track)), minThumb)
	y := s.bounds.Min.Y + min(int(start*float64(track)), track-length)
	x := s.bounds.Max.X - inset - thickness
	return image.Rect(x, y, x+thickness, y+length), true
}

// onScrollbar reports whether p is on the scrollbar: its track along the
// right edge, with a little slack.
func (s *ViewStack) onScrollbar(p image.Point) bool {
	if _, ok := s.thumb(); !ok {
		return false
	}
	thickness, inset, _ := s.barGeometry()
	b := s.bounds
	return p.In(image.Rect(b.Max.X-thickness-2*inset, b.Min.Y, b.Max.X, b.Max.Y))
}

// barOpacity is how visible the scrollbar is at now, from 0 to 1.
func (s *ViewStack) barOpacity(now time.Duration) float64 {
	switch {
	case s.bar.style != nil && s.bar.style.AlwaysVisible, s.bar.dragging, s.bar.hovered:
		return 1
	case now >= s.bar.revealUntil:
		return 0
	}
	return min(1, float64(s.bar.revealUntil-now)/float64(stackScrollbarFade))
}

// barAnimating reports whether the scrollbar is fading.
func (s *ViewStack) barAnimating() bool {
	return s.bar.style != nil && s.bar.now < s.bar.revealUntil
}

// beginScrollbarDrag starts dragging the scrollbar from p: a press on the
// thumb holds it where pressed, one elsewhere on the track its middle,
// so it jumps under the pointer.
func (s *ViewStack) beginScrollbarDrag(p image.Point) {
	thumb, ok := s.thumb()
	if !ok {
		return
	}
	s.bar.dragging, s.bar.hovered = true, true
	s.bar.grab = 0.5
	if p.Y >= thumb.Min.Y && p.Y < thumb.Max.Y {
		s.bar.grab = float64(p.Y-thumb.Min.Y) / float64(thumb.Dy())
	}
	s.dragScrollbarTo(p.Y)
}

// dragScrollbarTo moves the dragged thumb to y.
func (s *ViewStack) dragScrollbarTo(y int) {
	thumb, ok := s.thumb()
	if !ok || s.bounds.Dy() <= 0 {
		return
	}
	top := float64(y-s.bounds.Min.Y) - s.bar.grab*float64(thumb.Dy())
	s.ScrollToRatio(top / float64(s.bounds.Dy()))
}

// endScrollbarDrag ends a drag: the scrollbar then fades out.
func (s *ViewStack) endScrollbarDrag() {
	s.bar.dragging = false
	s.bar.revealUntil = s.bar.now + stackScrollbarHold + stackScrollbarFade
}

// drawScrollbar draws the scrollbar at now, revealing it if the stack
// moved since it was last drawn.
func (s *ViewStack) drawScrollbar(dst canvas.Canvas, now time.Duration) {
	s.bar.now = now
	if p := s.position(); p != s.bar.shown {
		s.bar.shown = p
		s.bar.revealUntil = now + stackScrollbarHold + stackScrollbarFade
	}
	thumb, ok := s.thumb()
	if !ok {
		return
	}
	opacity := s.barOpacity(now)
	if opacity <= 0 {
		return
	}
	c := color.NRGBAModel.Convert(s.ScrollbarColor(s.bar.hovered || s.bar.dragging, s.bar.dragging)).(color.NRGBA)
	c.A = uint8(float64(c.A) * opacity)
	dst.DrawRect(thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), c)
}
