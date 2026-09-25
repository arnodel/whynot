package browser

import (
	"bytes"
	"image"
	"image/png"
)

// decodeIcon loads a PNG from assetsFS (assets/icons/) - decoded once
// at startup (see the icon vars below), not per frame. Returns a plain
// image.Image, not a backend-specific type: each renderer backend
// already has its own way to upload/tint an image (e.g.
// ebitenrenderer's own DrawImageOptions.ColorScale, or a Gio app's own
// paint ops), so this package stays backend-agnostic.
func decodeIcon(name string) image.Image {
	data, err := assetsFS.ReadFile("assets/icons/" + name)
	if err != nil {
		panic(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return img
}

// BackIcon/ForwardIcon/ReloadIcon/ZoomInIcon/ZoomOutIcon/DarkModeIcon/
// LightModeIcon are the toolbar buttons' icons - light, mostly-white
// silhouettes on a transparent background, so a backend can tint them
// to match a button's current state. Zoom uses plain +/- (add.png/
// remove.png), not the magnifying-glass-with-+/- alternative also on
// hand: two of those side by side read as busier than two plain marks,
// for no extra clarity - a plain +/- pair is already the established
// convention for zoom controls elsewhere (e.g. Google Maps). The theme
// icon shows only the current theme's, not the one switching would
// produce - since DarkModeIcon/LightModeIcon read as a day/night state
// indicator (like a physical light switch) rather than an action to
// perform, unlike e.g. a play/pause button.
var (
	BackIcon      = decodeIcon("arrow_back.png")
	ForwardIcon   = decodeIcon("arrow_forward.png")
	ReloadIcon    = decodeIcon("refresh.png")
	ZoomInIcon    = decodeIcon("add.png")
	ZoomOutIcon   = decodeIcon("remove.png")
	DarkModeIcon  = decodeIcon("dark_mode.png")
	LightModeIcon = decodeIcon("light_mode.png")
)
