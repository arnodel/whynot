package ebitenrenderer

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot/input"
)

// wheelPixels is how far one unit of ebiten.Wheel scrolls, in logical
// pixels.
const wheelPixels = 2

// Input is an input.Source reading Ebitengine's input: Events reports
// what changed since the previous call, which should be once per tick,
// from the game's Update. A touch, while there is one, takes over from
// the mouse; at most one touch is tracked.
type Input struct {
	// Scale converts Ebitengine's wheel units to the screen's pixels:
	// the scale the document is drawn at (whynot.Panel's Scale times
	// its Zoom) when the game draws at the device's resolution or
	// zooms. Zero means 1.
	Scale float64

	cursor    image.Point
	hasCursor bool
	mouseDown bool

	trackingTouch bool
	activeTouch   ebiten.TouchID
	// touching is whether the last call saw a touch, to report its end
	// once it lifts.
	touching bool
}

var _ input.Source = (*Input)(nil)

// Events implements input.Source.
func (in *Input) Events() []input.Event {
	if cx, cy, justPressed, ok := in.touchInput(); ok {
		in.touching = true
		if justPressed {
			return []input.Event{input.TouchStart{ID: int(in.activeTouch), X: cx, Y: cy}}
		}
		return []input.Event{input.TouchMove{ID: int(in.activeTouch), X: cx, Y: cy}}
	}
	var events []input.Event
	if in.touching {
		in.touching = false
		events = append(events, input.TouchEnd{ID: int(in.activeTouch)})
	}
	cx, cy := ebiten.CursorPosition()
	wx, wy := ebiten.Wheel()
	scale := in.Scale
	if scale == 0 {
		scale = 1
	}
	return append(events, in.mouseEvents(mouseInput{
		x: cx, y: cy,
		down:   ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),
		wheelX: wx, wheelY: wy,
		scale: scale,
		mods:  modifiers(),
	})...)
}

// mouseInput is one tick's mouse state.
type mouseInput struct {
	x, y           int
	down           bool // the primary button
	wheelX, wheelY float64
	scale          float64 // see Input.Scale
	mods           input.Modifiers
}

// mouseEvents reports what changed in st since the last tick, as events.
func (in *Input) mouseEvents(st mouseInput) []input.Event {
	var events []input.Event
	pos := image.Pt(st.x, st.y)
	if !in.hasCursor || pos != in.cursor {
		events = append(events, input.PointerMove{X: st.x, Y: st.y})
		in.cursor, in.hasCursor = pos, true
	}
	if st.down != in.mouseDown {
		events = append(events, input.PointerButton{X: st.x, Y: st.y, Button: input.ButtonPrimary, Down: st.down, Mods: st.mods})
		in.mouseDown = st.down
	}
	if st.wheelX != 0 || st.wheelY != 0 {
		// ebiten.Wheel is positive up and left; Wheel events are
		// positive towards the end, in pixels.
		k := -wheelPixels * st.scale
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

// touchInput reports the touch being tracked - ok is false when there's
// none. Tracks at most one touch, ignoring any second simultaneous one.
func (in *Input) touchInput() (cx, cy int, justPressed bool, ok bool) {
	if in.trackingTouch {
		for _, id := range ebiten.AppendTouchIDs(nil) {
			if id == in.activeTouch {
				x, y := ebiten.TouchPosition(id)
				return x, y, false, true
			}
		}
		// The touch we were tracking ended - fall through to look for a
		// different one already active this same tick.
		in.trackingTouch = false
	}

	ids := ebiten.AppendTouchIDs(nil)
	if len(ids) == 0 {
		return 0, 0, false, false
	}
	in.activeTouch = ids[0]
	in.trackingTouch = true
	x, y := ebiten.TouchPosition(in.activeTouch)
	return x, y, true, true
}
