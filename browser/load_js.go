package browser

import (
	"fmt"
	"io"
	"net/url"
	"strings"
)

// ResolveLocationArg is the browser build's counterpart to
// load_notjs.go's: no local file path support, since there's no real
// filesystem to check in a browser sandbox - just the word "welcome"
// or an http(s) URL.
func ResolveLocationArg(text string) (*url.URL, error) {
	if strings.EqualFold(text, "welcome") {
		return WelcomeURL, nil
	}
	if u, err := url.Parse(text); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return u, nil
	}
	return nil, fmt.Errorf("%q isn't \"welcome\" or a URL", text)
}

// LoadDocument is load_notjs.go's counterpart, minus the "file"/""
// (local path) case - see fetchDocument for the shared http(s) GET.
func LoadDocument(location *url.URL) ([]byte, error) {
	switch location.Scheme {
	case "whynot":
		return renderWelcome(), nil
	case "http", "https":
		return fetchDocument(location)
	default:
		return nil, fmt.Errorf("unsupported link scheme %q", location.Scheme)
	}
}

// openImageLocation is load_notjs.go's counterpart, minus the "file"/""
// (local path) case - see fetchImage for the shared http(s) GET.
func openImageLocation(location *url.URL) (io.ReadCloser, error) {
	switch location.Scheme {
	case "http", "https":
		return fetchImage(location)
	default:
		return nil, fmt.Errorf("unsupported image scheme %q", location.Scheme)
	}
}
