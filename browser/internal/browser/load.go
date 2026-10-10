package browser

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/fetch"
)

// httpTimeout bounds how long a document or image fetch waits for a
// server's response, so a hung server can't leave it loading forever.
const httpTimeout = 10 * time.Second

// loadWait is how long loadDocument waits for a document to arrive in
// full, so that one arriving quickly is shown whole, and a link to one of
// its headings lands there at once.
const loadWait = 100 * time.Millisecond

// loadDocument fetches the document at location through registry, and
// returns it parsed by parser: complete if it arrives within loadWait,
// otherwise growing as the rest arrives in the background (see
// whynot.Parser.Stream). It's checked to be Markdown first (see
// checkMarkdown): a body of unknown media type is read in full, to sniff
// it.
func loadDocument(parser *whynot.Parser, registry *fetch.Registry, location *url.URL) (*whynot.Document, error) {
	src, err := registry.Resolve(location, "")
	if err != nil {
		return nil, err
	}
	body, mediaType, err := src.Fetch(context.Background())
	if err != nil {
		return nil, err
	}
	base := whynot.WithBaseURL(location)
	if mediaType == "" {
		defer body.Close()
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		if err := checkMarkdown(mediaType, data, location); err != nil {
			return nil, err
		}
		return parser.Parse(data, base), nil
	}
	if err := checkMarkdown(mediaType, nil, location); err != nil {
		body.Close()
		return nil, err
	}
	doc, w := parser.Stream(base)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := io.Copy(w, body); err != nil {
			log.Printf("loading %s: %v", location, err)
		}
		body.Close()
		w.Close()
	}()
	select {
	case <-done:
	case <-time.After(loadWait):
	}
	return doc, nil
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

// webPageError means loadDocument found a web page rather than a
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
