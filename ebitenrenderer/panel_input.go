package ebitenrenderer

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Update reads ebiten's own input state (cursor position, wheel,
// mouse button) directly - call once per game tick.
func (p *Panel) Update() {
	cx, cy := ebiten.CursorPosition()
	_, wheelDy := ebiten.Wheel()
	mouseDown := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	justPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	p.update(cx, cy, wheelDy, mouseDown, justPressed)
}

// update is Update's actual logic, taking this tick's input as
// parameters rather than reading ebiten itself - so it's reachable
// from a plain Go test without a live ebiten context.
func (p *Panel) update(cx, cy int, wheelDy float64, mouseDown, justPressed bool) {
	cursor := image.Pt(cx, cy)

	// Gated on bounds - a panel embedded in a larger game must not eat
	// wheel events meant for whatever's around it (cmd/whynot never had
	// to think about this; its View occupies basically the whole window).
	if cursor.In(p.bounds) {
		p.view.Scroll(wheelDy * p.scale * 2)
	}

	if p.scrollbarEnabled && p.updateScrollbarDrag(cx, cy, mouseDown, justPressed) {
		// The drag owns this frame - don't also treat it as a document
		// hover/click underneath it.
		return
	}

	var dest string
	var hasLink bool
	if cursor.In(p.bounds) {
		dest, hasLink = p.view.Hover(cx-p.bounds.Min.X, cy-p.bounds.Min.Y)
	} else {
		// (-1, -1) can't land on anything - only ever clears a
		// highlight left over from moving off a link while still
		// inside bounds.
		p.view.Hover(-1, -1)
	}

	newHoverDest := ""
	if hasLink {
		newHoverDest = dest
	}
	if p.OnLinkHover != nil && newHoverDest != p.hoverDest {
		p.OnLinkHover(newHoverDest)
	}
	p.hoverDest = newHoverDest

	if hasLink && justPressed {
		if p.anchorScrolling && strings.HasPrefix(dest, "#") {
			p.view.ScrollToAnchor(strings.TrimPrefix(dest, "#"))
		} else if p.OnLinkClick != nil {
			p.OnLinkClick(dest)
		}
	}
}

// updateScrollbarDrag handles pressing, dragging, and releasing the
// scrollbar thumb, reporting whether it consumed this frame's mouse
// input. Takes this tick's raw input as parameters for the same
// testability reason as update. The target ratio is recomputed from
// the cursor's current position every call, not a value captured once
// at drag start, so a jump into not-yet-resolved territory (see
// View.ScrollToRatio) only ever corrects toward the cursor, never
// drifts from it. Likewise, the thumb rect is re-fetched every call
// (not just at drag start) so scrollbarGrabRatio is always applied to
// the thumb's *current* height.
func (p *Panel) updateScrollbarDrag(cx, cy int, mouseDown, justPressed bool) bool {
	r, ok := p.scrollbarThumbRect()
	hovering := ok && (image.Point{X: cx, Y: cy}).In(r)
	defer func() {
		p.scrollbarState = buttonState{hover: hovering || p.draggingScrollbar, pressed: p.draggingScrollbar}
	}()

	if !mouseDown {
		p.draggingScrollbar = false
		return false
	}

	if !p.draggingScrollbar {
		if !justPressed || !hovering {
			return false
		}
		p.draggingScrollbar = true
		p.scrollbarGrabRatio = float64(cy-r.Min.Y) / float64(r.Dy())
	}

	trackHeight := p.bounds.Dy()
	if !ok || trackHeight <= 0 {
		return true
	}
	target := float64(cy) - p.scrollbarGrabRatio*float64(r.Dy())
	p.view.ScrollToRatio((target - float64(p.bounds.Min.Y)) / float64(trackHeight))
	return true
}
