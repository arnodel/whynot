package main

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot/browser/internal/browser"
)

// toEbitenIcon converts one of browser's shared, backend-agnostic
// icons to an *ebiten.Image once at startup, not per frame.
func toEbitenIcon(img image.Image) *ebiten.Image {
	return ebiten.NewImageFromImage(img)
}

var (
	backIcon    = toEbitenIcon(browser.BackIcon)
	forwardIcon = toEbitenIcon(browser.ForwardIcon)
	reloadIcon  = toEbitenIcon(browser.ReloadIcon)
	tocIcon     = toEbitenIcon(browser.TOCIcon)
	actionsIcon = toEbitenIcon(browser.ActionsIcon)
)
