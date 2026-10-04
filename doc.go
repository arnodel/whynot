// Package whynot shows Markdown documents in Go programs, such as games and
// GUI apps: it lays a document out, draws it, and lets the user scroll
// through it and click its links. It draws through a small interface
// rather than a particular graphics library, and receives input as plain
// values, so it works with any framework a backend adapts: whynot comes
// with backends for Ebitengine and Gio.
//
// # Overview
//
// The package has four main types:
//
//   - [Parse] turns Markdown into a [Document].
//   - A [View] lays a Document out and draws it into a rectangle of a
//     canvas, scrolled to a position. It only lays out what's on screen,
//     so long documents cost no more than short ones.
//   - A [Controller] turns the user's input into what it does to a View:
//     scrolling (by wheel, touch, scrollbar or command), highlighting the
//     link under the pointer, and reporting clicks.
//   - A [Panel] is a View and a Controller together, with settings that
//     survive showing another document: the simplest way to put a document
//     on screen.
//
// A View is given what decides how the document looks:
//
//   - a [StyleSheet], for colors, text styles, margins and scrollbars,
//     made by a package under styles/, such as styles/simpletheme;
//   - a [fonts.FaceSelector], for the fonts the text is drawn with (see
//     package [fonts]);
//   - optionally, an [images.Source], for where images come from
//     ([WithImageSource]; see package [images]).
//
// [Parse] can be given code-block plugins ([WithCodeBlockPlugin]; see
// package [codeblocks]), for syntax highlighting or diagrams.
//
// # Showing a document
//
// A backend provides a [canvas.Canvas] to draw on and an [input.Source]
// of the user's input. With Ebitengine, for example:
//
//	renderer := ebitenbackend.New()
//	var source ebitenbackend.Input       // reads Ebitengine's input: an input.Source
//	canvas := renderer.NewCanvas(screen) // draws on the game's screen: a canvas.Canvas
//
// With Gio, a giobackend.Renderer and a giobackend.Input play the same
// roles. A program sets a Panel up once:
//
//	doc := whynot.Parse(markdown)
//	view := whynot.NewView(doc, fonts.NewGoSelector(), simpletheme.DarkStyleSheet)
//	panel := whynot.NewPanel(view, bounds) // where it's drawn on the canvas
//	panel.SetScale(scale)                  // the screen's density
//
// then, every frame, passes it the input and the time, reacts to what
// happened, and draws it:
//
//	events := panel.Frame(source.Events(), now)
//	// react to events, such as a LinkClick
//	panel.Draw(canvas, now)
//
// The packages backends/ebitenbackend and backends/giobackend show the
// complete frame loop for their framework. When the window is resized,
// the screen changes or the user zooms, the program calls
// [Panel.SetBounds], [Panel.SetScale] or [Panel.SetZoom]; the document
// is laid out again, keeping the scroll position.
//
// # A Panel, or a View and a Controller
//
// A Panel covers what most programs need. Using a View and a Controller
// directly gives more control: a program can drive a View with input of
// its own, place it in a structure of its own, or move it by code alone
// ([View.ScrollBy], [View.ScrollToAnchor], [View.ScrollToRatio]). A View
// also answers questions about what it shows: the link under the pointer
// ([View.HoveredLink]), the part of the document in view
// ([View.VisibleRange]), the heading currently on screen
// ([View.CurrentHeadingID]).
//
// # Coordinates
//
// whynot uses one coordinate space: the canvas's. The Canvas a View draws
// on, the input events a Controller acts on, a View's or Panel's bounds,
// and everything a View reports by position ([View.HitTest],
// [View.LinkAt]) share the same origin and the same unit, canvas pixels:
// device pixels for whynot's own backends. Where the canvas sits in a
// window is the backend's business, not whynot's.
//
// A View is a window onto that space: [View.SetBounds] says where it is,
// and it draws and answers only there. There is no separate coordinate
// space of the View's own to convert to and from: a point a Controller
// receives can be passed to HitTest as it is, and the rectangle HitTest
// returns can be drawn on the canvas as it is.
//
// Distances are in canvas pixels too, such as [View.ScrollBy]'s. The
// scale relates them to logical pixels, the unit of a StyleSheet's
// dimensions: it's the number of canvas pixels per logical pixel
// ([View.SetScale]), and a View is laid out at its scale times its zoom
// ([View.SetZoom]). Command scrolling ([Controller.ScrollDown],
// [Controller.ScrollLeft] and the like, also on a Panel) steps in logical
// pixels without the zoom, so a step covers the same distance on screen
// at any zoom.
//
// How far a View is scrolled into its document isn't in pixels, because
// the document's height is an estimate until all of it has been laid
// out: [View.VisibleRange] reports the part in view as fractions of the
// height, and [View.ScrollToRatio] scrolls to one.
//
// # Events and state
//
// What happens in a View that a program may react to comes out of
// [Controller.Frame] as [Event] values, in order: the counterpart of the
// input events going in. They're a sealed set, like input's, so new kinds
// can be added without breaking anyone:
//
//	for _, e := range panel.Frame(source.Events(), now) {
//		switch e := e.(type) {
//		case whynot.LinkClick:
//			// follow e.Destination: load another document, say
//		case whynot.AnchorClick:
//			// already scrolled to e.ID, unless turned off
//		}
//	}
//
// A Controller only reports events. A Panel also acts on one: it scrolls
// to an [AnchorClick]'s heading, the way a browser follows a "#id" link,
// unless [Panel.SetAnchorScrolling] turns that off for a program that
// handles it itself, to record history first, say. Following a
// [LinkClick] is always up to the program.
//
// What's true at the moment, rather than what happened, is read from the
// View when needed, such as the link under the pointer
// ([View.HoveredLink]), which a program may show in a status bar.
//
// # Related packages
//
// The contracts whynot works with each have a package of their own, so
// that each can be implemented separately:
//
//   - [canvas] and [input]: what a backend provides, drawing and input
//     events. The backends are under backends/.
//   - [fonts], [images] and [codeblocks]: fonts, image loading and
//     code-block plugins. Ready-made implementations are in their
//     subpackages: fonts/systemfont (fonts installed on the system),
//     codeblocks/chromahighlight (syntax highlighting) and codeblocks/kroki
//     (diagrams).
//   - Stylesheets: styles/simpletheme, with light and dark themes.
package whynot
