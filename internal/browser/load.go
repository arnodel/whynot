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

// httpTimeout bounds every document and image fetch, so a hung server
// can't leave a document or an image loading forever.
const httpTimeout = 10 * time.Second

// fetchDocument performs the http(s) GET both LoadDocument variants
// (load_notjs.go, load_js.go) use for that scheme - only the URL
// scheme dispatch around this differs between platforms (a local
// file: scheme on desktop, nothing on the web). An http(s) response
// whose Content-Type is HTML fails with *webPageError rather than
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
		return nil, &requestError{err: err}
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
				return nil, &webPageError{url: location.String()}
			default:
				return nil, fmt.Errorf("%s: not Markdown (Content-Type: %s)", location, mediaType)
			}
		}
	}
	return io.ReadAll(resp.Body)
}

// webPageError means LoadDocument found a web page rather than a
// Markdown document - an http(s) response whose Content-Type is HTML, or
// in the browser build, a page it isn't allowed to fetch (see
// load_js.go). App.Follow/Reload/Navigate open it in a web browser
// instead of just reporting an error.
type webPageError struct {
	url string
}

func (e *webPageError) Error() string {
	return fmt.Sprintf("%s looks like a web page, not Markdown", e.url)
}

// requestError means fetchDocument's request itself failed: no response
// at all, as opposed to an error status or an unsuitable Content-Type.
type requestError struct {
	err error
}

func (e *requestError) Error() string { return e.err.Error() }
func (e *requestError) Unwrap() error { return e.err }

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
