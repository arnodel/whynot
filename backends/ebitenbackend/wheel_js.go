//go:build js

package ebitenbackend

// In a web browser, ebiten.Wheel reports the browser's wheel deltas,
// which are in CSS pixels: already logical pixels.
const wheelPixels = 1
