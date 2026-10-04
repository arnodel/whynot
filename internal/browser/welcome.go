package browser

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"path"
	"runtime"

	"github.com/arnodel/whynot/images"
)

// WelcomeURL identifies the embedded welcome page - an opaque, non-file,
// non-http(s) URL so it can't collide with a real document location, but
// still a *url.URL like every other location this package handles (so it
// flows through App's history, address bar, and reload unremarkably).
// Typed as literally "welcome" (see App.Paste and the page's own text)
// rather than something like a query parameter, since it's meant to be
// memorable enough to paste from memory, not just discovered by clicking
// a link.
var WelcomeURL = &url.URL{Scheme: "whynot", Opaque: "welcome"}

// Version is set via -X github.com/arnodel/whynot/internal/browser.Version=...
// at build time (see .goreleaser.yml) - "dev" for an ordinary local
// build. Shown on the welcome page (see renderWelcome's {{VERSION}}
// substitution), which puts it in the window title too, since that's
// the document's own first heading.
var Version = "dev"

// AddressBarEditable adds a bullet to the welcome page's "Open a
// document" section mentioning the address bar can be clicked and
// typed into - false by default, since the welcome page is shared
// between
// cmd/whynot (read-only address bar) and backends/giobackend/cmd/giowhynot (editable); set
// this to true before the first call to LoadDocument(WelcomeURL, ...)
// (i.e. at the very start of main) in a host whose address bar
// actually is editable.
var AddressBarEditable bool

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

// versionSuffix is what {{VERSION}} in the welcome page's heading, and so
// the window title, expands to: "vX.Y.Z" for a release, or "dev" plus
// the commit, if known, for any other build.
func versionSuffix() string {
	if Version != "dev" {
		return "v" + Version
	}
	if b, ok := readVCSBuild(); ok {
		return "dev " + b.short()
	}
	return Version
}

// buildLine is what {{BUILD}} expands to: a line saying which commit the
// binary was built from, and when that was, so a build can be identified
// (say, which version a web demo is serving); nothing if that isn't
// known.
func buildLine() string {
	if b, ok := readVCSBuild(); ok {
		return b.line()
	}
	return ""
}

// renderWelcome reads the embedded welcome page and fills in its
// placeholders. Panics on error - assets/welcome.md is embedded via
// assetsFS (see assets.go), so a failure here means the binary itself
// was built wrong, not something a caller can recover from at runtime.
func renderWelcome() []byte {
	md, err := assetsFS.ReadFile("assets/welcome.md")
	if err != nil {
		panic(err)
	}
	md = bytes.ReplaceAll(md, []byte("{{PASTE_SHORTCUT}}"), []byte(welcomeShortcut()))
	md = bytes.ReplaceAll(md, []byte("{{VERSION}}"), []byte(versionSuffix()))
	// Like {{ADDRESS_BAR_TIP}}, replaces the whole line.
	md = bytes.ReplaceAll(md, []byte("{{BUILD}}\n"), []byte(buildLine()))
	// Replaces the whole placeholder line, trailing newline included, so
	// a false AddressBarEditable removes the line entirely rather than
	// leaving a blank one in the middle of the bullet list.
	md = bytes.ReplaceAll(md, []byte("{{ADDRESS_BAR_TIP}}\n"), []byte(addressBarTip()))
	return md
}

func addressBarTip() string {
	if !AddressBarEditable {
		return ""
	}
	return "- Click the address bar, type a path, URL, or \"welcome\", then press Enter.\n"
}

// welcomeImageSource implements images.Source for images the
// welcome page itself references - always bundled in assetsFS
// alongside welcome.md, never fetched, so a src is just a path
// relative to assets/ (e.g. an image sitting right next to welcome.md
// is referenced from it as "screenshot.png", the same as any other
// relative image reference). Used instead of docImageSource only when
// the current document's location is WelcomeURL - see App.NewView.
type welcomeImageSource struct{}

func (welcomeImageSource) Image(src string) (images.AsyncImage, error) {
	return images.AsyncImage{Key: src, Fetch: func(context.Context) (io.ReadCloser, error) {
		data, err := assetsFS.ReadFile(path.Join("assets", src))
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}}, nil
}
