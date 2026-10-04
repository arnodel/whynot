package imagecache

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"testing"
	"time"
)

// testSource is a fetch.Source serving fixed data (or a fixed error),
// counting its fetches. open, if set, is called instead - for tests that
// need to control exactly when the fetch returns.
type testSource struct {
	key        string
	data       []byte
	openErr    error
	open       func() (io.ReadCloser, error)
	fetchCalls int
}

func (s *testSource) Key() string { return s.key }

func (s *testSource) Fetch(context.Context) (io.ReadCloser, string, error) {
	s.fetchCalls++
	if s.open != nil {
		rc, err := s.open()
		return rc, "", err
	}
	if s.openErr != nil {
		return nil, "", s.openErr
	}
	return io.NopCloser(bytes.NewReader(s.data)), "", nil
}

func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// waitForSettled polls cache.Load(src) until it's no longer Pending,
// or fails the test after a generous timeout - fetchAndDecode always
// runs on its own goroutine, so a test using a real (or realistically
// delayed) Source needs to give it a moment rather than checking
// the very first, necessarily-still-pending result.
func waitForSettled(t *testing.T, cache *Cache, src *testSource) Result {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		result := cache.Load(src)
		if result.Status != Pending {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("Load(%q) still pending after 2s", src.key)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestImageCacheLoadDoesNotBlock is the core bug this cache exists to
// fix: Load must return immediately even while Open is still running -
// the whole point of loading in the background rather than on whatever
// goroutine is doing layout.
func TestImageCacheLoadDoesNotBlock(t *testing.T) {
	release := make(chan struct{})
	source := &testSource{
		key: "img.png",
		open: func() (io.ReadCloser, error) {
			<-release
			return io.NopCloser(bytes.NewReader(onePixelPNG(t))), nil
		},
	}
	cache := NewCache()
	defer close(release)

	done := make(chan Result, 1)
	go func() {
		result := cache.Load(source)
		done <- result
	}()

	select {
	case result := <-done:
		if result.Status != Pending {
			t.Errorf("Status = %v, want Pending", result.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Load blocked instead of returning immediately")
	}
}

// TestImageCacheLoadFetchesOnce checks that repeated Load calls for the
// same key only fetch and decode once - the whole point of Cache: every
// layout rebuild (a resize, a zoom change, a theme change, even just
// hovering a different link elsewhere in the document) calls Load again
// for every image in the document, and none of that should touch the
// network or disk again once an image has already been loaded.
func TestImageCacheLoadFetchesOnce(t *testing.T) {
	source := &testSource{key: "img.png", data: onePixelPNG(t)}
	cache := NewCache()

	first := waitForSettled(t, cache, source)
	if first.Status != Ready || first.Image == nil {
		t.Fatalf("first Load settled to %+v, want Ready with an image", first)
	}

	for i := 0; i < 4; i++ {
		if result := cache.Load(source); result.Status != Ready || result.Image == nil {
			t.Errorf("Load #%d: result = %+v, want the cached Ready result", i, result)
		}
	}
	if source.fetchCalls != 1 {
		t.Errorf("fetchCalls = %d, want 1 (fetch and decode only on the first Load)", source.fetchCalls)
	}
}

// TestImageCacheHeaderPeekRevealsBoundsEarly checks that Bounds becomes
// known while the fetch is still in flight - decoded from just the
// image's header, before the rest of the body (held back by the test)
// is released - so a pending image can be laid out at its correct
// final size instead of an arbitrary placeholder.
func TestImageCacheHeaderPeekRevealsBoundsEarly(t *testing.T) {
	full := onePixelPNG(t)
	release := make(chan struct{})
	source := &testSource{
		key: "img.png",
		open: func() (io.ReadCloser, error) {
			// A PNG's IHDR chunk (dimensions) is the very first bytes of
			// the file - splitting after 33 bytes (signature + IHDR
			// chunk) guarantees DecodeConfig has everything it needs
			// without needing the rest, which release gates.
			head := full[:33]
			rest := full[33:]
			r := io.MultiReader(bytes.NewReader(head), blockingReader{release: release, rest: bytes.NewReader(rest)})
			return io.NopCloser(r), nil
		},
	}
	cache := NewCache()

	deadline := time.Now().Add(2 * time.Second)
	for {
		result := cache.Load(source)
		if result.Status == Pending && result.Bounds != (image.Rectangle{}) {
			break // bounds revealed - success, before we ever release the rest
		}
		if result.Status != Pending {
			t.Fatalf("settled to %+v before bounds were ever revealed as pending", result)
		}
		if time.Now().After(deadline) {
			t.Fatal("bounds never became known while still pending")
		}
		time.Sleep(time.Millisecond)
	}

	close(release)
	final := waitForSettled(t, cache, source)
	if final.Status != Ready {
		t.Errorf("final status = %v, want Ready", final.Status)
	}
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

// TestImageCacheLoadRetriesFailureAfterDelay checks that a failed
// fetch is retried once its retry delay has elapsed, but not before -
// a persistently broken or slow src shouldn't be hammered on every
// rebuild, but should eventually get another chance rather than
// staying broken for the rest of the session.
func TestImageCacheLoadRetriesFailureAfterDelay(t *testing.T) {
	source := &testSource{key: "img.png", openErr: errors.New("boom")}
	cache := NewCache()
	cache.retryDelay = 10 * time.Millisecond

	first := waitForSettled(t, cache, source)
	if first.Status != Failed {
		t.Fatalf("first settle = %+v, want Failed", first)
	}
	if result := cache.Load(source); result.Status != Failed {
		t.Fatalf("immediate retry = %+v, want the still-cached Failed (no retry yet)", result)
	}
	if source.fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d, want 1 before the retry delay elapses", source.fetchCalls)
	}

	time.Sleep(20 * time.Millisecond)
	second := waitForSettled(t, cache, source)
	if second.Status != Failed {
		t.Fatalf("retry settle = %+v, want Failed again (source still fails)", second)
	}
	if source.fetchCalls != 2 {
		t.Errorf("fetchCalls = %d, want 2 (one retry after the delay elapsed)", source.fetchCalls)
	}
}

// TestImageCacheChangedSince checks that changes are reported exactly
// once each, the mark advances correctly, and BoundsRevealed is true
// only for the specific change that's a size becoming known for the
// first time.
func TestImageCacheChangedSince(t *testing.T) {
	source := &testSource{key: "img.png", data: onePixelPNG(t)}
	cache := NewCache()

	changes, mark := cache.ChangedSince(0)
	if len(changes) != 0 {
		t.Fatalf("changes before any Load = %v, want none", changes)
	}

	waitForSettled(t, cache, source)

	changes, mark = cache.ChangedSince(mark)
	if len(changes) != 1 || changes[0].Key != "img.png" {
		t.Fatalf("changes = %+v, want one change for img.png", changes)
	}
	if !changes[0].BoundsRevealed {
		t.Error("BoundsRevealed = false, want true (bounds became known for the first time)")
	}

	// Nothing further happened - a repeat check should see no changes.
	changes, _ = cache.ChangedSince(mark)
	if len(changes) != 0 {
		t.Errorf("changes after settling = %v, want none (already reported)", changes)
	}
}

// TestImageCacheLoadDistinguishesKeys checks that two Sources with
// different keys are cached independently, not conflated.
func TestImageCacheLoadDistinguishesKeys(t *testing.T) {
	pixel := onePixelPNG(t)
	a := &testSource{key: "a.png", data: pixel}
	b := &testSource{key: "b.png", data: pixel}
	cache := NewCache()

	resultA := waitForSettled(t, cache, a)
	resultB := waitForSettled(t, cache, b)
	if resultA.Image == resultB.Image {
		t.Error("both loads returned the same image.Image - want independent cache entries")
	}
	if a.fetchCalls != 1 || b.fetchCalls != 1 {
		t.Errorf("fetchCalls = %d and %d, want 1 each", a.fetchCalls, b.fetchCalls)
	}
}

// twoFrameGIF returns the encoded bytes of a minimal 2-frame animated
// GIF, for tests that just need "a real animated GIF", not specific
// pixel content (see animation_test.go for compositing
// correctness).
func twoFrameGIF(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	frame := func(c uint8) *image.Paletted {
		p := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		for i := range p.Pix {
			p.Pix[i] = c
		}
		return p
	}
	g := &gif.GIF{
		Image:  []*image.Paletted{frame(0), frame(1)},
		Delay:  []int{5, 5},
		Config: image.Config{ColorModel: palette, Width: 2, Height: 2},
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestImageCacheDecodesAnimatedGIF checks that a GIF source produces
// an Result with Animation set (not Image) - Cache picks the
// decode path by the format name image.DecodeConfig already reports.
func TestImageCacheDecodesAnimatedGIF(t *testing.T) {
	source := &testSource{key: "img.gif", data: twoFrameGIF(t)}
	cache := NewCache()

	result := waitForSettled(t, cache, source)
	if result.Status != Ready {
		t.Fatalf("status = %v, want Ready", result.Status)
	}
	if result.Image != nil {
		t.Errorf("Image = %v, want nil for an animated GIF", result.Image)
	}
	if result.Animation == nil {
		t.Fatal("Animation = nil, want a decoded Animation")
	}
	if len(result.Animation.frames) != 2 {
		t.Errorf("got %d frames, want 2", len(result.Animation.frames))
	}
	if result.Bounds != image.Rect(0, 0, 2, 2) {
		t.Errorf("Bounds = %v, want (0,0)-(2,2)", result.Bounds)
	}
}
