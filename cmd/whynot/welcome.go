package main

import (
	"bytes"
	_ "embed"
	"net/url"
	"runtime"
)

//go:embed welcome.md
var welcomeMD []byte

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

// renderWelcome fills in welcomeMD's one platform-specific placeholder.
func renderWelcome() []byte {
	return bytes.ReplaceAll(welcomeMD, []byte("{{PASTE_SHORTCUT}}"), []byte(welcomeShortcut()))
}
