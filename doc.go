// Package whynot renders Markdown documents in Go programs, onto any
// graphics library a backend adapts (see package [canvas]).
//
// [Parse] turns Markdown into a [Document]. A [View] draws a Document into
// a rectangle of a canvas, scrolled to a position. A [Controller] turns a
// backend's input events (see package [input]) into what they do to a
// View: scrolling, hovering, clicking links. A [Panel] puts a View and a
// Controller together, for an app that just wants a document on screen.
//
// # Coordinates
//
// whynot uses one coordinate space: the canvas's. The [canvas.Canvas] a
// View draws on, the input events a Controller acts on, a View's or
// Panel's bounds, and everything a View reports by position
// ([View.HitTest], [View.LinkAt]) share the same origin and the same unit,
// canvas pixels: device pixels for whynot's own backends. Where the
// canvas sits in a window is the backend's business, not whynot's.
//
// A View is a window onto that space: [View.SetBounds] says where it is,
// and it draws and answers only there. There is no separate coordinate
// space of the View's own to convert to and from: a point a Controller
// receives can be passed to HitTest as it is, and the rectangle HitTest
// returns can be drawn on the canvas as it is.
//
// Distances are in canvas pixels too, such as [View.ScrollBy]'s. The
// scale relates them to logical pixels, the unit of a StyleSheet's
// dimensions: it's the number of canvas pixels per logical pixel, and a
// View's scale ([View.SetScale]) also includes any zoom. Command
// scrolling ([Controller.ScrollDown], [Controller.ScrollLeft] and the
// like, also on a Panel) steps in logical pixels without the zoom, so a
// step covers the same distance on screen at any zoom.
//
// How far a View is scrolled into its document isn't in pixels, because
// the document's height is an estimate until all of it has been laid
// out: [View.VisibleRange] reports the part in view as fractions of the
// height, and [View.ScrollToRatio] scrolls to one.
//
// # Events and state
//
// What happens in a View that an app may react to comes out of
// [Controller.Frame] as [Event] values, in order: the counterpart of the
// input events going in. They're a sealed set, like input's, so new
// kinds can be added without breaking anyone:
//
//	for _, e := range panel.Frame(in.Events(), now) {
//		switch e := e.(type) {
//		case whynot.LinkClick:
//			// follow e.Destination: load another document, say
//		case whynot.AnchorClick:
//			// already scrolled to e.ID, unless turned off
//		}
//	}
//
// A Controller only reports events. A Panel also acts on one: it
// scrolls to an [AnchorClick]'s heading, the way a browser follows a
// "#id" link, unless [Panel.SetAnchorScrolling] turns that off for an
// app that handles it itself, to record history first, say. Following
// a [LinkClick] is always up to the app.
//
// What's true at the moment, rather than what happened, is read from the
// View when needed, such as the link under the pointer
// ([View.HoveredLink]), which an app may show in a status bar.
package whynot
