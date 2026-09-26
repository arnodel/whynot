//go:build !js

package browser

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
// "welcome" for the built-in welcome page, http(s) as-is, file: checked
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
		case "http", "https":
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

// LoadDocument fetches the bytes at location - a local read for a
// file: URL, an HTTP GET for http(s) (see fetchDocument), the embedded
// welcome page for WelcomeURL. Any other scheme (e.g. a mailto:
// autolink) is rejected rather than misread as a file path.
func LoadDocument(location *url.URL) ([]byte, error) {
	switch location.Scheme {
	case "whynot":
		return renderWelcome(), nil
	case "http", "https":
		return fetchDocument(location)
	case "file", "":
		return os.ReadFile(location.Path)
	default:
		return nil, fmt.Errorf("unsupported link scheme %q", location.Scheme)
	}
}

// openImageLocation fetches the bytes at location - the same file-or-
// http(s) rule LoadDocument uses (see fetchImage).
func openImageLocation(location *url.URL) (io.ReadCloser, error) {
	switch location.Scheme {
	case "http", "https":
		return fetchImage(location)
	case "file", "":
		return os.Open(location.Path)
	default:
		return nil, fmt.Errorf("unsupported image scheme %q", location.Scheme)
	}
}
