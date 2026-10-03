package whynot

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/arnodel/whynot/images"
	"github.com/arnodel/whynot/internal/imagecache"
)

// countingImageSource is an images.Source serving a fixed image (or a
// fixed error) under the key resolved, counting Image calls
// (resolveCalls) and fetches (openCalls). open, if set, replaces the
// default fetch - for tests that need to control when it returns.
type countingImageSource struct {
	resolved     string
	resolveErr   error
	data         []byte
	openErr      error
	open         func() (io.ReadCloser, error)
	resolveCalls int
	openCalls    int
}

func (s *countingImageSource) Image(src string) (images.AsyncImage, error) {
	s.resolveCalls++
	if s.resolveErr != nil {
		return images.AsyncImage{}, s.resolveErr
	}
	return images.AsyncImage{Key: s.resolved, Fetch: func(context.Context) (io.ReadCloser, error) {
		s.openCalls++
		if s.open != nil {
			return s.open()
		}
		if s.openErr != nil {
			return nil, s.openErr
		}
		return io.NopCloser(bytes.NewReader(s.data)), nil
	}}, nil
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
func waitForSettled(t *testing.T, cache *imagecache.Cache, src string) imagecache.Result {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, result := cache.Load(src)
		if result.Status != imagecache.Pending {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("Load(%q) still pending after 2s", src)
		}
		time.Sleep(time.Millisecond)
	}
}
