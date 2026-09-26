package browser

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// httpTimeout bounds every document/image fetch - shared by
// fetchDocument and fetchImage so a hung server can't leave the game
// loop waiting forever.
const httpTimeout = 10 * time.Second

// fetchDocument performs the http(s) GET both LoadDocument variants
// (load_notjs.go, load_js.go) use for that scheme - only the URL
// scheme dispatch around this differs between platforms (a local
// file: scheme on desktop, nothing on the web). An http(s) response
// whose Content-Type is HTML fails with *htmlContentError rather than
// being fed straight into the Markdown parser (whynot has no way to
// tell HTML apart from Markdown itself); any other clearly-non-Markdown
// Content-Type is just rejected outright, since it's not a web page
// either. A missing or unparseable Content-Type is let through - a
// heuristic, not a guarantee, since some servers omit or misreport it
// for a perfectly good Markdown file.
func fetchDocument(location *url.URL) ([]byte, error) {
	client := http.Client{Timeout: httpTimeout}
	resp, err := client.Get(location.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", location, resp.Status)
	}
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
}

// fetchImage performs the http(s) GET both openImageLocation variants
// use for that scheme - no Content-Type gate, unlike fetchDocument: an
// image's varies far more widely (image/png, image/jpeg, image/gif,
// ...) than Markdown/plain-text's narrow set, so there's nothing
// useful to check here.
func fetchImage(location *url.URL) (io.ReadCloser, error) {
	client := http.Client{Timeout: httpTimeout}
	resp, err := client.Get(location.String())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", location, resp.Status)
	}
	return resp.Body, nil
}

// htmlContentError means LoadDocument found an http(s) response whose
// Content-Type is HTML, not a fetch failure or a genuinely unreadable
// one - App.Follow/Reload/Paste offer to open the URL in the system's
// own browser instead of just reporting an error, since it's
// presumably a real webpage rather than a broken link.
type htmlContentError struct {
	url string
}

func (e *htmlContentError) Error() string {
	return fmt.Sprintf("%s looks like a web page, not Markdown", e.url)
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

// looksLikeHost reports whether s (up to its first '/', if any) looks
// like a hostname a browser's address bar would recognize without an
// explicit scheme - contains a '.' (e.g. "example.com"), or is
// "localhost" (with an optional ":port") - the same simple heuristic a
// browser's own address bar uses to tell a bare domain apart from a
// relative path or a search query.
func looksLikeHost(s string) bool {
	host := s
	if i := strings.IndexByte(s, '/'); i >= 0 {
		host = s[:i]
	}
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return host == "localhost" || strings.Contains(host, ".")
}

// docImageSource implements whynot.ImageSource by resolving an image's
// src against base (a document's own location) exactly the way
// App.ResolveLink resolves a link's href, then fetching it the same
// way LoadDocument does - so a relative or http(s) image works
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
