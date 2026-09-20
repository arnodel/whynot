package main

import (
	"flag"
	"image"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

// initialWindowWidth/Height are ebiten's starting window size, and also
// what game.outsideWidth/Height are seeded to in main - so the very
// first Bounds/Scale handed to NewPanel already match what ebiten's own
// first Layout callback would report, rather than a throwaway guess
// immediately superseded.
const initialWindowWidth, initialWindowHeight = 1024, 768

// version is set via -X main.version=... at build time (see
// .goreleaser.yml) - "dev" for an ordinary local build. Shown on the
// welcome page (see renderWelcome's {{VERSION}} substitution), which
// puts it in the window title too, since that's the document's own
// first heading.
var version = "dev"

func main() {
	light := flag.Bool("light", false, "use whynot's light theme instead of the default dark one")
	debugHit := flag.Bool("debug-hit", false, "outline the box under the mouse, via View.HitTest")
	debugStats := flag.Bool("debug-stats", false, "show FPS/TPS and per-frame Update/Draw timing at startup - togglable at runtime with F regardless")
	flag.Parse()

	// No file/URL given: land on the welcome page rather than a
	// hardcoded local file, matching what a person launching whynot
	// with no arguments should actually see.
	location := welcomeURL
	if flag.NArg() != 0 {
		var err error
		location, err = resolveLocationArg(flag.Arg(0))
		if err != nil {
			panic(err)
		}
	}
	source, err := loadDocument(location)
	if err != nil {
		panic(err)
	}

	ebiten.SetWindowSize(initialWindowWidth, initialWindowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	styleSheet := whynot.StyleSheet(whynot.NewDarkStyleSheet())
	if *light {
		styleSheet = whynot.NewLightStyleSheet()
	}

	scale := ebiten.Monitor().DeviceScaleFactor()
	faceSelector := whynot.NewGoFontFaceSelector(72 * scale)
	g := &game{
		faceSelector:        faceSelector,
		toolbarFaceSelector: whynot.NewGoFontFaceSelector(72 * scale),
		styleSheet:          styleSheet,
		darkTheme:           !*light,
		renderer:            ebitenrenderer.New(),
		debugHit:            *debugHit,
		debugStats:          *debugStats,
		zoom:                1,
		outsideWidth:        initialWindowWidth,
		outsideHeight:       initialWindowHeight,
		start:               time.Now(),
	}
	g.applyDeviceScale()

	view := g.newView(source, location)
	g.panel = ebitenrenderer.NewPanel(view, g.renderer,
		image.Rect(0, g.toolbarHeight, g.width, g.height),
		ebitenrenderer.WithScrollbar(),
		// Load-bearing, not cosmetic: panel's own scrollbar color reads
		// its own remembered StyleSheet (only ever set via
		// SetStyleSheet/WithStyleSheet), not the View's - without this,
		// the scrollbar would render with panel's flat gray fallback
		// until the user's first theme toggle, even though the View
		// itself is already correctly themed via newView above.
		ebitenrenderer.WithStyleSheet(g.styleSheet),
	)
	g.panel.SetScale(g.scale)
	g.panel.OnLinkClick = g.follow
	g.panel.OnLinkHover = g.onLinkHover

	g.location = location
	g.updateWindowTitle()
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
