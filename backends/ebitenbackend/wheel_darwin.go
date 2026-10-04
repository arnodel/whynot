//go:build darwin

package ebitenbackend

// On macOS, ebiten.Wheel reports a trackpad's (or a precise mouse's)
// scroll distance in points divided by 10, and an ordinary mouse wheel's
// in lines. Multiplying by 10 makes a trackpad move the content exactly
// as far as the fingers, as native apps do, and a wheel line 10 points.
// Fyne, whose desktop driver also reads GLFW's units, and Gio, which
// reads macOS's directly, scroll the same distances.
const wheelPixels = 10
