//go:build !js

package browser

import (
	"log"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/fonts/systemfont"
)

// NewDocumentFaceSelector resolves this platform's real UI/monospace
// fonts by name, when RegisterPreferredFont can find them - falling
// back to the bundled Go fonts (its default fallback, unchanged) for
// anything it can't. A toolbar deliberately keeps its own separate,
// plain fonts.GoSelector rather than sharing this one - chrome, not
// document content, and constructing a second systemfont.Selector
// there would mean a second, redundant host font-directory scan for a
// cosmetic-only benefit.
//
// systemfont's font-directory scanning (via adrg/xdg) has no js/wasm
// implementation at all - a hard compile error, not a graceful
// fallback - which is why this lives behind a build tag instead of
// systemfont being able to just no-op there itself; see
// facesel_js.go's own doc comment for the wasm side.
func NewDocumentFaceSelector() fonts.FaceSelector {
	faceSelector := systemfont.New()
	for _, family := range []fonts.Family{fonts.Proportional, fonts.Monospace} {
		// Not fatal: the Go fonts serve the family instead.
		if err := faceSelector.RegisterPreferredFont(family); err != nil {
			log.Print(err)
		}
	}
	return faceSelector
}
