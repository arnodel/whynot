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

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/giorenderer"
	"github.com/arnodel/whynot/internal/browser"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// initialWindowWidth/Height are Gio's starting window size, in dp.
const initialWindowWidth, initialWindowHeight = 1024, 768

func main() {
	// Before any LoadDocument(WelcomeURL, ...) call - see its own doc
	// comment on why this needs setting explicitly.
	browser.AddressBarEditable = true

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

	styleSheet := simpletheme.DarkStyleSheet
	if *light {
		styleSheet = simpletheme.LightStyleSheet
	}

	browserApp := browser.NewApp(browser.NewDocumentFaceSelector(), styleSheet, !*light)
	renderer := giorenderer.New()
	view := browserApp.NewView(source, location)
	panel := giorenderer.NewPanel(view, renderer, image.Rectangle{},
		giorenderer.WithScrollbar(),
		giorenderer.WithStyleSheet(styleSheet),
	)
	browserApp.Panel = panel
	panel.OnLinkClick = browserApp.Follow
	panel.OnLinkHover = browserApp.OnLinkHover

	tb := newToolbar(fonts.NewGoSelector(), renderer)
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
			tb.dpi = deviceScale * 72

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
