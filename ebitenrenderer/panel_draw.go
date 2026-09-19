package ebitenrenderer

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
)

// minScrollbarThumbHeight mirrors cmd/whynot's own 20px-at-1x minimum.
const minScrollbarThumbHeight = 20

// scrollbarThumbRect returns the scrollbar thumb's rectangle in screen
// space - see cmd/whynot/draw.go's scrollbarThumbRect for the full
// reasoning (thumb width from ScaledViewMargins, DocumentBounds/
// VisibleViewBounds ratio, minimum height clamp); this is that same
// logic in bounds-relative terms instead of the whole window.
func (p *Panel) scrollbarThumbRect() (r image.Rectangle, ok bool) {
	trackHeight := p.bounds.Dy()
	doc := p.view.DocumentBounds()
	visible := p.view.VisibleViewBounds(image.Pt(p.bounds.Dx(), trackHeight))
	if doc.Dy() == 0 || visible.Dy() >= doc.Dy() {
		return image.Rectangle{}, false
	}

	margin := p.view.ScaledViewMargins()
	width := int(margin.Right * 0.3)
	if width < 1 {
		width = 1
	}
	minHeight := int(minScrollbarThumbHeight * p.scale)

	y := p.bounds.Min.Y + visible.Min.Y*trackHeight/doc.Dy()
	height := visible.Dy() * trackHeight / doc.Dy()
	if height < minHeight {
		height = minHeight
	}
	if y+height > p.bounds.Max.Y {
		y = p.bounds.Max.Y - height
	}

	return image.Rect(p.bounds.Max.X-width, y, p.bounds.Max.X, y+height), true
}

func (p *Panel) drawScrollbar(canvas whynot.Canvas) {
	if r, ok := p.scrollbarThumbRect(); ok {
		canvas.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), p.scrollbarColor())
	}
}

// scrollbarColor picks the thumb's color via the last StyleSheet given
// to SetStyleSheet, if it implements the optional
// whynot.ScrollbarStyleSheet - falls back to a flat default if none was
// ever given, or it doesn't opt in.
func (p *Panel) scrollbarColor() color.Color {
	if sh, ok := p.styleSheet.(whynot.ScrollbarStyleSheet); ok {
		return sh.ScrollbarColor(p.scrollbarState.hover, p.scrollbarState.pressed)
	}
	return color.RGBA{0x80, 0x80, 0x80, 0xA0}
}

// Draw renders the panel - the document, clipped to Bounds, and its
// scrollbar if enabled - onto dst.
func (p *Panel) Draw(dst *ebiten.Image) {
	// Cheap when width/scale are unchanged; called unconditionally so
	// an animated GIF gets fresh timing every rendered frame regardless
	// of whether/how many times Update ran this tick.
	p.relayout()

	// SubImage is a real clipping destination in ebiten: drawing onto
	// it is scissored to bounds, but coordinates stay in dst's own
	// absolute space, not re-based to zero - so View.Draw's x, y below
	// must be bounds' own absolute position, exactly like cmd/whynot's
	// existing view.Draw(canvas, 0, g.toolbarHeight) call, not (0, 0).
	sub := dst.SubImage(p.bounds).(*ebiten.Image)
	canvas := p.Renderer.NewCanvas(sub)
	p.view.Draw(canvas, p.bounds.Min.X, p.bounds.Min.Y)

	if p.scrollbarEnabled {
		p.drawScrollbar(canvas)
	}
}
