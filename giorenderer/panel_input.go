package giorenderer

import (
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
)

// Update reads this frame's pointer input and drives scroll/hover/click
// - call once per frame, before Draw. Gio unifies mouse and touch into
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
	gotEvent := false

	for {
		e, ok := gtx.Source.Event(pointer.Filter{
			Target:  p,
			Kinds:   pointer.Press | pointer.Release | pointer.Cancel | pointer.Move | pointer.Drag | pointer.Scroll,
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
		case pointer.Release, pointer.Cancel:
			p.dragging = false
		case pointer.Drag:
			if p.dragging {
				// "Content follows your finger" - see whynot.Interaction's
				// own doc comment on AccumulateMomentum for the sign
				// convention this matches.
				delta := float64(pos.Y - p.lastDragPos.Y)
				p.lastDragPos = pos
				p.interaction.Scroll(pos.X, pos.Y, delta)
				p.interaction.AccumulateMomentum(delta, now)
			}
		case pointer.Scroll:
			// Gio's Scroll.Y is positive scrolling down (confirmed
			// against gioui.org/layout.List's own use of it), the
			// opposite sign from View.Scroll's own convention (negative
			// moves down - see ScrollDown) - negated here, not inside
			// Interaction, since that's specific to Gio's own event
			// shape, not something ebitenrenderer's wheel path shares.
			p.interaction.CancelMomentum()
			p.interaction.Scroll(pos.X, pos.Y, -float64(pe.Scroll.Y))
		}
	}

	if !p.dragging {
		if justPressed {
			p.interaction.CancelMomentum()
		} else {
			p.interaction.Momentum(now)
		}
	}

	if gotEvent {
		p.interaction.HoverAndClick(pos.X, pos.Y, justPressed)
	}
}
