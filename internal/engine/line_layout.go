package engine

import (
	"image"
	"time"

	"github.com/arnodel/whynot/canvas"
)

// LineBox is one line of inline content, each part at a precomputed x.
// Parts are placed once, by lineBuilder, and drawing, hit-testing and
// bounds all read those positions, so what's measured is what's drawn.
type LineBox struct {
	parts []InlineLayout
	// xs holds each part's x - where its DrawInline pen starts - relative
	// to the line's left edge.
	xs []int
	// bounds is the union of every part's bounds at its x, relative to
	// the baseline.
	bounds image.Rectangle
}

var _ BlockLayout = (*LineBox)(nil)

// NewLineBox places parts on a single line, never wrapping. glue abuts
// parts with no gap between them - for parts that are already contiguous
// slices of one string, whitespace included (CodeBlock's token spans).
func NewLineBox(parts []InlineLayout, glue bool) *LineBox {
	l := lineBuilder{glue: glue}
	for _, part := range parts {
		l.add(part)
	}
	return l.line()
}

// wrapLines breaks parts into lines no wider than width. A part Glued() to
// the one before it is never a break point: like a single oversized word,
// a glued run stays on one line even if it overflows width.
func wrapLines(parts []InlineLayout, width int) []BlockLayout {
	var lines []BlockLayout
	var l lineBuilder
	for _, part := range parts {
		if len(l.parts) > 0 && !part.Glued() {
			// Dx(), not Max.X: a part's own bounds can pull the line's
			// Min.X away from 0 (e.g. its left bearing), and Max.X alone
			// would then overstate the line's width.
			if _, bounds := l.fit(part); bounds.Dx() > width {
				lines = append(lines, l.line())
				l = lineBuilder{}
			}
		}
		l.add(part)
	}
	if len(l.parts) > 0 {
		lines = append(lines, l.line())
	}
	return lines
}

// lineBuilder places parts left to right on one line - the single place
// inter-part spacing is decided.
type lineBuilder struct {
	glue bool

	parts  []InlineLayout
	xs     []int
	bounds image.Rectangle

	// pen is where the last part's advance ended, and prevSpace that
	// part's SpaceWidth.
	pen       int
	prevSpace int
}

// fit reports where part would go if added next, and the line's bounds
// with it.
func (l *lineBuilder) fit(part InlineLayout) (x int, bounds image.Rectangle) {
	partBounds, _ := part.BoundsAndAdvance()
	if len(l.parts) == 0 {
		// A first part whose ink overshoots its origin to the left
		// (a negative left bearing) is shifted right to start at 0.
		x = max(0, -partBounds.Min.X)
		return x, partBounds.Add(image.Pt(x, 0))
	}
	x = l.pen + l.gap(part)
	return x, l.bounds.Union(partBounds.Add(image.Pt(x, 0)))
}

// gap is the space inserted before part: the wider of the two neighbors'
// space widths, since the space between two differently-sized fonts could
// be either one - unless the whole line is glued, or part itself is (no
// source whitespace before it; see InlineLayout.Glued).
func (l *lineBuilder) gap(part InlineLayout) int {
	if l.glue || part.Glued() {
		return 0
	}
	return max(l.prevSpace, part.SpaceWidth())
}

func (l *lineBuilder) add(part InlineLayout) {
	x, bounds := l.fit(part)
	_, advance := part.BoundsAndAdvance()
	l.parts = append(l.parts, part)
	l.xs = append(l.xs, x)
	l.bounds = bounds
	l.pen = x + advance
	l.prevSpace = part.SpaceWidth()
}

func (l *lineBuilder) line() *LineBox {
	return &LineBox{parts: l.parts, xs: l.xs, bounds: l.bounds}
}

// Source is always nil: a line can mix parts with different identities
// (e.g. plain text next to emphasized text); HitTest returns the specific
// part that matched instead.
func (b *LineBox) Source() Source {
	return nil
}

// Bounds only compensates for a negative bounds.Min.X, not a positive one:
// a code line's leading indentation has advance but no ink, so it's
// occupied space starting at the line's x=0, not excess to crop away.
func (b *LineBox) Bounds() image.Rectangle {
	left := min(b.bounds.Min.X, 0)
	return image.Rect(0, 0, b.bounds.Max.X-left, b.bounds.Dy())
}

func (b *LineBox) drawContents(dst canvas.Canvas, x, y int, now time.Duration) {
	y -= b.bounds.Min.Y
	for i, part := range b.parts {
		part.DrawInline(dst, x+b.xs[i], y, now)
	}
}

// HitTest checks each part where drawContents draws it for external y=0,
// which lands a part's footprint in Bounds()'s frame - the frame p is in.
func (b *LineBox) HitTest(p image.Point) (Hit, image.Point) {
	y := -b.bounds.Min.Y
	for i, part := range b.parts {
		if hit, offset := part.HitTest(p, b.xs[i], y); hit != nil {
			return hit, offset
		}
	}
	return nil, image.Point{}
}

func (b *LineBox) PendingImages() []string {
	var pending []string
	for _, part := range b.parts {
		pending = append(pending, part.PendingImages()...)
	}
	return pending
}
