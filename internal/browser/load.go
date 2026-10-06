package browser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

// loader reads a document's body in the background, so a document that
// arrives slowly, such as a claude: page, can be shown as it arrives.
type loader struct {
	location *url.URL
	cancel   context.CancelFunc

	finished chan struct{} // closed once the body is complete

	mu      sync.Mutex
	data    []byte
	done    bool
	changed bool // since the last take
	err     error
}

// loadWait is how long startLoad waits for a body to be complete, so a
// document that arrives quickly is shown at once, not in pieces.
const loadWait = 100 * time.Millisecond

// startLoad fetches the document at location through registry, and
// returns once its body is complete, or loadWait after it starts: the
// rest then arrives in the background. A
// body whose media type doesn't say it's text is read in full first, to
// check it's Markdown (see checkMarkdown).
func startLoad(registry *fetch.Registry, location *url.URL) (*loader, error) {
	src, err := registry.Resolve(location, "")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	body, mediaType, err := src.Fetch(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	l := &loader{location: location, cancel: cancel, changed: true, finished: make(chan struct{})}
	if mediaType == "" || checkMarkdown(mediaType, nil, location) != nil {
		defer cancel()
		defer body.Close()
		data, err := io.ReadAll(body)
		if err == nil {
			err = checkMarkdown(mediaType, data, location)
		}
		if err != nil {
			return nil, err
		}
		l.data, l.done = data, true
		return l, nil
	}
	go l.read(body)
	select {
	case <-l.finished:
	case <-time.After(loadWait):
	}
	return l, nil
}

func (l *loader) read(body io.ReadCloser) {
	defer close(l.finished)
	defer body.Close()
	buf := make([]byte, 4096)
	for {
		n, err := body.Read(buf)
		l.mu.Lock()
		l.data = append(l.data, buf[:n]...)
		l.changed = l.changed || n > 0
		if err != nil {
			l.done, l.changed = true, true
			if err != io.EOF {
				l.err = err
			}
		}
		l.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// take returns the body so far, and whether it's complete, if it changed
// since the last call; changed is false otherwise.
func (l *loader) take() (data []byte, done, changed bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.changed {
		return nil, l.done, false, nil
	}
	l.changed = false
	return bytes.Clone(l.data), l.done, true, l.err
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
