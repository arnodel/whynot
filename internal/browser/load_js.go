package browser

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/arnodel/whynot/fetch"
)

// ResolveLocationArg is the browser build's counterpart to
// load_notjs.go's: no local file path support, since there's no real
// filesystem to check in a browser sandbox - just the word "welcome",
// an explicit http(s) URL, or (see looksLikeHost) a bare domain guessed
// as https://.
func ResolveLocationArg(text string) (*url.URL, error) {
	if strings.EqualFold(text, "welcome") {
		return WelcomeURL, nil
	}

	// len(u.Scheme) > 1 - see load_notjs.go's own ResolveLocationArg on
	// why a single-letter "scheme" isn't treated as one.
	if u, err := url.Parse(text); err == nil && len(u.Scheme) > 1 {
		if u.Scheme == "http" || u.Scheme == "https" {
			return u, nil
		}
		return nil, fmt.Errorf("unsupported link scheme %q", u.Scheme)
	}

	if looksLikeHost(text) {
		if u, err := url.Parse("https://" + text); err == nil {
			return u, nil
		}
	}
	return nil, fmt.Errorf("%q isn't \"welcome\", a URL, or a domain", text)
}

// LoadDocument is load_notjs.go's counterpart, with errors as loadError
// makes them.
func LoadDocument(registry *fetch.Registry, location *url.URL) ([]byte, error) {
	source, err := fetchDocument(registry, location)
	return source, loadError(err, location)
}

// loadError makes a request that gets no response at all a
// *webPageError: most sites don't allow other sites' pages to fetch them
// (CORS), and in a browser that failure is indistinguishable from an
// unreachable server. Either way, a real browser tab is the right place
// for it: it shows the page, or why not.
func loadError(err error, location *url.URL) error {
	var reqErr *url.Error
	if errors.As(err, &reqErr) {
		return &webPageError{url: location.String()}
	}
	return err
}

// NewRegistry is load_notjs.go's counterpart, without local files: root
// is ignored.
func NewRegistry(root *os.Root, extra ...fetch.Resolver) *fetch.Registry {
	return fetch.NewRegistry(append([]fetch.Resolver{
		fetch.HTTPResolver{Client: &http.Client{Timeout: httpTimeout}, AllowHTTP: true},
		welcomeResolver{},
	}, extra...)...)
}
