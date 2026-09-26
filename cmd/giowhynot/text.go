package main

import (
	"image"

	"golang.org/x/image/font"
)

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

// baselineIn returns the y coordinate DrawText needs to vertically
// center one line of face-set text within r.
func baselineIn(face font.Face, r image.Rectangle) int {
	m := face.Metrics()
	return r.Min.Y + (r.Dy()+m.Ascent.Ceil()-m.Descent.Ceil())/2
}
