package browser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arnodel/whynot/fetch"
)

// httpTimeout bounds every document and image fetch, so a hung server
// can't leave a document or an image loading forever.
const httpTimeout = 10 * time.Second

// fetchDocument fetches the document at location through registry,
// checking it's Markdown (see checkMarkdown).
func fetchDocument(registry *fetch.Registry, location *url.URL) ([]byte, error) {
	src, err := registry.Resolve(location, "")
	if err != nil {
		return nil, err
	}
	body, mediaType, err := src.Fetch(context.Background())
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if err := checkMarkdown(mediaType, data, location); err != nil {
		return nil, err
	}
	return data, nil
}

// checkMarkdown reports whether data, of the given media type ("" if
// unknown), can be shown as Markdown: a web page is a *webPageError, and
// any other type that isn't text is an error. An unknown type is sniffed,
// but only to catch binary content: sniffing can't tell HTML from
// Markdown, which may itself start with HTML, such as a README's
// <p align="center">.
func checkMarkdown(mediaType string, data []byte, location *url.URL) error {
	switch {
	case mediaType == "text/html" || mediaType == "application/xhtml+xml":
		return &webPageError{url: location.String()}
	case strings.HasPrefix(mediaType, "text/"):
		return nil
	case mediaType != "":
		return fmt.Errorf("%s: not Markdown (%s)", location, mediaType)
	}
	if sniffed, _, _ := strings.Cut(http.DetectContentType(data), ";"); !strings.HasPrefix(sniffed, "text/") {
		return fmt.Errorf("%s: not Markdown (%s)", location, sniffed)
	}
	return nil
}

// webPageError means LoadDocument found a web page rather than a
// Markdown document - content whose media type is HTML, or in the
// browser build, a page it isn't allowed to fetch (see load_js.go). App.Follow/Reload/Navigate open it in a web browser
// instead of just reporting an error.
type webPageError struct {
	url string
}

func (e *webPageError) Error() string {
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
