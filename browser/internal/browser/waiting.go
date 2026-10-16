package browser

import (
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// waitAfter is how long a page that's still arriving can go without
// growing before the waiting dots show (see drawWaiting): a page that
// streams in steadily never shows them, and one waiting for an AI to
// start does.
const waitAfter = time.Second

// arrivals records when each document last grew, as watch sees it.
type arrivals struct {
	mu   sync.Mutex
	last map[*whynot.Document]time.Time
}

func (r *arrivals) note(doc *whynot.Document) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last == nil {
		r.last = map[*whynot.Document]time.Time{}
	}
	r.last[doc] = time.Now()
}

// since returns how long ago doc last grew, and whether it's known.
func (r *arrivals) since(doc *whynot.Document) (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.last[doc]
	return time.Since(t), ok
}

// forget drops doc, once it's complete.
func (r *arrivals) forget(doc *whynot.Document) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.last, doc)
}

// waiting reports whether the waiting dots show: the current page is still
// arriving, and hasn't grown for waitAfter.
func (a *App) waiting() bool {
	if a.overlaid != nil {
		return false
	}
	doc := a.Panel.View().Document()
	if doc.Complete() {
		return false
	}
	since, ok := a.arrivals.since(doc)
	return ok && since >= waitAfter
}

// drawWaiting draws the waiting dots, if they show: three dots that
// brighten in turn, as chat apps show someone typing, centred under the
// current page's content so far, or at the bottom of its View if that
// content goes on below.
func (a *App) drawWaiting(dst canvas.Canvas, now time.Duration) {
	if !a.waiting() {
		return
	}
	view := a.Panel.View()
	scale := a.deviceScale * a.zoom
	size := max(2, int(math.Round(7*scale)))
	gap := size
	height := size * 3
	// bounds is where the dots can go, and y where the content ends.
	bounds := view.Bounds()
	y := view.DocumentBounds(height).Max.Y
	if j := a.stacked(); j != nil {
		bounds = j.stack.bounds
		if last := len(j.stack.views) - 1; j.stack.regions[last].Empty() {
			y = bounds.Max.Y // the page is below what's shown
		}
	}
	if limit := bounds.Max.Y - height; y > limit {
		y = limit
		// Over the content: on the page's background.
		background, _ := a.stackColors()
		width := 3*size + 4*gap
		dst.DrawRect((bounds.Min.X+bounds.Max.X-width)/2, y, width, height, background)
	}
	if y < bounds.Min.Y {
		return
	}
	theme := simpletheme.Light()
	if a.darkTheme {
		theme = simpletheme.Dark()
	}
	x := (bounds.Min.X+bounds.Max.X)/2 - (3*size+2*gap)/2
	cy := y + (height-size)/2
	const period = 1.2 // seconds, for the three dots
	for i := range 3 {
		// Each dot's brightness peaks in turn.
		phase := now.Seconds()/period - float64(i)/3
		level := 0.25 + 0.75*math.Pow(math.Max(0, math.Sin(2*math.Pi*phase)), 2)
		dst.DrawImage(dot(theme.TextColor, level), x+i*(size+gap), cy, size, size)
	}
}

// dotLevels is how many brightness steps the dots have, each image made
// once.
const dotLevels = 16

type dotKey struct {
	clr   color.Color
	level int
}

var (
	dotsMu sync.Mutex
	dots   = map[dotKey]image.Image{}
)

// dot returns a round dot of clr at brightness level, from 0 to 1, as an
// image to scale.
func dot(clr color.Color, level float64) image.Image {
	key := dotKey{clr, int(math.Round(level * (dotLevels - 1)))}
	dotsMu.Lock()
	defer dotsMu.Unlock()
	if img, ok := dots[key]; ok {
		return img
	}
	const n = 32
	r, g, b, _ := color.NRGBAModel.Convert(clr).RGBA()
	alpha := float64(key.level) / (dotLevels - 1)
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for py := range n {
		for px := range n {
			d := math.Hypot(float64(px)+0.5-n/2, float64(py)+0.5-n/2)
			a := math.Max(0, math.Min(1, n/2-d)) * alpha
			img.SetNRGBA(px, py, color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a * 255)})
		}
	}
	dots[key] = img
	return img
}
