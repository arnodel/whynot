// Package giobackend is whynot's backend for Gio (gioui.org). It provides
// what a [whynot.View] or [whynot.Panel] needs from a Gio window:
//
//   - drawing: a [Renderer] makes a [Canvas] (a [canvas.Canvas]) that
//     records into a frame's op.Ops;
//   - input: an [Input] turns Gio's pointer events for an area into
//     [input.Event] values;
//   - optionally, a [NativeScrollbar]: Gio's own material scrollbar,
//     for an app that wants its scrollbar to match the rest of its UI
//     rather than use the View's.
//
// The core whynot packages don't depend on Gio; only this one does. It's
// a Go module of its own (github.com/arnodel/whynot/backends/giobackend),
// so the core doesn't pull in Gio, and it stays on v0 while Gio does:
// Gio's types are part of its API, so a breaking change in Gio can force
// one here.
//
// # A window showing a document
//
// Gio only produces a frame when something happens, so while a fling
// coasts or a scrollbar fades the app must ask for the next one
// ([whynot.Panel.Animating]):
//
//	w := new(app.Window)
//	panel := whynot.NewPanel(view, image.Rectangle{})
//	renderer := giobackend.New()
//	var in giobackend.Input
//	start := time.Now()
//	var ops op.Ops
//	for {
//		switch e := w.Event().(type) {
//		case app.DestroyEvent:
//			return e.Err
//		case app.FrameEvent:
//			gtx := app.NewContext(&ops, e)
//			bounds := image.Rectangle{Max: e.Size}
//			panel.SetBounds(bounds)
//			panel.SetScale(float64(e.Metric.PxPerDp))
//			now := time.Since(start)
//			for _, ev := range panel.Frame(in.Source(gtx, panel, bounds).Events(), now) {
//				if link, ok := ev.(whynot.LinkClick); ok {
//					log.Println("clicked", link.Destination)
//				}
//			}
//			if panel.Animating() {
//				gtx.Execute(op.InvalidateCmd{})
//			}
//			panel.Draw(renderer.NewCanvas(gtx.Ops, bounds), now)
//			e.Frame(gtx.Ops)
//		}
//	}
//
// The coordinates of the canvas and of the input events are those of
// the frame's ops, as whynot expects (see Coordinates in package
// whynot's documentation).
//
// # Text
//
// Gio's text shaper doesn't draw a font.Face's glyphs, which whynot lays
// text out with, so a Canvas rasterizes each glyph itself (through
// font.Face.Glyph) and paints the bitmaps, caching them.
package giobackend
