// Package input is the contract between a backend's input and whynot: a
// backend reports what the user did as Events, which a whynot.Controller
// turns into scrolling, hovering, clicking and so on.
//
// Positions and wheel deltas are in the coordinate space of the Canvas
// the View is drawn on (device pixels for whynot's own backends): see
// Coordinates in package whynot's documentation. A backend reports all
// the input it sees; a Controller acts on events inside its View's
// bounds, except that a press inside captures the pointer until it's
// released.
//
// Keyboard input isn't part of it: keys belong to the app, which binds
// them to commands such as paging.
package input

// Event is something the user did. Only this package defines kinds of
// Event, so new kinds can be added without breaking backends or
// handlers: a backend that doesn't know a kind never reports it, and a
// handler ignores kinds it doesn't know.
type Event interface{ isEvent() }

// PointerMove is the mouse pointer moving to (X, Y).
type PointerMove struct{ X, Y int }

// PointerLeave is the mouse pointer leaving the area the backend reports
// input for.
type PointerLeave struct{}

// PointerButton is a mouse button going down or up, with the pointer at
// (X, Y).
type PointerButton struct {
	X, Y   int
	Button Button
	Down   bool
	Mods   Modifiers
}

// Wheel is a scroll, e.g. by a mouse wheel or a trackpad, with the
// pointer at (X, Y). DX and DY are in pixels, positive towards the end
// of the content: right and down.
type Wheel struct {
	X, Y   int
	DX, DY float64
	Mods   Modifiers
}

// TouchStart is a finger touching at (X, Y). ID tells simultaneous
// touches apart, until the touch ends.
type TouchStart struct{ ID, X, Y int }

// TouchMove is the touch ID moving to (X, Y).
type TouchMove struct{ ID, X, Y int }

// TouchEnd is the touch ID lifting.
type TouchEnd struct{ ID int }

// TouchCancel is the touch ID being taken away, e.g. by a system
// gesture: unlike TouchEnd, it's never a tap.
type TouchCancel struct{ ID int }

func (PointerMove) isEvent()   {}
func (PointerLeave) isEvent()  {}
func (PointerButton) isEvent() {}
func (Wheel) isEvent()         {}
func (TouchStart) isEvent()    {}
func (TouchMove) isEvent()     {}
func (TouchEnd) isEvent()      {}
func (TouchCancel) isEvent()   {}

// Button is a mouse button.
type Button int

const (
	ButtonPrimary Button = iota
	ButtonSecondary
	ButtonMiddle
)

// Modifiers is the set of modifier keys held during an event.
type Modifiers int

const (
	ModShift Modifiers = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
)

// Contain reports whether all of mods are held.
func (m Modifiers) Contain(mods Modifiers) bool {
	return m&mods == mods
}

// Source is a backend's input reader. Each call to Events returns what
// the user did since the previous call, in order, as this package's
// events.
type Source interface {
	Events() []Event
}
