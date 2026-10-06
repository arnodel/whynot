//go:build !js

package browser

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/arnodel/whynot/fetch"
)

// absFileURL turns a command-line path into a file: URL with an
// absolute path, so it can be used as the base for resolving a
// relative link the same way an http(s) URL would be.
func absFileURL(path string) (*url.URL, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}, nil
}

// ResolveLocationArg turns text - a command-line argument, an edited
// address bar, or pasted clipboard content (see App.Paste) - into a
// location to load. It extracts an explicit protocol first (the word
// "welcome" for the built-in welcome page, http(s) and claude: as-is, file: checked
// against the local filesystem the same way a bare path is below) -
// only once text has none does it guess: an existing local file path
// turned into an absolute file: URL, or - if it instead looks like a
// bare domain (see looksLikeHost) - an https:// URL.
//
// A single-letter "scheme" (e.g. "C:" in a Windows path) is never
// treated as a real protocol - url.Parse's scheme grammar happily
// accepts one, but no actual URL scheme is a single letter, and
// rejecting it here (instead of erroring "unsupported link scheme
// \"c\"") is what lets a Windows absolute path reach the file-path
// guess below at all.
func ResolveLocationArg(text string) (*url.URL, error) {
	if strings.EqualFold(text, "welcome") {
		return WelcomeURL, nil
	}

	if u, err := url.Parse(text); err == nil && len(u.Scheme) > 1 {
		switch u.Scheme {
		case "http", "https", "claude":
			return u, nil
		case "file":
			path := u.Path
			if path == "" {
				path = u.Opaque
			}
			if _, err := os.Stat(path); err != nil {
				return nil, err
			}
			return absFileURL(path)
		default:
			return nil, fmt.Errorf("unsupported link scheme %q", u.Scheme)
		}
	}

	if _, err := os.Stat(text); err == nil {
		return absFileURL(text)
	}
	if looksLikeHost(text) {
		if u, err := url.Parse("https://" + text); err == nil {
			return u, nil
		}
	}
	return nil, fmt.Errorf("%q isn't \"welcome\", a URL, an existing file path, or a domain", text)
}

// LoadDocument fetches the document at location through registry (see
// NewRegistry). Content that isn't Markdown is an error, and a web page
// is a *webPageError, which App opens in a web browser instead.
func LoadDocument(registry *fetch.Registry, location *url.URL) ([]byte, error) {
	return fetchDocument(registry, location)
}

// NewRegistry returns what documents, and the images in them, can be
// fetched from: http(s), the welcome page and its bundled files, and
// local files - beneath root, or with a nil root, anywhere on the volume
// of the file's path - and through any extra resolvers.
func NewRegistry(root *os.Root, extra ...fetch.Resolver) *fetch.Registry {
	var files fetch.Resolver = &volumeResolver{roots: map[string]*os.Root{}}
	if root != nil {
		files = fetch.FileResolver{Root: root}
	}
	return fetch.NewRegistry(append([]fetch.Resolver{
		files,
		fetch.HTTPResolver{Client: &http.Client{Timeout: httpTimeout}, AllowHTTP: true},
		welcomeResolver{},
	}, extra...)...)
}

// volumeResolver resolves file: URLs to any file on the volume of its
// path: / on Unix, or a drive such as D:\ on Windows, so a document on
// one drive can show images from that drive whatever the working
// directory.
type volumeResolver struct {
	mu    sync.Mutex
	roots map[string]*os.Root // by volume
}

func (*volumeResolver) Schemes() []string { return []string{"file"} }

func (r *volumeResolver) Resolve(u *url.URL) (fetch.Source, error) {
	p := u.Path
	if p == "" {
		p = u.Opaque
	}
	// A Windows path in a URL has a slash before its drive: /C:/dir.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	abs, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		return nil, err
	}
	root, err := r.root(filepath.VolumeName(abs) + string(filepath.Separator))
	if err != nil {
		return nil, err
	}
	return fetch.FileResolver{Root: root}.Resolve(u)
}

// root returns the Root of the directory dir, opening it the first time.
func (r *volumeResolver) root(dir string) (*os.Root, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if root, ok := r.roots[dir]; ok {
		return root, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	r.roots[dir] = root
	return root, nil
}
