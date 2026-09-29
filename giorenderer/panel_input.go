package giorenderer

import (
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// Update reads this frame's pointer input and drives scroll/hover/click
// - call once per frame, before Draw. A touch drag keeps scrolling after
// release, decaying like native touch scrolling, until a new press or a
// mouse wheel cancels it. Gio unifies mouse and touch into
// one event stream (pointer.Event.Source tells them apart) - unlike
// ebitenrenderer.Panel, which has to poll two separate ebiten APIs (see
// its own touchInput) - and routes events by area, so a press on the
// scrollbar's own region (added after this Panel's own area - see Draw)
// never reaches here at all, with no manual bounds-vs-scrollbar check
// needed the way ebitenrenderer.Panel's draggingScrollbar has to do.
func (p *Panel) Update(gtx layout.Context) {
	p.interaction.View = p.view
	p.interaction.Bounds = p.bounds
	p.interaction.OnLinkClick = p.OnLinkClick
	p.interaction.OnLinkHover = p.OnLinkHover
	p.interaction.AnchorScrolling = p.anchorScrolling

	area := clip.Rect(p.bounds).Push(gtx.Ops)
	event.Op(gtx.Ops, p)
	area.Pop()

	now := time.Now()
	pos := image.Pt(-1, -1)
	justPressed := false
	mouseJustPressed := false
	gotEvent := false
	// dragDelta totals this frame's touch drag. Momentum is accumulated
	// once per frame from it, not per event: every event in a frame
	// shares the same now, so per event only the first would count.
	dragDelta := 0.0

	for {
		e, ok := gtx.Source.Event(pointer.Filter{
			Target:  p,
			Kinds:   pointer.Press | pointer.Release | pointer.Cancel | pointer.Move | pointer.Drag | pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			break
		}
		pe, ok := e.(pointer.Event)
		if !ok {
			continue
		}
		gotEvent = true
		pos = image.Pt(int(pe.Position.X), int(pe.Position.Y))

		switch pe.Kind {
		case pointer.Press:
			justPressed = true
			p.dragging = pe.Source == pointer.Touch
			p.lastDragPos = pos
			if pe.Source == pointer.Mouse {
				mouseJustPressed = true
				p.mouseDown = true
			}
		case pointer.Release, pointer.Cancel:
			p.dragging = false
			p.mouseDown = false
		case pointer.Drag:
			if p.dragging {
				// "Content follows your finger" - see whynot.Interaction's
				// own doc comment on AccumulateMomentum for the sign
				// convention this matches.
				delta := float64(pos.Y - p.lastDragPos.Y)
				p.lastDragPos = pos
				p.interaction.Scroll(pos.X, pos.Y, delta)
				dragDelta += delta
			}
		case pointer.Scroll:
			// Gio's Scroll.Y is positive scrolling down (confirmed
			// against gioui.org/layout.List's own use of it), the
			// opposite sign from View.Scroll's own convention (negative
			// moves down - see ScrollDown) - negated here, not inside
			// Interaction, since that's specific to Gio's own event
			// shape, not something ebitenrenderer's wheel path shares.
			sx, sy := pe.Scroll.X, pe.Scroll.Y
			if pe.Modifiers.Contain(key.ModShift) {
				// Shift+wheel scrolls sideways. Some platforms (macOS)
				// already report it as horizontal, leaving sy 0.
				sx, sy = sx+sy, 0
			}
			p.interaction.CancelMomentum()
			p.interaction.Scroll(pos.X, pos.Y, -float64(sy))
			p.interaction.ScrollHorizontal(pos.X, pos.Y, -float64(sx))
		}
	}

	if justPressed {
		p.interaction.CancelMomentum()
	}
	switch {
	case p.dragging || dragDelta != 0:
		// Also while the finger is held still (dragDelta 0), so the
		// velocity decays toward 0 before release rather than flinging.
		p.interaction.AccumulateMomentum(dragDelta, now)
	case !justPressed:
		p.interaction.Momentum(now)
	}
	// Gio only produces frames when something happens, so keep asking
	// for them while there's momentum to apply.
	if p.interaction.Moving() {
		gtx.Execute(op.InvalidateCmd{})
	}

	if justPressed && p.OnPress != nil {
		p.OnPress()
	}

	// A scrollbar drag owns the pointer: no hover or click underneath.
	if gotEvent && !p.interaction.DragHorizontalScrollbar(pos.X, pos.Y, p.mouseDown, mouseJustPressed) {
		p.interaction.HoverAndClick(pos.X, pos.Y, justPressed)
	}
}
