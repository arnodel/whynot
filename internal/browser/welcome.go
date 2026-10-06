package browser

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"path"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/arnodel/whynot/fetch"
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
// this to true before the welcome page is first loaded
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
// the window title, expands to: see versionFrom.
func versionSuffix() string {
	info, _ := debug.ReadBuildInfo()
	return versionFrom(Version, info)
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

// welcomeResolver resolves whynot: URLs: WelcomeURL to the welcome page,
// and any other to the files bundled in assetsFS, so the welcome page's
// images are bundled with it: an image next to welcome.md is referenced
// from it as "screenshot.png", like any other relative image.
type welcomeResolver struct{}

func (welcomeResolver) Schemes() []string { return []string{WelcomeURL.Scheme} }

func (welcomeResolver) Resolve(u *url.URL) (fetch.Source, error) {
	if u.Opaque == WelcomeURL.Opaque {
		return welcomePage{}, nil
	}
	return assetSource(path.Join("assets", strings.TrimPrefix(u.Path, "/"))), nil
}

// welcomePage is the welcome page, rendered when it's fetched.
type welcomePage struct{}

func (welcomePage) Key() string { return WelcomeURL.Opaque }

func (welcomePage) Fetch(context.Context) (io.ReadCloser, string, error) {
	return io.NopCloser(bytes.NewReader(renderWelcome())), "text/markdown", nil
}

// assetSource is a file in assetsFS, by its path.
type assetSource string

func (s assetSource) Key() string { return string(s) }

func (s assetSource) Fetch(context.Context) (io.ReadCloser, string, error) {
	f, err := assetsFS.Open(string(s))
	return f, "", err
}
