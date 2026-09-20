package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// resolveLocationArg turns text - a command-line argument, or pasted
// clipboard content (see game.paste) - into a location to load: the
// word "welcome" for the built-in welcome page, an http(s) URL parsed
// as-is, or an existing local file path turned into an absolute file:
// URL - in that order, so e.g. a URL is never misread as a file path
// the way absFileURL alone would (filepath.Abs happily "resolves" any
// string, URLs included, against the working directory).
func resolveLocationArg(text string) (*url.URL, error) {
	if strings.EqualFold(text, "welcome") {
		return welcomeURL, nil
	}
	if u, err := url.Parse(text); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return u, nil
	}
	if _, err := os.Stat(text); err == nil {
		return absFileURL(text)
	}
	return nil, fmt.Errorf("%q isn't \"welcome\", a URL, or an existing file path", text)
}

// htmlContentError means loadDocument found an http(s) response whose
// Content-Type is HTML, not a fetch failure or a genuinely unreadable
// one - callers can offer to open url in the system's own browser
// instead of just reporting an error, since it's presumably a real
// webpage rather than a broken link.
type htmlContentError struct {
	url string
}

func (e *htmlContentError) Error() string {
	return fmt.Sprintf("%s looks like a web page, not Markdown", e.url)
}

// loadDocument fetches the bytes at location - a local read for a
// file: URL, an HTTP GET for http(s), the embedded page for welcomeURL.
// Any other scheme (e.g. a mailto: autolink) is rejected rather than
// misread as a file path. An http(s) response whose Content-Type is
// HTML fails with *htmlContentError rather than being fed straight into
// the Markdown parser (whynot has no way to tell HTML apart from
// Markdown itself); any other clearly-non-Markdown Content-Type is
// just rejected outright, since it's not a web page either.
func loadDocument(location *url.URL) ([]byte, error) {
	switch location.Scheme {
	case "whynot":
		return renderWelcome(), nil
	case "http", "https":
		client := http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(location.String())
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", location, resp.Status)
		}
		// A missing or unparseable Content-Type is let through - a
		// heuristic, not a guarantee, since some servers omit or
		// misreport it for a perfectly good Markdown file.
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			if mediaType, _, err := mime.ParseMediaType(ct); err == nil {
				switch mediaType {
				case "text/plain", "text/markdown":
					// Proceed - read the body below.
				case "text/html", "application/xhtml+xml":
					return nil, &htmlContentError{url: location.String()}
				default:
					return nil, fmt.Errorf("%s: not Markdown (Content-Type: %s)", location, mediaType)
				}
			}
		}
		return io.ReadAll(resp.Body)
	case "file", "":
		return os.ReadFile(location.Path)
	default:
		return nil, fmt.Errorf("unsupported link scheme %q", location.Scheme)
	}
}

// openImageLocation fetches the bytes at location - the same file-or-
// http(s) rule loadDocument uses, minus the Content-Type check: an
// image's Content-Type varies far more widely (image/png, image/jpeg,
// image/gif, ...) than Markdown/plain-text's narrow set, so there's
// nothing useful to gate on here.
func openImageLocation(location *url.URL) (io.ReadCloser, error) {
	switch location.Scheme {
	case "http", "https":
		client := http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(location.String())
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("%s: %s", location, resp.Status)
		}
		return resp.Body, nil
	case "file", "":
		return os.Open(location.Path)
	default:
		return nil, fmt.Errorf("unsupported image scheme %q", location.Scheme)
	}
}

// resolveAgainst resolves ref against base, the way a relative link or
// image src in a document is meant to be interpreted - relative to
// wherever the document itself came from, whether that's a local file
// or an http(s) URL.
func resolveAgainst(base *url.URL, ref string) (*url.URL, error) {
	target, err := url.Parse(ref)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(target), nil
}

// docImageSource implements whynot.ImageSource by resolving an image's
// src against base (a document's own location) exactly the way
// (*game).resolveLink resolves a link's href, then fetching it the same
// way loadDocument does - so a relative or http(s) image works
// regardless of where its document came from. Resolving is kept
// separate from fetching so whynot.ImageCache can cache by the
// resolved identifier without re-resolving-and-fetching on every call
// - only a genuine cache miss ever reaches Open.
type docImageSource struct {
	base *url.URL
}

func (s docImageSource) Resolve(src string) (string, error) {
	resolved, err := resolveAgainst(s.base, src)
	if err != nil {
		return src, err
	}
	return resolved.String(), nil
}

func (s docImageSource) Open(resolved string) (io.ReadCloser, error) {
	location, err := url.Parse(resolved)
	if err != nil {
		return nil, err
	}
	return openImageLocation(location)
}
