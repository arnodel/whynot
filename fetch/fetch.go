package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
)

// Source is where some content comes from: an image, a diagram, a
// document.
type Source interface {
	// Key identifies the content, for caching: two Sources with the same
	// Key fetch the same content. It only needs to be unique among the
	// keys of one Resolver, or of one code-block plugin, because whynot
	// namespaces them.
	Key() string

	// Fetch returns the content, and its media type if the Source knows
	// it, such as "text/markdown" or "image/png", without parameters; ""
	// otherwise. It may be slow: whynot calls it in the background, at
	// most once per Key, and cancels ctx once the content is no longer
	// needed.
	Fetch(ctx context.Context) (body io.ReadCloser, mediaType string, err error)
}

// Resolver makes the Source for an absolute URL of one of its Schemes.
// It decides the Source's key, so it can normalize the URL: clean a path,
// lowercase a host, and so on.
type Resolver interface {
	// Schemes returns the URL schemes the Resolver handles, in lower
	// case.
	Schemes() []string

	// Resolve makes the Source for u, whose scheme is one of Schemes. It
	// should be quick, leaving the slow work to the Source's Fetch.
	Resolve(u *url.URL) (Source, error)
}

// ErrNoResolver is returned by [Registry.Resolve] for a reference whose
// scheme has no Resolver.
var ErrNoResolver = errors.New("no resolver")

// Registry resolves references, such as an image's src or a link's
// destination, through the Resolver for each one's URL scheme. Nothing is
// resolved unless a Resolver for its scheme was registered: see
// [FileResolver] and [HTTPResolver].
//
// A Registry is immutable, so one can be shared between documents and
// goroutines. A nil *Registry resolves nothing.
type Registry struct {
	resolvers map[string]Resolver
}

// NewRegistry returns a Registry of resolvers. It panics if two of them
// handle the same scheme.
func NewRegistry(resolvers ...Resolver) *Registry {
	r := &Registry{resolvers: map[string]Resolver{}}
	for _, res := range resolvers {
		for _, scheme := range res.Schemes() {
			if _, dup := r.resolvers[scheme]; dup {
				panic(fmt.Sprintf("fetch: two resolvers for scheme %q", scheme))
			}
			r.resolvers[scheme] = res
		}
	}
	return r
}

// Resolve returns the Source of ref, relative to base. A relative ref
// takes base's scheme, host and directory, as in a web browser; with no
// base, a relative ref is a file path relative to the working directory.
// A Windows path such as C:\img.png is a file path, not a URL with
// scheme "c".
//
// The Source's key is its Resolver's key, prefixed with the scheme and a
// colon, so the keys of two Resolvers never collide.
func (r *Registry) Resolve(base *url.URL, ref string) (Source, error) {
	u, err := absolute(base, ref)
	if err != nil {
		return nil, err
	}
	var res Resolver
	if r != nil {
		res = r.resolvers[u.Scheme]
	}
	if res == nil {
		return nil, fmt.Errorf("%w for %s: URLs: %s", ErrNoResolver, u.Scheme, ref)
	}
	src, err := res.Resolve(u)
	if err != nil {
		return nil, err
	}
	return prefixed{prefix: u.Scheme + ":", Source: src}, nil
}

// absolute resolves ref against base, as Resolve describes.
func absolute(base *url.URL, ref string) (*url.URL, error) {
	u, err := url.Parse(ref)
	// No real scheme is a single letter, but a Windows drive looks like
	// one to url.Parse.
	if err != nil || len(u.Scheme) == 1 {
		if filepath.VolumeName(ref) == "" {
			if err == nil {
				err = fmt.Errorf("invalid URL scheme %q", u.Scheme)
			}
			return nil, err
		}
		return &url.URL{Scheme: "file", Path: filepath.ToSlash(ref)}, nil
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme == "" {
		u.Scheme = "file"
	}
	u.Scheme = strings.ToLower(u.Scheme)
	return u, nil
}

// prefixed is a Source whose key is namespaced by prefix.
type prefixed struct {
	prefix string
	Source
}

func (p prefixed) Key() string { return p.prefix + p.Source.Key() }
