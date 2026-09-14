package main

import (
	"bytes"
	"io"
	"net/url"
	"path"
	"runtime"
)

// welcomeURL identifies the embedded welcome page - an opaque, non-file,
// non-http(s) URL so it can't collide with a real document location, but
// still a *url.URL like every other location this program handles (so it
// flows through history, the address bar, and reload unremarkably). Typed
// as literally "welcome" (see paste and its own text) rather than
// something like a query parameter, since it's meant to be memorable
// enough to paste from memory, not just discovered by clicking a link.
var welcomeURL = &url.URL{Scheme: "whynot", Opaque: "welcome"}

// welcomeShortcut is the platform's own paste shortcut, written the way
// a person would actually type it - the embedded page can't know at
// build time which OS it'll run on. Spelled out as "Cmd+V" rather than
// the ⌘ glyph: whynot only ships golang.org/x/image/font/gofont, which
// doesn't cover that symbol - it would silently render as a tofu box.
func welcomeShortcut() string {
	if runtime.GOOS == "darwin" {
		return "Cmd+V"
	}
	return "Ctrl+V"
}

// renderWelcome reads the embedded welcome page and fills in its one
// platform-specific placeholder. Panics on error - assets/welcome.md
// is embedded via assetsFS (see assets.go), so a failure here means the
// binary itself was built wrong, not something a caller can recover
// from at runtime.
func renderWelcome() []byte {
	md, err := assetsFS.ReadFile("assets/welcome.md")
	if err != nil {
		panic(err)
	}
	return bytes.ReplaceAll(md, []byte("{{PASTE_SHORTCUT}}"), []byte(welcomeShortcut()))
}

// welcomeImageSource implements whynot.ImageSource for images the
// welcome page itself references - always bundled in assetsFS
// alongside welcome.md, never fetched, so a src is just a path
// relative to assets/ (e.g. an image sitting right next to welcome.md
// is referenced from it as "screenshot.png", the same as any other
// relative image reference). Used instead of docImageSource only when
// the current document's location is welcomeURL - see newView.
type welcomeImageSource struct{}

func (welcomeImageSource) Resolve(src string) (string, error) {
	return src, nil
}

func (welcomeImageSource) Open(src string) (io.ReadCloser, error) {
	data, err := assetsFS.ReadFile(path.Join("assets", src))
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
