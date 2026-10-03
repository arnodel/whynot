package whynot

import (
	"image"
	"time"
)

// layoutView places v at the canvas origin, width by height, at scale,
// and does a frame's work at now without drawing.
func layoutView(v *View, width, height int, scale float64, now time.Duration) {
	v.SetScale(scale)
	v.SetBounds(image.Rect(0, 0, width, height))
	v.update(now)
}

// documentBounds is the document's estimated extent in document
// coordinates: v's width by the estimated total height.
func documentBounds(v *View) image.Rectangle {
	if !v.stack.laidOut() {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, v.width, int(v.stack.totalHeight()))
}

// visibleViewBounds is the part of documentBounds in view, for a
// viewport of viewportSize.
func visibleViewBounds(v *View, viewportSize image.Point) image.Rectangle {
	if !v.stack.laidOut() || v.stack.cursor.Index >= v.stack.len() {
		return image.Rectangle{}
	}
	top, bottom, _ := v.stack.visibleRange(viewportSize.Y)
	return image.Rect(0, int(top), viewportSize.X, int(bottom))
}
