package main

import (
	"flag"
	"image"
	"log"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/backends/ebitenbackend"
	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/browser"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// initialWindowWidth/Height are ebiten's starting window size, and also
// what game.outsideWidth/Height are seeded to in main - so the very
// first Bounds/Scale handed to NewPanel already match what ebiten's own
// first Layout callback would report, rather than a throwaway guess
// immediately superseded.
const initialWindowWidth, initialWindowHeight = 1024, 768

func main() {
	light := flag.Bool("light", false, "use whynot's light theme instead of the default dark one")
	debugHit := flag.Bool("debug-hit", false, "outline the box under the mouse, via View.HitTest")
	rootDir := flag.String("root", "", "only open local documents and images beneath this directory (default: anywhere on the document's drive)")
	debugStats := flag.Bool("debug-stats", false, "show FPS/TPS and per-frame Update/Draw timing at startup - togglable at runtime with F regardless")
	claudeModel := flag.String("claude-model", "claude-sonnet-5", "the model that writes claude: pages, when ANTHROPIC_API_KEY is set")
	flag.Parse()

	var root *os.Root
	if *rootDir != "" {
		var err error
		if root, err = os.OpenRoot(*rootDir); err != nil {
			log.Fatal(err)
		}
	}
	var extra []fetch.Resolver
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		extra = append(extra, &browser.ClaudeResolver{APIKey: key, Model: *claudeModel})
	}
	registry := browser.NewRegistry(root, extra...)

	// No file/URL given: land on the welcome page rather than a
	// hardcoded local file, matching what a person launching whynot
	// with no arguments should actually see.
	location := browser.WelcomeURL
	if flag.NArg() != 0 {
		var err error
		location, err = browser.ResolveLocationArg(flag.Arg(0))
		if err != nil {
			panic(err)
		}
	}
	ebiten.SetWindowSize(initialWindowWidth, initialWindowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	styleSheet := simpletheme.DarkStyleSheet
	if *light {
		styleSheet = simpletheme.LightStyleSheet
	}

	scale := ebiten.Monitor().DeviceScaleFactor()
	faceSelector := browser.NewDocumentFaceSelector()
	app := browser.NewApp(faceSelector, styleSheet, !*light, registry)

	g := &game{
		app:                 app,
		toolbarFaceSelector: fonts.NewGoSelector(),
		renderer:            ebitenbackend.New(),
		debugHit:            *debugHit,
		debugStats:          *debugStats,
		outsideWidth:        initialWindowWidth,
		outsideHeight:       initialWindowHeight,
		start:               time.Now(),
	}
	g.applyDeviceScale()

	view := app.NewView(nil, location)
	initialHeight := int(float64(initialWindowHeight) * scale)
	g.panel = whynot.NewPanel(view, image.Rect(0, g.toolbarHeight, g.width, initialHeight))
	g.panel.SetScrollbar(true)
	app.Panel = g.panel
	app.OnTitleChange = ebiten.SetWindowTitle

	g.relayout()
	g.panel.SetAnchorScrolling(false) // the app follows anchors itself (HandleEvents)

	if err := app.Open(location); err != nil {
		log.Fatal(err)
	}
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
