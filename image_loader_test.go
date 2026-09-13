package whynot

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"testing"
)

// countingImageSource wraps a fixed image (or a fixed error) behind an
// ImageSource that counts how many times Resolve/Open were actually
// called - what the tests below use to check ImageCache.Load only
// resolves/fetches/decodes once per resolved src, however many times
// it's asked for.
type countingImageSource struct {
	resolved     string
	resolveErr   error
	data         []byte
	openErr      error
	resolveCalls int
	openCalls    int
}

func (s *countingImageSource) Resolve(src string) (string, error) {
	s.resolveCalls++
	if s.resolveErr != nil {
		return src, s.resolveErr
	}
	return s.resolved, nil
}

func (s *countingImageSource) Open(resolved string) (io.ReadCloser, error) {
	s.openCalls++
	if s.openErr != nil {
		return nil, s.openErr
	}
	return io.NopCloser(bytes.NewReader(s.data)), nil
}

func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestImageCacheLoadFetchesOnce checks that repeated Load calls for the
// same src only resolve/fetch/decode once - the whole point of
// ImageCache: every layout rebuild (a resize, a zoom change, a theme
// change, even just hovering a different link elsewhere in the
// document) calls GetInlineLayout, hence Load, again for every image in
// the document, and none of that should touch the network or disk
// again once an image has already been loaded.
func TestImageCacheLoadFetchesOnce(t *testing.T) {
	source := &countingImageSource{resolved: "resolved.png", data: onePixelPNG(t)}
	cache := NewImageCache(source)

	for i := 0; i < 5; i++ {
		resolved, img, err := cache.Load("src.png")
		if err != nil {
			t.Fatalf("Load #%d: %v", i, err)
		}
		if resolved != "resolved.png" {
			t.Errorf("Load #%d: resolved = %q, want %q", i, resolved, "resolved.png")
		}
		if img == nil {
			t.Errorf("Load #%d: img = nil", i)
		}
	}
	if source.resolveCalls != 5 {
		t.Errorf("resolveCalls = %d, want 5 (Resolve is cheap/pure - no reason to cache it)", source.resolveCalls)
	}
	if source.openCalls != 1 {
		t.Errorf("openCalls = %d, want 1 (Open/decode should happen only on the first Load)", source.openCalls)
	}
}

// TestImageCacheLoadCachesFailureToo checks that a persistently broken
// or slow src isn't retried on every Load either - the same reasoning
// as caching a success, for the same reason: a rebuild triggered by
// something as frequent as a hover shouldn't repeat an expensive (or
// hanging) fetch just because it failed last time.
func TestImageCacheLoadCachesFailureToo(t *testing.T) {
	source := &countingImageSource{resolved: "resolved.png", openErr: errors.New("boom")}
	cache := NewImageCache(source)

	for i := 0; i < 3; i++ {
		_, img, err := cache.Load("src.png")
		if err == nil {
			t.Fatalf("Load #%d: err = nil, want the cached failure", i)
		}
		if img != nil {
			t.Errorf("Load #%d: img = %v, want nil", i, img)
		}
	}
	if source.openCalls != 1 {
		t.Errorf("openCalls = %d, want 1 (a failure should be cached, not retried)", source.openCalls)
	}
}

// TestImageCacheLoadDistinguishesResolvedSrc checks two different
// literal srcs that resolve to two different identifiers are cached
// independently, not conflated.
func TestImageCacheLoadDistinguishesResolvedSrc(t *testing.T) {
	pixel := onePixelPNG(t)
	source := &stubMultiImageSource{
		byLiteral: map[string]string{"a.png": "resolved-a.png", "b.png": "resolved-b.png"},
		data:      pixel,
	}
	cache := NewImageCache(source)

	resolvedA, imgA, err := cache.Load("a.png")
	if err != nil {
		t.Fatal(err)
	}
	resolvedB, imgB, err := cache.Load("b.png")
	if err != nil {
		t.Fatal(err)
	}
	if resolvedA == resolvedB {
		t.Fatalf("both resolved to %q, want distinct identifiers", resolvedA)
	}
	if imgA == imgB {
		t.Error("both loads returned the same image.Image - want independent cache entries")
	}
	if source.openCalls != 2 {
		t.Errorf("openCalls = %d, want 2 (one per distinct resolved src)", source.openCalls)
	}
}

type stubMultiImageSource struct {
	byLiteral map[string]string
	data      []byte
	openCalls int
}

func (s *stubMultiImageSource) Resolve(src string) (string, error) {
	return s.byLiteral[src], nil
}

func (s *stubMultiImageSource) Open(resolved string) (io.ReadCloser, error) {
	s.openCalls++
	return io.NopCloser(bytes.NewReader(s.data)), nil
}
