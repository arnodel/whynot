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
// [NewView]'s options decide how the document looks, and each has a
// default:
//
//   - a [StyleSheet], for colors, text styles, margins and scrollbars
//     ([WithStyleSheet]), made by a package under styles/: by default,
//     the dark theme of styles/simpletheme;
//   - a [fonts.FaceSelector], for the fonts the text is drawn with
//     ([WithFaceSelector]; see package [fonts]): by default, the Go fonts.
//
// [Parse] can be given code-block plugins ([WithCodeBlockPlugin]; see
// package [codeblocks]), for syntax highlighting or diagrams, and the
// language of the document's inline code, so that they highlight that
// too ([WithInlineCodeLanguage]), and what
// the document's images can be fetched from ([WithImageRegistry] and
// [WithBaseURL]; see package [fetch]). Nothing is fetched by default: a
// document parsed without a registry shows no images.
//
// # Showing a document
//
// A backend provides two things: an [input.Source] that reads the user's
// input from the framework, and a [canvas.Canvas] to draw on. Here is how
// to use them with Ebitengine.
//
// Once, when your program starts, make the backend's input reader and
// renderer, and set a Panel up:
//
//	var source ebitenbackend.Input  // reads Ebitengine's input: an input.Source
//	renderer := ebitenbackend.New() // keeps image textures and glyph caches across frames
//
//	doc := whynot.Parse(markdown)
//	view := whynot.NewView(doc)            // Go fonts, dark theme
//	panel := whynot.NewPanel(view, bounds) // where it's drawn on the screen
//	panel.SetScale(scale)                  // the screen's density
//
// Then, every frame, pass the input to the Panel, with the time, and
// react to what happened. With Ebitengine, do that in your game's Update
// method:
//
//	events := panel.Frame(source.Events(), now)
//	// react to events, such as a LinkClick
//
// Draw the Panel every frame too. Ebitengine gives your game's Draw
// method the screen image to draw on each time, so make a canvas onto it
// each time as well. That's cheap: the renderer holds what's worth
// keeping.
//
//	panel.Draw(renderer.NewCanvas(screen), now)
//
// With Gio, a giobackend.Input and a giobackend.Renderer play the same
// roles, and the frame loop is shaped by Gio's events instead. The
// packages backends/ebitenbackend and backends/giobackend each show a
// complete program. When the window is resized, the screen changes or the
// user zooms, call [Panel.SetBounds], [Panel.SetScale] or [Panel.SetZoom]:
// the document is laid out again, keeping the scroll position.
//
// # A Panel, or a View and a Controller
//
// A Panel covers what most programs need. Use a View and a Controller
// directly for more control: you can then drive a View with input of your
// own, fit it into a structure of your own, or move it by code alone
// ([View.ScrollBy], [View.ScrollToAnchor], [View.ScrollToRatio],
// [View.ScrollToEnd]). A View
// also answers questions about what it shows: the link under the pointer
// ([View.HoveredLink]), the part of the document in view
// ([View.VisibleRange]), the heading currently on screen
// ([View.CurrentHeadingID]), and exactly how much of the document is
// above and below its top ([View.ContentAbove], [View.ContentBelow]), for
// a program arranging Views itself.
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
// # Front matter
//
// A document may start with front matter, YAML between two "---" lines,
// as used by Jekyll and Hugo. It isn't shown, and whynot doesn't read it:
// [Document.RawFrontMatter] returns it as written, for the program to
// decode with the YAML library of its choice.
//
// # Documents that arrive a piece at a time
//
// A Document can grow as its Markdown arrives: a download over a slow
// network, a reply being generated, or a game's narration revealed a word
// at a time. [Parser.Stream] returns a Document and a writer for it:
//
//	doc, w := whynot.NewParser().Stream()
//	view := whynot.NewView(doc)
//	go func() {
//		io.Copy(w, response.Body)
//		w.Close() // the Document is complete
//	}()
//
// Writing only stores the text. Each time a View draws, it takes what was
// written since it last drew: the text is parsed then, at most once a
// frame, and only from the start of the blocks still unfinished, and the
// View lays out only what's new, keeping its scroll position. A program
// whose framework draws every frame, as Ebitengine's does, needs nothing
// more. One whose framework draws only when something happens, as Gio's
// does, asks for a frame whenever the Document changes:
//
//	go func() {
//		for range doc.Updates() {
//			window.Invalidate()
//		}
//	}()
//
// A Document that grows shows what [Parse] would make of the text so far,
// with one difference: a reference definition ("[x]: https://...") that
// arrives after a View has shown the paragraph using it doesn't make that
// paragraph a link. A Document written in one go, before any View draws
// it, is parsed as a whole.
//
// # Concurrency
//
// A View, a Controller and a Panel are not safe for concurrent use: use
// each from one goroutine, normally the one your framework draws on. Only
// image fetching happens in the background, on goroutines of its own (see
// [fetch.Source]). A Document and the writer of one made by
// [Parser.Stream] are the exception: write it on any goroutine, typically
// one reading from the network, while Views of it draw on another.
//
// Views made without [WithFaceSelector] share one selector of the Go
// fonts, and so share its font faces. That lets the backends cache what
// they make from each face once for every View, instead of once per
// document. But font faces aren't safe for concurrent use either, so if
// your program uses Views on several goroutines at once, for example to
// render documents in parallel, give each goroutine's Views a selector of
// their own.
//
// # Related packages
//
// The contracts whynot works with each have a package of their own, so
// that each can be implemented separately:
//
//   - [canvas] and [input]: what a backend provides, drawing and input
//     events. The backends are under backends/.
//   - [fonts], [fetch] and [codeblocks]: fonts, fetching images and
//     code-block plugins. Ready-made implementations are in their
//     subpackages: fonts/systemfont (fonts installed on the system),
//     codeblocks/chromahighlight (syntax highlighting) and codeblocks/kroki
//     (diagrams).
//   - Stylesheets: styles/simpletheme, with light and dark themes.
package whynot
