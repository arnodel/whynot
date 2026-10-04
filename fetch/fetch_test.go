package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubResolver resolves its schemes to Sources keyed by the URL's path.
type stubResolver struct{ schemes []string }

func (r stubResolver) Schemes() []string { return r.schemes }

func (r stubResolver) Resolve(u *url.URL) (Source, error) { return stubSource(u.Path), nil }

type stubSource string

func (s stubSource) Key() string { return string(s) }

func (s stubSource) Fetch(context.Context) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader(string(s))), "", nil
}

func mustParse(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestRegistryResolve(t *testing.T) {
	r := NewRegistry(stubResolver{[]string{"xx", "yy"}}, stubResolver{[]string{"file"}})
	base := mustParse(t, "xx://host/docs/README.md")
	for _, c := range []struct {
		base    *url.URL
		ref     string
		wantKey string
	}{
		{base, "img/a.png", "xx:/docs/img/a.png"},
		{base, "../a.png", "xx:/a.png"},
		{base, "/a.png", "xx:/a.png"},
		{base, "yy://other/b.png", "yy:/b.png"},
		{base, "YY://other/b.png", "yy:/b.png"},
		{base, "", "xx:/docs/README.md"},
		{nil, "a.png", "file:a.png"},
		{nil, "xx://host/c.png", "xx:/c.png"},
	} {
		src, err := r.Resolve(c.base, c.ref)
		if err != nil {
			t.Errorf("Resolve(%v, %q): %v", c.base, c.ref, err)
			continue
		}
		if got := src.Key(); got != c.wantKey {
			t.Errorf("Resolve(%v, %q).Key() = %q, want %q", c.base, c.ref, got, c.wantKey)
		}
	}
}

func TestRegistryNoResolver(t *testing.T) {
	for _, r := range []*Registry{nil, NewRegistry(), NewRegistry(stubResolver{[]string{"xx"}})} {
		_, err := r.Resolve(nil, "img.png")
		if !errors.Is(err, ErrNoResolver) {
			t.Errorf("Resolve of a file with no file resolver: error %v, want ErrNoResolver", err)
		}
	}
}

func TestNewRegistryPanicsOnDuplicateScheme(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewRegistry with two resolvers for one scheme didn't panic")
		}
	}()
	NewRegistry(stubResolver{[]string{"xx", "yy"}}, stubResolver{[]string{"yy"}})
}

func TestFileResolver(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	r := NewRegistry(FileResolver{Root: root})
	base := &url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, "sub", "index.md"))}

	src, err := r.Resolve(base, "../doc.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := "file:" + filepath.ToSlash(filepath.Join(dir, "doc.md")); src.Key() != want {
		t.Errorf("Key() = %q, want %q", src.Key(), want)
	}
	body, mediaType, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if data, _ := io.ReadAll(body); string(data) != "# Hi" {
		t.Errorf("Fetch read %q, want %q", data, "# Hi")
	}
	if mediaType != "text/markdown" {
		t.Errorf("media type = %q, want text/markdown", mediaType)
	}

	if _, err := r.Resolve(base, "../../outside.png"); err == nil {
		t.Error("Resolve of a path outside the root succeeded")
	}
	if _, err := NewRegistry(FileResolver{}).Resolve(base, "../doc.md"); err == nil {
		t.Error("Resolve with a nil Root succeeded")
	}
}

func TestHTTPResolverKeys(t *testing.T) {
	r := NewRegistry(HTTPResolver{AllowHTTP: true})
	for ref, want := range map[string]string{
		"https://Example.COM:443/a.png#top": "https://example.com/a.png",
		"http://example.com:80/a.png":       "http://example.com/a.png",
		"https://example.com:8443/a.png":    "https://example.com:8443/a.png",
	} {
		src, err := r.Resolve(nil, ref)
		if err != nil {
			t.Errorf("Resolve(%q): %v", ref, err)
			continue
		}
		if src.Key() != want {
			t.Errorf("Resolve(%q).Key() = %q, want %q", ref, src.Key(), want)
		}
	}
	if _, err := NewRegistry(HTTPResolver{}).Resolve(nil, "http://example.com/a.png"); !errors.Is(err, ErrNoResolver) {
		t.Errorf("plain http without AllowHTTP: error %v, want ErrNoResolver", err)
	}
}

func TestHTTPResolverFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/doc.md" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		io.WriteString(w, "# Hi")
	}))
	defer server.Close()
	r := NewRegistry(HTTPResolver{AllowHTTP: true, Client: server.Client()})

	src, err := r.Resolve(nil, server.URL+"/doc.md")
	if err != nil {
		t.Fatal(err)
	}
	body, mediaType, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if data, _ := io.ReadAll(body); string(data) != "# Hi" {
		t.Errorf("Fetch read %q, want %q", data, "# Hi")
	}
	if mediaType != "text/markdown" {
		t.Errorf("media type = %q, want text/markdown", mediaType)
	}

	missing, err := r.Resolve(nil, server.URL+"/missing.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := missing.Fetch(context.Background()); err == nil {
		t.Error("Fetch of a 404 succeeded")
	}
}
