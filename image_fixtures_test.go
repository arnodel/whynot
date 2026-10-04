package whynot

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/imagecache"
)

// testImage is a fetch.Source serving fixed data (or a fixed error)
// under key, counting its fetches. open, if set, replaces the default
// fetch - for tests that need to control when it returns.
type testImage struct {
	key        string
	data       []byte
	openErr    error
	open       func() (io.ReadCloser, error)
	fetchCalls atomic.Int32
}

func (s *testImage) Key() string { return s.key }

func (s *testImage) Fetch(context.Context) (io.ReadCloser, string, error) {
	s.fetchCalls.Add(1)
	if s.open != nil {
		rc, err := s.open()
		return rc, "", err
	}
	if s.openErr != nil {
		return nil, "", s.openErr
	}
	return io.NopCloser(bytes.NewReader(s.data)), "", nil
}

// testResolver resolves file: URLs to img, or, if img is nil, to an
// image keyed by the URL's path that has no data.
type testResolver struct{ img *testImage }

func (testResolver) Schemes() []string { return []string{"file"} }

func (r testResolver) Resolve(u *url.URL) (fetch.Source, error) {
	if r.img != nil {
		return r.img, nil
	}
	return &testImage{key: u.Path}, nil
}

// withTestImage is a ParseOption resolving every image src to img.
func withTestImage(img *testImage) ParseOption {
	return WithImageRegistry(fetch.NewRegistry(testResolver{img}))
}

func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// blockingReader blocks on the first Read until release is closed,
// then reads from rest normally.
type blockingReader struct {
	release <-chan struct{}
	rest    io.Reader
}

func (r blockingReader) Read(p []byte) (int, error) {
	<-r.release
	return r.rest.Read(p)
}

// waitForSettled polls cache.Load(src) until it's no longer pending,
// failing the test after 2s - fetches run on their own goroutine.
func waitForSettled(t *testing.T, cache *imagecache.Cache, src fetch.Source) imagecache.Result {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		result := cache.Load(src)
		if result.Status != imagecache.Pending {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("Load(%q) still pending after 2s", src.Key())
		}
		time.Sleep(time.Millisecond)
	}
}
