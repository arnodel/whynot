package whynot

import (
	"image"
	"time"
)

// layoutView places v at the canvas origin, width by height, at scale,
// and does a frame's work at now without drawing.
func layoutView(v *View, width, height int, scale float64, now time.Duration) {
	if v.zoom == 0 {
		v.zoom = 1 // a View built as a literal in a test, rather than by NewView
	}
	v.SetScale(scale)
	v.SetBounds(image.Rect(0, 0, width, height))
	v.update(now)
}

// stackBounds is the laid-out document's estimated extent, in pixels
// from its top: v's width by the estimated total height.
func stackBounds(v *View) image.Rectangle {
	if !v.stack.laidOut() {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, v.width, int(v.stack.totalHeight()))
}

// stackVisibleBounds is the part of stackBounds in view, for a viewport
// of viewportSize.
func stackVisibleBounds(v *View, viewportSize image.Point) image.Rectangle {
	if !v.stack.laidOut() || v.stack.cursor.Index >= v.stack.len() {
		return image.Rectangle{}
	}
	top, bottom, _ := v.stack.visibleRange(viewportSize.Y)
	return image.Rect(0, int(top), viewportSize.X, int(bottom))
}
