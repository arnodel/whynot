// Command giowhynot is cmd/whynot's Gio (gioui.org) counterpart: the
// same browser.App - navigation history, theme, zoom, document/image
// loading, welcome page - drawn with giobackend instead of
// ebitenbackend, with Gio's own scrollbar and a Gio-native toolbar (see
// toolbar.go).
package main

import (
	"flag"
	"image"
	"log"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/backends/giobackend"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/browser"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// initialWindowWidth/Height are Gio's starting window size, in dp.
const initialWindowWidth, initialWindowHeight = 1024, 768

func main() {
	// Before the welcome page is first loaded - see its own doc
	// comment on why this needs setting explicitly.
	browser.AddressBarEditable = true

	light := flag.Bool("light", false, "use whynot's light theme instead of the default dark one")
	rootDir := flag.String("root", "", "only open local documents and images beneath this directory (default: anywhere on the document's drive)")
	flag.Parse()

	var root *os.Root
	if *rootDir != "" {
		var err error
		if root, err = os.OpenRoot(*rootDir); err != nil {
			log.Fatal(err)
		}
	}
	registry := browser.NewRegistry(root)

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
	styleSheet := simpletheme.DarkStyleSheet
	if *light {
		styleSheet = simpletheme.LightStyleSheet
	}

	browserApp := browser.NewApp(browser.NewDocumentFaceSelector(), styleSheet, !*light, registry)
	renderer := giobackend.New()
	view := browserApp.NewView(nil, location)
	panel := whynot.NewPanel(view, image.Rectangle{})
	browserApp.Panel = panel
	panel.SetAnchorScrolling(false) // the app follows anchors itself (HandleEvents)

	tb := newToolbar(fonts.NewGoSelector(), renderer)
	pr := newPrompt(fonts.NewGoSelector(), renderer)
	doc := &document{panel: panel, renderer: renderer, start: time.Now(),
		onPress: func() {
			tb.cancelEdit()
			// A press in the document, outside the prompt, cancels it.
			browserApp.CancelPrompt()
		},
		onEvents: func(events []whynot.Event) {
			// The document can't be used while the prompt shows.
			if !browserApp.Prompting() {
				browserApp.HandleEvents(events)
			}
		},
	}

	win := new(app.Window)
	win.Option(app.Title("Why Not?"), app.Size(initialWindowWidth, initialWindowHeight))
	browserApp.OnTitleChange = func(title string) { win.Option(app.Title(title)) }
	if err := browserApp.Open(location); err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := run(win, browserApp, doc, tb, pr); err != nil {
			log.Fatal(err)
		}
	}()
	app.Main()
}

func run(win *app.Window, browserApp *browser.App, doc *document, tb *toolbar, pr *prompt) error {
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
			browserApp.Update()
			// Gio only produces frames when something happens.
			if browserApp.Loading() {
				gtx.Execute(op.InvalidateCmd{})
			}
			tb.dpi = deviceScale * 72

			tb.update(gtx, browserApp)
			pr.update(gtx, browserApp)
			// Suspended while editing the address bar or the prompt -
			// pollKeys's filters match regardless of focus, so e.g.
			// typing "-" or space into it would otherwise also fire
			// ZoomOut/PageDown.
			if !tb.editing && !pr.open {
				pollKeys(gtx, browserApp, doc.panel)
			}
			doc.layout(gtx)
			tb.layout(gtx, browserApp)
			pr.layout(gtx, toolbarHeight)
			// A link clicked this frame may be waiting for text: the
			// prompt opens next frame.
			if browserApp.Prompting() && !pr.open {
				gtx.Execute(op.InvalidateCmd{})
			}

			e.Frame(gtx.Ops)
		}
	}
}
