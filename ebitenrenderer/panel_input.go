package ebitenrenderer

import (
	"image"

	"github.com/arnodel/whynot/input"
)

// Update reads this tick's input and passes it to the Panel's Controller
// - call once per game tick.
func (p *Panel) Update() {
	p.input.Scale = p.scale
	p.apply(p.input.Events())
}

// apply passes events to the Controller, after the Panel's own scrollbar
// has had the pointer: while it's dragged, only wheel events go through.
func (p *Panel) apply(events []input.Event) {
	p.syncController()
	cx, cy, down, justPressed := p.followPointer(events)
	if p.scrollbarEnabled && p.updateScrollbarDrag(cx, cy, down, justPressed) {
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

// followPointer updates the pointer's position and state, mouse or
// touch, from events, for the Panel's own scrollbar: justPressed is
// whether it went down during them.
func (p *Panel) followPointer(events []input.Event) (x, y int, down, justPressed bool) {
	for _, e := range events {
		switch e := e.(type) {
		case input.PointerMove:
			p.pointer = image.Pt(e.X, e.Y)
		case input.PointerButton:
			if e.Button != input.ButtonPrimary {
				continue
			}
			p.pointer = image.Pt(e.X, e.Y)
			justPressed = justPressed || e.Down && !p.pointerDown
			p.pointerDown = e.Down
		case input.TouchStart:
			p.pointer = image.Pt(e.X, e.Y)
			justPressed = justPressed || !p.pointerDown
			p.pointerDown = true
		case input.TouchMove:
			p.pointer = image.Pt(e.X, e.Y)
		case input.TouchEnd, input.TouchCancel:
			p.pointerDown = false
		}
	}
	return p.pointer.X, p.pointer.Y, p.pointerDown, justPressed
}

// syncController copies the Panel's link callbacks, which a host may
// reassign at any time, into its Controller.
func (p *Panel) syncController() {
	p.controller.OnLinkClick = p.OnLinkClick
	p.controller.OnLinkHover = p.OnLinkHover
	p.controller.AnchorScrolling = p.anchorScrolling
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
