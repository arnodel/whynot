package main

import (
	"flag"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

func main() {
	light := flag.Bool("light", false, "use whynot's light theme instead of the default dark one")
	debugHit := flag.Bool("debug-hit", false, "outline the box under the mouse, via View.HitTest")
	flag.Parse()

	// No file/URL given: land on the welcome page rather than a
	// hardcoded local file, matching what a person launching whynot
	// with no arguments should actually see.
	location := welcomeURL
	if flag.NArg() != 0 {
		var err error
		location, err = absFileURL(flag.Arg(0))
		if err != nil {
			panic(err)
		}
	}
	source, err := loadDocument(location)
	if err != nil {
		panic(err)
	}

	ebiten.SetWindowSize(1024, 768)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	styleSheet := whynot.StyleSheet(whynot.NewDarkStyleSheet())
	if *light {
		styleSheet = whynot.NewLightStyleSheet()
	}

	scale := ebiten.Monitor().DeviceScaleFactor()
	faceSelector := whynot.NewGoFontFaceSelector(72 * scale)
	game := &game{
		faceSelector:        faceSelector,
		toolbarFaceSelector: whynot.NewGoFontFaceSelector(72 * scale),
		styleSheet:          styleSheet,
		darkTheme:           !*light,
		renderer:            ebitenrenderer.New(),
		debugHit:            *debugHit,
		zoom:                1,
		start:               time.Now(),
	}
	game.current = document{location: location, view: game.newView(source, location)}
	game.updateWindowTitle()
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
