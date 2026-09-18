package main

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
)

func (g *game) Draw(screen *ebiten.Image) {
	canvas := g.renderer.NewCanvas(screen)

	// View.Draw fills the background itself, from the View's StyleSheet -
	// no separate clear step needed here. It's drawn below the toolbar,
	// which is painted over it afterward.
	g.current.view.Draw(canvas, 0, g.toolbarHeight)
	g.drawToolbar(screen, canvas)
	g.drawZoomIndicator(canvas)
	g.drawScrollbar(canvas)

	if g.debugHit {
		docY := g.hoverY - g.toolbarHeight
		if hit, offset := g.current.view.HitTest(g.hoverX, docY); hit != nil {
			drawOutline(canvas, hit.Bounds().Add(offset).Add(image.Pt(0, g.toolbarHeight)), color.RGBA{255, 0, 0, 255})
		}
	}
}

// drawToolbar paints the address bar (the current document's location,
// or - while hovering a link - that link's destination instead, in
// StyleSheet.HighlightColor to match the hovered link's own color in
// the document) and the back/forward/reload/zoom/theme buttons. Icon
// drawing needs dst directly - whynot.Canvas has no primitive for a
// scaled, tinted image - so this is the one part of cmd/whynot's own
// UI that goes around the library's rendering abstraction rather than
// through it.
func (g *game) drawToolbar(dst *ebiten.Image, canvas whynot.Canvas) {
	canvas.DrawRect(0, 0, g.width, g.toolbarHeight, color.RGBA{0x20, 0x20, 0x20, 0xFF})

	drawButton(dst, canvas, backIcon, g.backButton, len(g.history) > 0, g.backState)
	drawButton(dst, canvas, forwardIcon, g.forwardButton, len(g.future) > 0, g.forwardState)
	drawButton(dst, canvas, reloadIcon, g.reloadButton, true, g.reloadState)
	drawButton(dst, canvas, zoomOutIcon, g.zoomOutButton, true, g.zoomOutState)
	drawButton(dst, canvas, zoomInIcon, g.zoomInButton, true, g.zoomInState)
	themeIcon := lightModeIcon
	if g.darkTheme {
		themeIcon = darkModeIcon
	}
	drawButton(dst, canvas, themeIcon, g.themeButton, true, g.themeState)

	face, err := g.toolbarFaceSelector.SelectFace(whynot.TextStyle{Size: 14})
	if err != nil {
		return
	}
	text, textColor := g.current.location.String(), color.Color(color.RGBA{0xCC, 0xCC, 0xCC, 0xFF})
	if g.hoverDest != "" {
		text, textColor = g.hoverDest, g.styleSheet.HighlightColor()
	}
	x := g.reloadButton.Max.X + int(16*g.deviceScale)
	maxWidth := g.zoomOutButton.Min.X - int(16*g.deviceScale) - x
	text = truncateMiddle(face, text, maxWidth)
	canvas.DrawText(text, face, x, baselineIn(face, image.Rect(x, 0, g.width, g.toolbarHeight)), textColor)
}

// truncateMiddle shortens s, if needed, so it renders in face no wider
// than maxWidth - cutting from the middle (keeping the start and end,
// usually the most identifying parts of a URL or path) and joining the
// two halves with an ellipsis, rather than truncating at the end. s is
// returned unchanged if it already fits.
func truncateMiddle(face font.Face, s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if font.MeasureString(face, s).Ceil() <= maxWidth {
		return s
	}
	const ellipsis = "…"
	if font.MeasureString(face, ellipsis).Ceil() > maxWidth {
		return ""
	}
	runes := []rune(s)
	left, right := len(runes)/2, len(runes)/2
	for left > 0 || right < len(runes) {
		if left > 0 {
			left--
		}
		if right < len(runes) {
			right++
		}
		candidate := string(runes[:left]) + ellipsis + string(runes[right:])
		if font.MeasureString(face, candidate).Ceil() <= maxWidth {
			return candidate
		}
	}
	return ellipsis
}

// zoomIndicatorDuration is how long the "N%" popup stays up after a
// zoom change - long enough to read, short enough to get out of the
// way on its own.
const zoomIndicatorDuration = 1500 * time.Millisecond

// drawZoomIndicator shows the current zoom level for a little while
// after it changes (see setZoom), in the content area's top-right
// corner - not the toolbar's, which will get its own zoom control
// later and is a fixed size regardless of zoom (see deviceScale);
// this is explicitly "on top of the document" instead, since zoom is a
// document-only setting. Uses toolbarFaceSelector, for the same reason
// drawToolbar does - a size fixed at the display's own scale, not
// zoomed.
func (g *game) drawZoomIndicator(canvas whynot.Canvas) {
	if !time.Now().Before(g.zoomIndicatorUntil) {
		return
	}
	face, err := g.toolbarFaceSelector.SelectFace(whynot.TextStyle{Size: 14})
	if err != nil {
		return
	}
	label := fmt.Sprintf("%.0f%%", g.zoom*100)
	padX, padY := int(10*g.deviceScale), int(6*g.deviceScale)
	margin := int(12 * g.deviceScale)
	textW := font.MeasureString(face, label).Ceil()
	textH := (face.Metrics().Ascent + face.Metrics().Descent).Ceil()
	w, h := textW+2*padX, textH+2*padY

	x := g.width - margin - w
	y := g.toolbarHeight + margin
	r := image.Rect(x, y, x+w, y+h)
	canvas.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{0x20, 0x20, 0x20, 0xFF})
	canvas.DrawText(label, face, r.Min.X+padX, baselineIn(face, r), color.RGBA{0xE0, 0xE0, 0xE0, 0xFF})
}

// drawScrollbar draws a thumb indicating the current scroll position,
// derived entirely from View.DocumentBounds/VisibleViewBounds - proof
// that those two methods are enough to build a scrollbar from outside
// the library, not just plausible on paper. Both are themselves only
// estimates until the whole document's been visited (see
// DocumentBounds), so only the ratio between them is meaningful here,
// scaled to the real track height - skipped entirely when there's
// nothing to scroll (the whole document, as currently estimated,
// already fits).
func (g *game) drawScrollbar(canvas whynot.Canvas) {
	trackHeight := g.height - g.toolbarHeight
	doc := g.current.view.DocumentBounds()
	visible := g.current.view.VisibleViewBounds(image.Pt(g.width, trackHeight))
	if doc.Dy() == 0 || visible.Dy() >= doc.Dy() {
		return
	}

	width := int(6 * g.deviceScale)
	minHeight := int(20 * g.deviceScale)

	y := g.toolbarHeight + visible.Min.Y*trackHeight/doc.Dy()
	height := visible.Dy() * trackHeight / doc.Dy()
	if height < minHeight {
		height = minHeight
	}
	if y+height > g.height {
		y = g.height - height
	}

	canvas.DrawRect(g.width-width, y, width, height, color.RGBA{0x80, 0x80, 0x80, 0xA0})
}

// drawButton draws an icon button, filling the whole of r - no border,
// no margin around the icon, just a background fill (for hover/pressed
// feedback) showing through wherever the icon's own transparency lets
// it. enabled only affects appearance; back is still harmless to click
// with no history, so nothing needs disabling functionally (see
// buttonColors).
func drawButton(dst *ebiten.Image, canvas whynot.Canvas, iconImg *ebiten.Image, r image.Rectangle, enabled bool, st buttonState) {
	fillColor, tint := buttonColors(enabled, st)
	if fillColor != nil {
		canvas.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), fillColor)
	}
	drawIcon(dst, iconImg, r, tint)
}

// drawIcon draws icon scaled to fill r exactly (preserving aspect
// ratio, icons are square anyway), tinted to clr - icon is expected to
// be a light, mostly-white silhouette on transparency, the same
// assumption ebiten's multiplicative ColorScale tinting relies on
// elsewhere (e.g compositing a hovered link's glyphs).
func drawIcon(dst, iconImg *ebiten.Image, r image.Rectangle, clr color.Color) {
	b := iconImg.Bounds()
	scale := float64(r.Dy()) / float64(b.Dy())
	if s := float64(r.Dx()) / float64(b.Dx()); s < scale {
		scale = s
	}
	w, h := float64(b.Dx())*scale, float64(b.Dy())*scale

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate(float64(r.Min.X)+(float64(r.Dx())-w)/2, float64(r.Min.Y)+(float64(r.Dy())-h)/2)
	opts.ColorScale.ScaleWithColor(clr)
	dst.DrawImage(iconImg, opts)
}

// buttonColors picks a button's fill/icon-tint colors for its current
// state - a disabled button ignores hover/pressed entirely, reading as
// inert regardless of where the cursor is. fillColor is nil for "no
// fill", i.e. the toolbar's own background shows through.
func buttonColors(enabled bool, st buttonState) (fillColor, iconTint color.Color) {
	switch {
	case !enabled:
		return nil, color.RGBA{0x60, 0x60, 0x60, 0xFF}
	case st.pressed:
		return color.RGBA{0x50, 0x50, 0x50, 0xFF}, color.White
	case st.hover:
		return color.RGBA{0x30, 0x30, 0x30, 0xFF}, color.RGBA{0xF0, 0xF0, 0xF0, 0xFF}
	default:
		return nil, color.RGBA{0xE0, 0xE0, 0xE0, 0xFF}
	}
}

// baselineIn returns the y coordinate DrawText needs to vertically
// center one line of face-set text within r.
func baselineIn(face font.Face, r image.Rectangle) int {
	m := face.Metrics()
	return r.Min.Y + (r.Dy()+m.Ascent.Ceil()-m.Descent.Ceil())/2
}

// drawOutline draws a thin border around r - Canvas has no dedicated
// outline primitive, so this is 4 filled rects along the edges.
func drawOutline(dst whynot.Canvas, r image.Rectangle, clr color.Color) {
	const thickness = 2
	dst.DrawRect(r.Min.X, r.Min.Y, r.Dx(), thickness, clr)
	dst.DrawRect(r.Min.X, r.Max.Y-thickness, r.Dx(), thickness, clr)
	dst.DrawRect(r.Min.X, r.Min.Y, thickness, r.Dy(), clr)
	dst.DrawRect(r.Max.X-thickness, r.Min.Y, thickness, r.Dy(), clr)
}
