package browser

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
)

// zoomIndicatorDuration is how long the "N%" popup stays up after a
// zoom change - long enough to read, short enough to get out of the
// way on its own.
const zoomIndicatorDuration = 1500 * time.Millisecond

// DrawZoomIndicator shows the current zoom level for a little while
// after it changes (see SetZoom), in the document area's top-right
// corner - not the toolbar's, since zoom is a document-only setting.
// face is the caller's own chrome font, sized independently of the
// document's own zoomed scale (the indicator's own size shouldn't zoom
// with the document).
func (a *App) DrawZoomIndicator(canvas whynot.Canvas, face font.Face) {
	if !time.Now().Before(a.zoomIndicatorUntil) {
		return
	}
	label := fmt.Sprintf("%.0f%%", a.zoom*100)
	padX, padY := int(10*a.deviceScale), int(6*a.deviceScale)
	margin := int(12 * a.deviceScale)
	textW := font.MeasureString(face, label).Ceil()
	textH := (face.Metrics().Ascent + face.Metrics().Descent).Ceil()
	w, h := textW+2*padX, textH+2*padY

	x := a.width - margin - w
	y := a.toolbarHeight + margin
	r := image.Rect(x, y, x+w, y+h)
	canvas.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{0x20, 0x20, 0x20, 0xFF})
	canvas.DrawText(label, face, r.Min.X+padX, baselineIn(face, r), color.RGBA{0xE0, 0xE0, 0xE0, 0xFF})
}

// DrawDebugStats shows fps/tps alongside the most recent Update/Draw
// call durations, in the document area's top-left corner - the caller
// gates whether to call this at all (e.g. cmd/whynot's -debug-stats/F
// toggle) and supplies its own backend's performance counters, which
// this package has no way to read itself.
func (a *App) DrawDebugStats(canvas whynot.Canvas, face font.Face, fps, tps float64, updateDuration, drawDuration time.Duration) {
	lines := [2]string{
		fmt.Sprintf("%.0f fps  %.0f tps", fps, tps),
		fmt.Sprintf("upd %.2fms  draw %.2fms", updateDuration.Seconds()*1000, drawDuration.Seconds()*1000),
	}

	padX, padY := int(10*a.deviceScale), int(6*a.deviceScale)
	margin := int(12 * a.deviceScale)
	lineH := (face.Metrics().Ascent + face.Metrics().Descent).Ceil()

	textW := 0
	for _, l := range lines {
		if w := font.MeasureString(face, l).Ceil(); w > textW {
			textW = w
		}
	}
	w, h := textW+2*padX, lineH*len(lines)+2*padY

	x := margin
	y := a.toolbarHeight + margin
	r := image.Rect(x, y, x+w, y+h)
	canvas.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{0x20, 0x20, 0x20, 0xFF})
	baseline := r.Min.Y + padY + face.Metrics().Ascent.Ceil()
	for i, l := range lines {
		canvas.DrawText(l, face, r.Min.X+padX, baseline+i*lineH, color.RGBA{0xE0, 0xE0, 0xE0, 0xFF})
	}
}

// baselineIn returns the y coordinate DrawText needs to vertically
// center one line of face-set text within r.
func baselineIn(face font.Face, r image.Rectangle) int {
	m := face.Metrics()
	return r.Min.Y + (r.Dy()+m.Ascent.Ceil()-m.Descent.Ceil())/2
}
