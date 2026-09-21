package main

import "github.com/arnodel/whynot"

// newDocumentFaceSelector falls back to the bundled Go fonts in the
// browser - systemfont's font-directory scanning (via adrg/xdg) has no
// js/wasm implementation at all, a hard compile error rather than a
// graceful fallback, confirmed by trying - so it's not an option here,
// not just skipped for being pointless in a browser sandbox (which it
// would also be: there's no real host font directory to scan).
func newDocumentFaceSelector(scale float64) whynot.FaceSelector {
	return whynot.NewGoFontFaceSelector(72 * scale)
}
