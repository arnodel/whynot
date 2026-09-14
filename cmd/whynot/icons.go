package main

import (
	"bytes"
	"embed"
	"image/png"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed arrow_back.png arrow_forward.png refresh.png add.png remove.png dark_mode.png light_mode.png
var iconFS embed.FS

// icon loads a PNG embedded via iconFS into an *ebiten.Image - decoded
// once at startup (see backIcon/forwardIcon/reloadIcon below), not per
// frame.
func icon(name string) *ebiten.Image {
	data, err := iconFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return ebiten.NewImageFromImage(img)
}

// backIcon/forwardIcon/reloadIcon/zoomInIcon/zoomOutIcon/darkModeIcon/
// lightModeIcon are the toolbar buttons' icons - light, mostly-white
// silhouettes on a transparent background, so drawIcon can tint them
// to match a button's current state the same way drawButton already
// tints its border/fill. Zoom uses plain +/- (add.png/remove.png), not
// the magnifying-glass-with-+/- alternative also on hand: two of those
// side by side read as busier than two plain marks, for no extra
// clarity - a plain +/- pair is already the established convention
// for zoom controls elsewhere (e.g. Google Maps). The theme button
// shows only one icon at a time - the current theme's, not the one
// switching would produce - since darkModeIcon/lightModeIcon read as
// a day/night state indicator (like a physical light switch) rather
// than an action to perform, unlike e.g. a play/pause button.
var (
	backIcon      = icon("arrow_back.png")
	forwardIcon   = icon("arrow_forward.png")
	reloadIcon    = icon("refresh.png")
	zoomInIcon    = icon("add.png")
	zoomOutIcon   = icon("remove.png")
	darkModeIcon  = icon("dark_mode.png")
	lightModeIcon = icon("light_mode.png")
)
