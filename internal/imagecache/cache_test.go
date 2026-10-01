package imagecache

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/arnodel/whynot/images"
)

// countingImageSource wraps a fixed image (or a fixed error) behind an
// images.Source that counts how many times Image was called
// (resolveCalls - cheap, called on every Load) and how many times the
// AsyncImage it returned was actually fetched
// (openCalls - only on a genuine cache miss). open, if set, is called
// instead of the default (returning data/openErr directly) - for tests
// that need to control exactly when the fetch returns.
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
	return images.AsyncImage{Key: s.resolved, Fetch: func() (io.ReadCloser, error) {
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

// waitForSettled polls cache.Load(src) until it's no longer Pending,
// or fails the test after a generous timeout - fetchAndDecode always
// runs on its own goroutine, so a test using a real (or realistically
// delayed) Source needs to give it a moment rather than checking
// the very first, necessarily-still-pending result.
func waitForSettled(t *testing.T, cache *Cache, src string) Result {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, result := cache.Load(src)
		if result.Status != Pending {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("Load(%q) still pending after 2s", src)
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
	source := &countingImageSource{
		resolved: "resolved.png",
		open: func() (io.ReadCloser, error) {
			<-release
			return io.NopCloser(bytes.NewReader(onePixelPNG(t))), nil
		},
	}
	cache := NewCache(source)
	defer close(release)

	done := make(chan Result, 1)
	go func() {
		_, result := cache.Load("src.png")
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
// same src only resolve/fetch/decode once - the whole point of
// Cache: every layout rebuild (a resize, a zoom change, a theme
// change, even just hovering a different link elsewhere in the
// document) calls GetInlineLayout, hence Load, again for every image in
// the document, and none of that should touch the network or disk
// again once an image has already been loaded.
func TestImageCacheLoadFetchesOnce(t *testing.T) {
	source := &countingImageSource{resolved: "resolved.png", data: onePixelPNG(t)}
	cache := NewCache(source)

	first := waitForSettled(t, cache, "src.png")
	if first.Status != Ready || first.Image == nil {
		t.Fatalf("first Load settled to %+v, want Ready with an image", first)
	}

	for i := 0; i < 4; i++ {
		resolved, result := cache.Load("src.png")
		if resolved != "resolved.png" {
			t.Errorf("Load #%d: resolved = %q, want %q", i, resolved, "resolved.png")
		}
		if result.Status != Ready || result.Image == nil {
			t.Errorf("Load #%d: result = %+v, want the cached Ready result", i, result)
		}
	}
	if source.resolveCalls < 5 {
		t.Errorf("resolveCalls = %d, want at least 5 (Resolve is cheap/pure - no reason to cache it)", source.resolveCalls)
	}
	if source.openCalls != 1 {
		t.Errorf("openCalls = %d, want 1 (Open/decode should happen only on the first fetch)", source.openCalls)
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
	source := &countingImageSource{
		resolved: "resolved.png",
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
	cache := NewCache(source)

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, result := cache.Load("src.png")
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
	final := waitForSettled(t, cache, "src.png")
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
	source := &countingImageSource{resolved: "resolved.png", openErr: errors.New("boom")}
	cache := NewCache(source)
	cache.retryDelay = 10 * time.Millisecond

	first := waitForSettled(t, cache, "src.png")
	if first.Status != Failed {
		t.Fatalf("first settle = %+v, want Failed", first)
	}
	if _, result := cache.Load("src.png"); result.Status != Failed {
		t.Fatalf("immediate retry = %+v, want the still-cached Failed (no retry yet)", result)
	}
	if source.openCalls != 1 {
		t.Fatalf("openCalls = %d, want 1 before the retry delay elapses", source.openCalls)
	}

	time.Sleep(20 * time.Millisecond)
	second := waitForSettled(t, cache, "src.png")
	if second.Status != Failed {
		t.Fatalf("retry settle = %+v, want Failed again (source still fails)", second)
	}
	if source.openCalls != 2 {
		t.Errorf("openCalls = %d, want 2 (one retry after the delay elapsed)", source.openCalls)
	}
}

// TestImageCacheChangedSince checks that changes are reported exactly
// once each, the mark advances correctly, and BoundsRevealed is true
// only for the specific change that's a size becoming known for the
// first time.
func TestImageCacheChangedSince(t *testing.T) {
	source := &countingImageSource{resolved: "resolved.png", data: onePixelPNG(t)}
	cache := NewCache(source)

	changes, mark := cache.ChangedSince(0)
	if len(changes) != 0 {
		t.Fatalf("changes before any Load = %v, want none", changes)
	}

	waitForSettled(t, cache, "src.png")

	changes, mark = cache.ChangedSince(mark)
	if len(changes) != 1 || changes[0].Src != "resolved.png" {
		t.Fatalf("changes = %+v, want one change for resolved.png", changes)
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

// TestImageCacheLoadDistinguishesResolvedSrc checks two different
// literal srcs that resolve to two different identifiers are cached
// independently, not conflated.
func TestImageCacheLoadDistinguishesResolvedSrc(t *testing.T) {
	pixel := onePixelPNG(t)
	source := &stubMultiImageSource{
		byLiteral: map[string]string{"a.png": "resolved-a.png", "b.png": "resolved-b.png"},
		data:      pixel,
	}
	cache := NewCache(source)

	resultA := waitForSettled(t, cache, "a.png")
	resultB := waitForSettled(t, cache, "b.png")
	imgA, _ := source.Image("a.png")
	imgB, _ := source.Image("b.png")
	resolvedA, resolvedB := imgA.Key, imgB.Key
	if resolvedA == resolvedB {
		t.Fatalf("both resolved to %q, want distinct identifiers", resolvedA)
	}
	if resultA.Image == resultB.Image {
		t.Error("both loads returned the same image.Image - want independent cache entries")
	}
	if source.openCalls != 2 {
		t.Errorf("openCalls = %d, want 2 (one per distinct resolved src)", source.openCalls)
	}
}

// waitForSettledImage is waitForSettled's LoadImage counterpart.
func waitForSettledImage(t *testing.T, cache *Cache, img images.AsyncImage) Result {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		result := cache.LoadImage(img)
		if result.Status != Pending {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("LoadImage(%q) still pending after 2s", img.Key)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestImageCacheLoadImageFetchesOnce mirrors TestImageCacheLoadFetchesOnce
// for LoadImage - the path a diagram drives, which has nothing to resolve,
// only its own key.
func TestImageCacheLoadImageFetchesOnce(t *testing.T) {
	pixel := onePixelPNG(t)
	var fetchCalls int
	img := images.AsyncImage{
		Key: "diagram-key",
		Fetch: func() (io.ReadCloser, error) {
			fetchCalls++
			return io.NopCloser(bytes.NewReader(pixel)), nil
		},
	}
	cache := NewCache(images.FileSource{})

	first := waitForSettledImage(t, cache, img)
	if first.Status != Ready || first.Image == nil {
		t.Fatalf("first LoadImage settled to %+v, want Ready with an image", first)
	}

	for i := 0; i < 4; i++ {
		result := cache.LoadImage(img)
		if result.Status != Ready || result.Image == nil {
			t.Errorf("LoadImage #%d: result = %+v, want the cached Ready result", i, result)
		}
	}
	if fetchCalls != 1 {
		t.Errorf("fetchCalls = %d, want 1 (fetch should happen only on the first LoadImage)", fetchCalls)
	}
}

// TestImageCacheLoadImageParticipatesInChangedSince checks that a
// LoadImage-driven entry reports through ChangedSince exactly like a
// Load-driven one - the mechanism View.invalidateChangedImages already
// polls to redraw a slot once its pending content settles, reused
// unmodified for a diagram block since both paths write into the same
// cache keyed by string.
func TestImageCacheLoadImageParticipatesInChangedSince(t *testing.T) {
	pixel := onePixelPNG(t)
	cache := NewCache(images.FileSource{})

	_, mark := cache.ChangedSince(0)
	waitForSettledImage(t, cache, images.AsyncImage{
		Key: "diagram-key",
		Fetch: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(pixel)), nil
		},
	})

	changes, _ := cache.ChangedSince(mark)
	if len(changes) != 1 || changes[0].Src != "diagram-key" {
		t.Fatalf("changes = %+v, want one change for \"diagram-key\"", changes)
	}
	if !changes[0].BoundsRevealed {
		t.Error("BoundsRevealed = false, want true (bounds became known for the first time)")
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
	source := &countingImageSource{resolved: "resolved.gif", data: twoFrameGIF(t)}
	cache := NewCache(source)

	result := waitForSettled(t, cache, "src.gif")
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

type stubMultiImageSource struct {
	byLiteral map[string]string
	data      []byte
	openCalls int
}

func (s *stubMultiImageSource) Image(src string) (images.AsyncImage, error) {
	resolved := s.byLiteral[src]
	return images.AsyncImage{Key: resolved, Fetch: func() (io.ReadCloser, error) {
		s.openCalls++
		return io.NopCloser(bytes.NewReader(s.data)), nil
	}}, nil
}
