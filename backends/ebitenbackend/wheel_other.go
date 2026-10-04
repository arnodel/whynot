//go:build !darwin && !js

package ebitenbackend

// On Windows and Linux, ebiten.Wheel reports 1 per notch of a mouse
// wheel, which has no inherent distance. 25 logical pixels, about one
// and a half lines of text, is what Fyne's desktop driver uses for the
// same units; Gio's choices differ between platforms (10 pixels per
// notch on Linux, 120 on Windows), so they're no guide.
const wheelPixels = 25
