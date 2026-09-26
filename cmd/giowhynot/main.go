// Command giowhynot is cmd/whynot's Gio (gioui.org) counterpart: the
// same browser.App - navigation history, theme, zoom, document/image
// loading, welcome page - driving a giorenderer.Panel instead of an
// ebitenrenderer.Panel, with a Gio-native toolbar (see toolbar.go).
package main

import (
	"flag"
	"image"
	"log"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/browser"
	"github.com/arnodel/whynot/giorenderer"
)

// initialWindowWidth/Height are Gio's starting window size, in dp.
const initialWindowWidth, initialWindowHeight = 1024, 768

func main() {
	light := flag.Bool("light", false, "use whynot's light theme instead of the default dark one")
	flag.Parse()

	// No file/URL given: land on the welcome page rather than a
	// hardcoded local file, matching what a person launching giowhynot
	// with no arguments should actually see.
	location := browser.WelcomeURL
	if flag.NArg() != 0 {
		var err error
		location, err = browser.ResolveLocationArg(flag.Arg(0))
		if err != nil {
			log.Fatal(err)
		}
	}
	source, err := browser.LoadDocument(location)
	if err != nil {
		log.Fatal(err)
	}

	styleSheet := whynot.StyleSheet(whynot.NewDarkStyleSheet())
	if *light {
		styleSheet = whynot.NewLightStyleSheet()
	}

	// scale (1) is a placeholder - View.Layout (via browserApp.Relayout)
	// resets this selector's DPI to the real deviceScale*zoom on the
	// very first frame, before anything is drawn.
	browserApp := browser.NewApp(browser.NewDocumentFaceSelector(1), styleSheet, !*light)
	renderer := giorenderer.New()
	view := browserApp.NewView(source, location)
	panel := giorenderer.NewPanel(view, renderer, image.Rectangle{},
		giorenderer.WithScrollbar(),
		giorenderer.WithStyleSheet(styleSheet),
	)
	browserApp.Panel = panel
	panel.OnLinkClick = browserApp.Follow
	panel.OnLinkHover = browserApp.OnLinkHover

	tb := newToolbar(whynot.NewGoFontFaceSelector(72), renderer)
	panel.OnPress = tb.cancelEdit

	win := new(app.Window)
	win.Option(app.Title("Why Not?"), app.Size(initialWindowWidth, initialWindowHeight))
	browserApp.OnTitleChange = func(title string) { win.Option(app.Title(title)) }
	browserApp.Open(location)

	go func() {
		if err := run(win, browserApp, panel, tb); err != nil {
			log.Fatal(err)
		}
	}()
	app.Main()
}

func run(win *app.Window, browserApp *browser.App, panel *giorenderer.Panel, tb *toolbar) error {
	var ops op.Ops
	for {
		e := win.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			deviceScale := float64(e.Metric.PxPerDp)
			toolbarHeight := gtx.Dp(unit.Dp(toolbarLogicalHeight))
			outsideWidth := int(float64(e.Size.X) / deviceScale)
			outsideHeight := int(float64(e.Size.Y) / deviceScale)
			browserApp.Relayout(outsideWidth, outsideHeight, deviceScale, toolbarHeight)
			// Pinned to deviceScale alone, not the document's own
			// zoomed scale - the toolbar's own text stays a fixed
			// physical size regardless of document zoom, matching
			// cmd/whynot's identically-reasoned toolbarFaceSelector.
			tb.faceSelector.SetDPI(deviceScale * 72)

			tb.update(gtx, browserApp)
			// Suspended while editing the address bar - pollKeys's
			// filters match regardless of focus, so e.g. typing "-" or
			// space into it would otherwise also fire ZoomOut/PageDown.
			if !tb.editing {
				pollKeys(gtx, browserApp, panel, deviceScale)
			}
			panel.Update(gtx)

			panel.Draw(gtx)
			tb.layout(gtx, browserApp)

			e.Frame(gtx.Ops)
		}
	}
}
