package ebitenrenderer

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/arnodel/whynot/input"
)

// wheelPixels is how far one unit of ebiten.Wheel scrolls, in logical
// pixels.
const wheelPixels = 2

// Update reads this tick's input (touch if active, else mouse) and
// passes it to the Panel's Controller - call once per game tick.
func (p *Panel) Update() {
	if cx, cy, justPressed, ok := p.touchInput(); ok {
		p.touching = true
		var e input.Event = input.TouchMove{ID: int(p.activeTouch), X: cx, Y: cy}
		if justPressed {
			e = input.TouchStart{ID: int(p.activeTouch), X: cx, Y: cy}
		}
		p.apply([]input.Event{e}, cx, cy, true, justPressed)
		return
	}
	var events []input.Event
	if p.touching {
		p.touching = false
		events = append(events, input.TouchEnd{ID: int(p.activeTouch)})
	}

	cx, cy := ebiten.CursorPosition()
	wx, wy := ebiten.Wheel()
	st := mouseInput{x: cx, y: cy, down: ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft), wheelX: wx, wheelY: wy, mods: modifiers()}
	events = append(events, p.mouseEvents(st)...)
	p.apply(events, cx, cy, st.down, inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft))
}

// mouseInput is one tick's mouse state.
type mouseInput struct {
	x, y           int
	down           bool // the primary button
	wheelX, wheelY float64
	mods           input.Modifiers
}

// mouseEvents reports what changed in st since the last tick, as events.
func (p *Panel) mouseEvents(st mouseInput) []input.Event {
	var events []input.Event
	pos := image.Pt(st.x, st.y)
	if !p.hasCursor || pos != p.cursor {
		events = append(events, input.PointerMove{X: st.x, Y: st.y})
		p.cursor, p.hasCursor = pos, true
	}
	if st.down != p.mouseDown {
		events = append(events, input.PointerButton{X: st.x, Y: st.y, Button: input.ButtonPrimary, Down: st.down, Mods: st.mods})
		p.mouseDown = st.down
	}
	if st.wheelX != 0 || st.wheelY != 0 {
		// ebiten.Wheel is positive up and left; Wheel events are
		// positive towards the end, in pixels.
		k := -wheelPixels * p.scale
		events = append(events, input.Wheel{X: st.x, Y: st.y, DX: st.wheelX * k, DY: st.wheelY * k, Mods: st.mods})
	}
	return events
}

// modifiers reports the modifier keys held now.
func modifiers() input.Modifiers {
	var m input.Modifiers
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		m |= input.ModShift
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) {
		m |= input.ModCtrl
	}
	if ebiten.IsKeyPressed(ebiten.KeyAlt) {
		m |= input.ModAlt
	}
	if ebiten.IsKeyPressed(ebiten.KeyMeta) {
		m |= input.ModMeta
	}
	return m
}

// apply passes events to the Controller, after the Panel's own scrollbar
// has had the pointer, at (cx, cy) with the given button state: while
// it's dragged, only wheel events go through.
func (p *Panel) apply(events []input.Event, cx, cy int, pointerDown, justPressed bool) {
	p.syncController()
	if p.scrollbarEnabled && p.updateScrollbarDrag(cx, cy, pointerDown, justPressed) {
		var kept []input.Event
		for _, e := range events {
			if _, ok := e.(input.Wheel); ok {
				kept = append(kept, e)
			}
		}
		events = kept
		p.controller.CancelMomentum()
	}
	p.controller.Frame(events, p.elapsed())
}

// syncController copies the Panel's link callbacks, which a host may
// reassign at any time, into its Controller.
func (p *Panel) syncController() {
	p.controller.OnLinkClick = p.OnLinkClick
	p.controller.OnLinkHover = p.OnLinkHover
	p.controller.AnchorScrolling = p.anchorScrolling
}

// touchInput reports the touch being tracked - ok is false when there's
// none, so Update falls back to the mouse. Tracks at most one touch,
// ignoring any second simultaneous one.
func (p *Panel) touchInput() (cx, cy int, justPressed bool, ok bool) {
	if p.trackingTouch {
		for _, id := range ebiten.AppendTouchIDs(nil) {
			if id == p.activeTouch {
				x, y := ebiten.TouchPosition(id)
				return x, y, false, true
			}
		}
		// The touch we were tracking ended - fall through to look for a
		// different one already active this same tick.
		p.trackingTouch = false
	}

	ids := ebiten.AppendTouchIDs(nil)
	if len(ids) == 0 {
		return 0, 0, false, false
	}
	p.activeTouch = ids[0]
	p.trackingTouch = true
	x, y := ebiten.TouchPosition(p.activeTouch)
	return x, y, true, true
}

// updateScrollbarDrag handles pressing, dragging, and releasing the
// scrollbar thumb, reporting whether it consumed this frame's input. The
// target is recomputed from the cursor every call, so a jump into
// not-yet-laid-out territory (see View.ScrollToRatio) only ever corrects
// toward the cursor.
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
