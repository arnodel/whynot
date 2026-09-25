package ebitenrenderer

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Update reads this tick's input (touch if active, else mouse) and
// drives scroll/hover/click - call once per game tick. A touch drag
// keeps scrolling after release, decaying like native touch scrolling,
// until a new drag or a mouse wheel/click cancels it.
func (p *Panel) Update() {
	now := time.Now()

	if cx, cy, scrollDelta, down, justPressed, ok := p.touchInput(); ok {
		p.update(cx, cy, scrollDelta, down, justPressed)
		switch {
		case justPressed, p.draggingScrollbar, !image.Pt(cx, cy).In(p.bounds):
			p.interaction.CancelMomentum()
		default:
			p.interaction.AccumulateMomentum(scrollDelta, now)
		}
		return
	}

	cx, cy := ebiten.CursorPosition()
	_, wheelDy := ebiten.Wheel()
	mouseDown := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	justPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)

	if wheelDy != 0 || justPressed {
		p.interaction.CancelMomentum()
	} else {
		p.interaction.Momentum(now)
	}
	p.update(cx, cy, wheelDy*p.scale*2, mouseDown, justPressed)
}

// update takes this tick's input as parameters rather than reading
// ebiten itself, so it's testable without a live ebiten context.
// scrollDelta is in Scroll's own units, ready to pass straight through.
// Delegates scroll/hover/click to whynot.Interaction (shared with
// giorenderer.Panel) - only the scrollbar-drag check stays here, since
// each backend represents its scrollbar too differently to share.
func (p *Panel) update(cx, cy int, scrollDelta float64, pointerDown, justPressed bool) {
	p.interaction.View = p.view
	p.interaction.Bounds = p.bounds
	p.interaction.OnLinkClick = p.OnLinkClick
	p.interaction.OnLinkHover = p.OnLinkHover
	p.interaction.AnchorScrolling = p.anchorScrolling

	p.interaction.Scroll(cx, cy, scrollDelta)

	if p.scrollbarEnabled && p.updateScrollbarDrag(cx, cy, pointerDown, justPressed) {
		// The drag owns this frame - don't also treat it as a document
		// hover/click underneath it.
		return
	}
	p.interaction.HoverAndClick(cx, cy, justPressed)
}

// touchInput is Update's touch equivalent of reading mouse state - ok
// is false when there's no touch, so Update falls back to the mouse.
// Tracks at most one touch, ignoring any second simultaneous one.
// scrollDelta follows "content follows your finger": dragging down is
// positive, reporting 0 on a touch's first tick since there's no
// previous position yet to diff against.
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
// scrollbar thumb, reporting whether it consumed this frame's
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
