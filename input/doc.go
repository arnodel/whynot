// Package input is the contract between a backend's input and whynot. A
// backend reports what the user did as [Event] values: pointer movements
// and buttons ([PointerMove], [PointerLeave], [PointerButton]), scrolling
// ([Wheel]) and touches ([TouchStart], [TouchMove], [TouchEnd],
// [TouchCancel]). A whynot.Controller turns them into scrolling, hovering,
// clicking and so on.
//
// A backend's input reader is a [Source]: each call to its Events method
// returns what happened since the previous call, in order. A program calls
// it once per frame and passes the events to its Controller or Panel.
//
// # Units
//
// Positions are in the coordinate space of the Canvas the View is drawn
// on (device pixels for whynot's own backends): see the Coordinates
// section of package whynot's documentation. A [Wheel]'s deltas are an
// amount to scroll rather than a position, so they're in logical pixels,
// independent of the screen's density: a Controller scales them.
//
// A backend reports all the input it sees. A Controller acts on the events
// inside its View's bounds, except that a press inside them captures the
// pointer until it's released.
//
// Keyboard input isn't part of it: keys belong to the program, which binds
// them to commands such as a Panel's PageDown.
//
// # Without a Controller
//
// A program driving a View by hand reads the events itself, acting on the
// ones it cares about. This scrolls with the wheel:
//
//	for _, e := range source.Events() {
//		if w, ok := e.(input.Wheel); ok {
//			view.ScrollBy(w.DY * view.Scale()) // logical pixels, scaled to the View's
//		}
//	}
package input
