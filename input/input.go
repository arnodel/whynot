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
// pointer at (X, Y). DX and DY are how far to scroll, positive towards
// the end of the content (right and down), in logical pixels: a backend
// doesn't need to know the screen's density, and a Controller converts
// them to the View's pixels with its scale, so a wheel scrolls the same
// distance on any screen.
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
