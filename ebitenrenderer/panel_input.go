package ebitenrenderer

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Update reads this tick's input and drives scroll/hover/click from
// it - call once per game tick. A touch takes priority over mouse
// state when one is active (see touchInput) - not additive with it,
// since a browser can synthesize compatibility mouse events from a
// touch and handling both would double up.
func (p *Panel) Update() {
	if cx, cy, scrollDelta, down, justPressed, ok := p.touchInput(); ok {
		p.update(cx, cy, scrollDelta, down, justPressed)
		return
	}
	cx, cy := ebiten.CursorPosition()
	_, wheelDy := ebiten.Wheel()
	mouseDown := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	justPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	// wheelDy is a small notch count, not a pixel amount - unlike
	// touchInput's scrollDelta, which is already in the same
	// device-pixel space Scroll expects (see PageDown/PageUp's own
	// comment on that space), so only the mouse path needs this
	// conversion.
	p.update(cx, cy, wheelDy*p.scale*2, mouseDown, justPressed)
}

// update is Update's actual logic, taking this tick's input as
// parameters rather than reading ebiten itself - so it's reachable
// from a plain Go test without a live ebiten context. scrollDelta is
// already in Scroll's own device-pixel units, ready to pass straight
// through - converting from whatever unit the input source (wheel
// notches, a touch drag) actually reports is the caller's job.
func (p *Panel) update(cx, cy int, scrollDelta float64, pointerDown, justPressed bool) {
	cursor := image.Pt(cx, cy)

	// Gated on bounds - a panel embedded in a larger game must not eat
	// scroll input meant for whatever's around it (cmd/whynot never had
	// to think about this; its View occupies basically the whole window).
	if cursor.In(p.bounds) {
		p.view.Scroll(scrollDelta)
	}

	if p.scrollbarEnabled && p.updateScrollbarDrag(cx, cy, pointerDown, justPressed) {
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

// touchInput computes this tick's (cx, cy, scrollDelta, down,
// justPressed) from ebiten's touch state, the same five values Update's
// mouse path reads - so a touch drives the exact same hover/click/
// scroll logic a mouse would, through one shared update, rather than a
// separate touch-handling pipeline. ok is false when there's no touch
// to report (Update falls back to mouse then).
//
// Tracks at most one touch (Panel.trackingTouch/activeTouch) -
// deliberately simple, ignoring any second simultaneous touch rather
// than trying to support a pinch/multi-touch gesture Panel has no use
// for yet.
//
// scrollDelta is "content follows your finger" (the standard modern
// touchscreen convention): dragging down feeds a positive scrollDelta,
// same sign View.Scroll already gives a positive value (move toward
// the document's start) - matching what a finger physically dragging
// content down the screen should reveal (earlier content). The very
// first tick a touch is tracked reports scrollDelta 0 rather than
// diffing against inpututil.TouchPositionInPreviousTick's zero-value
// default for a not-yet-tracked ID, which would otherwise read as a
// spurious jump from (0,0).
func (p *Panel) touchInput() (cx, cy int, scrollDelta float64, down, justPressed bool, ok bool) {
	if p.trackingTouch {
		for _, id := range ebiten.AppendTouchIDs(nil) {
			if id == p.activeTouch {
				x, y := ebiten.TouchPosition(id)
				_, py := inpututil.TouchPositionInPreviousTick(id)
				return x, y, float64(y - py), true, false, true
			}
		}
		// The touch we were tracking ended - fall through to look for a
		// different one already active this same tick.
		p.trackingTouch = false
	}

	ids := ebiten.AppendTouchIDs(nil)
	if len(ids) == 0 {
		return 0, 0, 0, false, false, false
	}
	p.activeTouch = ids[0]
	p.trackingTouch = true
	x, y := ebiten.TouchPosition(p.activeTouch)
	return x, y, 0, true, true, true
}

// updateScrollbarDrag handles pressing, dragging, and releasing the
// scrollbar thumb - a mouse button or a touch, either drives it the
// same way (see update) - reporting whether it consumed this frame's
// input. Takes this tick's raw input as parameters for the same
// testability reason as update. The target ratio is recomputed from
// the cursor's current position every call, not a value captured once
// at drag start, so a jump into not-yet-resolved territory (see
// View.ScrollToRatio) only ever corrects toward the cursor, never
// drifts from it. Likewise, the thumb rect is re-fetched every call
// (not just at drag start) so scrollbarGrabRatio is always applied to
// the thumb's *current* height.
func (p *Panel) updateScrollbarDrag(cx, cy int, pointerDown, justPressed bool) bool {
	r, ok := p.scrollbarThumbRect()
	hovering := ok && (image.Point{X: cx, Y: cy}).In(r)
	defer func() {
		p.scrollbarState = buttonState{hover: hovering || p.draggingScrollbar, pressed: p.draggingScrollbar}
	}()

	if !pointerDown {
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
