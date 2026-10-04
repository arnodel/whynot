package fetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// HTTPResolver resolves https: URLs, and http: URLs too if AllowHTTP is
// set, fetching them with a GET. A response other than 200 OK fails. Its
// Sources' media type is the response's Content-Type.
//
// Its keys are the URLs normalized: the host in lower case, without a
// default port or a fragment.
type HTTPResolver struct {
	// Client makes the requests; nil means [http.DefaultClient], which
	// has no timeout.
	Client *http.Client

	// AllowHTTP also resolves plain http: URLs, which aren't encrypted.
	AllowHTTP bool
}

func (r HTTPResolver) Schemes() []string {
	if r.AllowHTTP {
		return []string{"https", "http"}
	}
	return []string{"https"}
}

func (r HTTPResolver) Resolve(u *url.URL) (Source, error) {
	if u.Host == "" {
		return nil, fmt.Errorf("fetch: %s has no host", u)
	}
	n := *u
	n.Host = strings.ToLower(n.Host)
	if host, port, err := net.SplitHostPort(n.Host); err == nil &&
		(n.Scheme == "https" && port == "443" || n.Scheme == "http" && port == "80") {
		n.Host = host
	}
	n.Fragment, n.RawFragment = "", ""
	return httpSource{client: r.Client, url: n.String()}, nil
}

type httpSource struct {
	client *http.Client
	url    string
}

// Key leaves out the scheme, which the Registry adds.
func (s httpSource) Key() string {
	_, rest, _ := strings.Cut(s.url, ":")
	return rest
}

func (s httpSource) Fetch(ctx context.Context) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, "", err
	}
	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("%s: %s", s.url, resp.Status)
	}
	return resp.Body, mediaType(resp.Header.Get("Content-Type")), nil
}
