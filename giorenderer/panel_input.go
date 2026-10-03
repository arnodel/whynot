package giorenderer

import (
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/arnodel/whynot/input"
)

// Update reads this frame's pointer input and passes it to the Panel's
// Controller - call once per frame, before Draw. Gio routes events by
// area, so a press on the scrollbar's own region (added after this
// Panel's own area - see Draw) never reaches here.
func (p *Panel) Update(gtx layout.Context) {
	p.controller.OnLinkClick = p.OnLinkClick
	p.controller.OnLinkHover = p.OnLinkHover
	p.controller.AnchorScrolling = p.anchorScrolling

	area := clip.Rect(p.bounds).Push(gtx.Ops)
	event.Op(gtx.Ops, p)
	area.Pop()

	var events []input.Event
	pressed := false
	for {
		e, ok := gtx.Source.Event(pointer.Filter{
			Target:  p,
			Kinds:   pointer.Press | pointer.Release | pointer.Cancel | pointer.Move | pointer.Drag | pointer.Scroll | pointer.Leave,
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
		if e := p.inputEvent(pe); e != nil {
			events = append(events, e)
		}
		pressed = pressed || pe.Kind == pointer.Press
	}

	p.controller.Frame(events, p.elapsed())
	// Gio only produces frames when something happens, so keep asking
	// for them while there's a fling or a fading scrollbar to animate.
	if p.controller.Animating() {
		gtx.Execute(op.InvalidateCmd{})
	}
	if pressed && p.OnPress != nil {
		p.OnPress()
	}
}

// inputEvent translates a Gio pointer event, or returns nil for one with
// no counterpart.
func (p *Panel) inputEvent(pe pointer.Event) input.Event {
	x, y := int(pe.Position.X), int(pe.Position.Y)
	if pe.Source == pointer.Touch {
		id := int(pe.PointerID)
		switch pe.Kind {
		case pointer.Press:
			return input.TouchStart{ID: id, X: x, Y: y}
		case pointer.Drag:
			return input.TouchMove{ID: id, X: x, Y: y}
		case pointer.Release:
			return input.TouchEnd{ID: id}
		case pointer.Cancel:
			return input.TouchCancel{ID: id}
		}
		return nil
	}
	mods := modifiers(pe.Modifiers)
	switch pe.Kind {
	case pointer.Press:
		button := input.ButtonPrimary
		switch {
		case pe.Buttons.Contain(pointer.ButtonPrimary):
			p.mouseDown = true
		case pe.Buttons.Contain(pointer.ButtonSecondary):
			button = input.ButtonSecondary
		case pe.Buttons.Contain(pointer.ButtonTertiary):
			button = input.ButtonMiddle
		}
		return input.PointerButton{X: x, Y: y, Button: button, Down: true, Mods: mods}
	case pointer.Release, pointer.Cancel:
		if p.mouseDown && !pe.Buttons.Contain(pointer.ButtonPrimary) {
			p.mouseDown = false
			return input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary, Mods: mods}
		}
	case pointer.Move, pointer.Drag:
		return input.PointerMove{X: x, Y: y}
	case pointer.Leave:
		return input.PointerLeave{}
	case pointer.Scroll:
		// Gio's scroll is already in pixels, positive right and down.
		return input.Wheel{X: x, Y: y, DX: float64(pe.Scroll.X), DY: float64(pe.Scroll.Y), Mods: mods}
	}
	return nil
}

// modifiers translates Gio's modifier keys.
func modifiers(m key.Modifiers) input.Modifiers {
	var mods input.Modifiers
	if m.Contain(key.ModShift) {
		mods |= input.ModShift
	}
	if m.Contain(key.ModCtrl) {
		mods |= input.ModCtrl
	}
	if m.Contain(key.ModAlt) {
		mods |= input.ModAlt
	}
	if m.Contain(key.ModSuper) || m.Contain(key.ModCommand) {
		mods |= input.ModMeta
	}
	return mods
}
