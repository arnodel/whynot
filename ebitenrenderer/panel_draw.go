package ebitenrenderer

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot/canvas"
)

// Scrollbar thumb dimensions, in logical (unscaled) pixels.
const (
	scrollbarWidth          = 6
	minScrollbarThumbHeight = 20
)

// scrollbarThumbRect returns the scrollbar thumb's rectangle in screen
// space, along bounds' right edge: its position and height are the
// visible part of the document (VisibleViewBounds) as a share of the
// whole (DocumentBounds), with a minimum height so it stays grabbable.
func (p *Panel) scrollbarThumbRect() (r image.Rectangle, ok bool) {
	trackHeight := p.bounds.Dy()
	doc := p.view.DocumentBounds()
	visible := p.view.VisibleViewBounds(image.Pt(p.bounds.Dx(), trackHeight))
	if doc.Dy() == 0 || visible.Dy() >= doc.Dy() {
		return image.Rectangle{}, false
	}

	width := max(1, int(scrollbarWidth*p.scale))
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

func (p *Panel) drawScrollbar(dst canvas.Canvas) {
	if r, ok := p.scrollbarThumbRect(); ok {
		dst.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), p.scrollbarColor())
	}
}

// scrollbarColor is the thumb's color from the View's StyleSheet, for its
// current hover/pressed state.
func (p *Panel) scrollbarColor() color.Color {
	return p.view.ScrollbarColor(p.scrollbarState.hover, p.scrollbarState.pressed)
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
