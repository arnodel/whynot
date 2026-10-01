package whynot

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/arnodel/whynot/internal/images"
)

// countingImageSource is an ImageSource serving a fixed image (or a
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

func (s *countingImageSource) Image(src string) (AsyncImage, error) {
	s.resolveCalls++
	if s.resolveErr != nil {
		return AsyncImage{}, s.resolveErr
	}
	return AsyncImage{Key: s.resolved, Fetch: func() (io.ReadCloser, error) {
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
func waitForSettled(t *testing.T, cache *images.Cache, src string) images.Result {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, result := cache.Load(src)
		if result.Status != images.Pending {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("Load(%q) still pending after 2s", src)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNewImageCacheUsesSource checks the cache resolves srcs through the
// ImageSource: its Key identifies the image, and its Fetch loads it.
func TestNewImageCacheUsesSource(t *testing.T) {
	source := &countingImageSource{resolved: "resolved.png", data: onePixelPNG(t)}
	cache := newImageCache(source)

	result := waitForSettled(t, cache, "src.png")
	if result.Status != images.Ready {
		t.Fatalf("status = %v, want Ready", result.Status)
	}
	if resolved, _ := cache.Load("src.png"); resolved != "resolved.png" {
		t.Errorf("resolved = %q, want the source's Key %q", resolved, "resolved.png")
	}
	if source.openCalls != 1 {
		t.Errorf("openCalls = %d, want 1", source.openCalls)
	}
}

// TestNewImageCacheSourceError checks an ImageSource error fails the
// image, naming the literal src.
func TestNewImageCacheSourceError(t *testing.T) {
	cache := newImageCache(&countingImageSource{resolveErr: errors.New("bad src")})
	resolved, result := cache.Load("src.png")
	if result.Status != images.Failed || resolved != "src.png" {
		t.Errorf("Load = %q, %+v; want src.png, Failed", resolved, result)
	}
}
