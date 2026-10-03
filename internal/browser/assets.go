package browser

import "embed"

// assetsFS embeds this package's own bundled files - toolbar icons
// (assets/icons/) and the welcome page (assets/welcome.md) - so a
// binary built against this package needs nothing alongside it at
// runtime. Embedding the whole tree, not just welcome.md itself, means
// an image the welcome page wants to show (e.g. assets/screenshot.png,
// referenced from welcome.md as "screenshot.png") is bundled right
// along with it - see welcomeImageSource in welcome.go.
//
//go:embed assets
var assetsFS embed.FS
