package main

import (
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
)

func (g *game) Draw(screen *ebiten.Image) {
	start := time.Now()
	defer func() { g.drawDuration = time.Since(start) }()

	// panel fills its own background (from the View's StyleSheet),
	// draws the document, and its scrollbar - real SubImage-clipped to
	// its own bounds, so it can never bleed into the toolbar above it.
	g.panel.Draw(screen)

	canvas := g.renderer.NewCanvas(screen)
	g.drawToolbar(screen, canvas)
	if face, err := g.toolbarFaceSelector.SelectFace(whynot.TextStyle{Size: 14}); err == nil {
		g.app.DrawZoomIndicator(canvas, face)
		if g.debugStats {
			g.app.DrawDebugStats(canvas, face, ebiten.ActualFPS(), ebiten.ActualTPS(), g.updateDuration, g.drawDuration)
		}
	}

	if g.debugHit {
		cursor := image.Pt(g.hoverX, g.hoverY)
		if cursor.In(g.panel.Bounds()) {
			rel := cursor.Sub(g.panel.Bounds().Min)
			if hit, offset := g.panel.View().HitTest(rel.X, rel.Y); hit != nil {
				drawOutline(canvas, hit.Bounds().Add(offset).Add(g.panel.Bounds().Min), color.RGBA{255, 0, 0, 255})
			}
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

	drawButton(dst, canvas, backIcon, g.backButton, g.app.CanGoBack(), g.backState)
	drawButton(dst, canvas, forwardIcon, g.forwardButton, g.app.CanGoForward(), g.forwardState)
	drawButton(dst, canvas, reloadIcon, g.reloadButton, g.app.CanReload(), g.reloadState)
	drawButton(dst, canvas, tocIcon, g.tocButton, g.app.CanShowTOC(), g.tocState)
	drawButton(dst, canvas, zoomOutIcon, g.zoomOutButton, true, g.zoomOutState)
	drawButton(dst, canvas, zoomInIcon, g.zoomInButton, true, g.zoomInState)
	themeIcon := lightModeIcon
	if g.app.DarkTheme() {
		themeIcon = darkModeIcon
	}
	drawButton(dst, canvas, themeIcon, g.themeButton, true, g.themeState)

	face, err := g.toolbarFaceSelector.SelectFace(whynot.TextStyle{Size: 14})
	if err != nil {
		return
	}
	text, textColor := g.app.Location().String(), color.Color(color.RGBA{0xCC, 0xCC, 0xCC, 0xFF})
	if hoverDest := g.app.HoverDest(); hoverDest != "" {
		text, textColor = hoverDest, g.app.StyleSheet().HighlightColor()
	}
	x := g.tocButton.Max.X + int(16*g.deviceScale)
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
