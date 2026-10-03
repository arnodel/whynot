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
// # Links
//
// A [Controller] highlights the link under the pointer and reports it
// to its OnLinkHover callback. What a click or tap on a link does
// depends on where it leads:
//
//   - A link within the document, "#id", scrolls the View to the heading
//     with that id ([View.ScrollToAnchor]) by default. Setting the
//     OnAnchorClick callback replaces that, for an app that also wants
//     to record history, say.
//   - Any other link is passed to the OnLinkClick callback, since
//     following it (resolving it, loading another document) is up to the
//     app. By default, nothing happens.
//
// A Panel has the same callbacks.
package whynot
