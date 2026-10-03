package giobackend

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/arnodel/whynot/input"
)

// Input reads Gio's pointer input for an area of the window. Gio only
// has events within a frame, so each frame gets its own input.Source
// (see Source).
type Input struct {
	// mouseDown is whether the primary mouse button is pressed, to report
	// its release.
	mouseDown bool
}

// Source returns this frame's input.Source: the pointer events Gio has
// for area, registering it, with tag, to receive them. Call Events once.
func (in *Input) Source(gtx layout.Context, tag event.Tag, area image.Rectangle) input.Source {
	return frameSource{in: in, gtx: gtx, tag: tag, area: area}
}

// frameSource is one frame's input.Source (see Input.Source).
type frameSource struct {
	in   *Input
	gtx  layout.Context
	tag  event.Tag
	area image.Rectangle
}

// Events implements input.Source.
func (s frameSource) Events() []input.Event {
	stack := clip.Rect(s.area).Push(s.gtx.Ops)
	event.Op(s.gtx.Ops, s.tag)
	stack.Pop()

	var events []input.Event
	for {
		e, ok := s.gtx.Source.Event(pointer.Filter{
			Target:  s.tag,
			Kinds:   pointer.Press | pointer.Release | pointer.Cancel | pointer.Move | pointer.Drag | pointer.Scroll | pointer.Leave,
			ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			return events
		}
		if pe, ok := e.(pointer.Event); ok {
			if e := s.in.translate(pe); e != nil {
				events = append(events, e)
			}
		}
	}
}

// translate translates a Gio pointer event, or returns nil for one with
// no counterpart.
func (in *Input) translate(pe pointer.Event) input.Event {
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
			in.mouseDown = true
		case pe.Buttons.Contain(pointer.ButtonSecondary):
			button = input.ButtonSecondary
		case pe.Buttons.Contain(pointer.ButtonTertiary):
			button = input.ButtonMiddle
		}
		return input.PointerButton{X: x, Y: y, Button: button, Down: true, Mods: mods}
	case pointer.Release, pointer.Cancel:
		if in.mouseDown && !pe.Buttons.Contain(pointer.ButtonPrimary) {
			in.mouseDown = false
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
