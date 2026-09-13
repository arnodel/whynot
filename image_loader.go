package whynot

import (
	"image"
	_ "image/gif"  // registers the GIF format with image.Decode
	_ "image/jpeg" // registers the JPEG format with image.Decode
	_ "image/png"  // registers the PNG format with image.Decode
	"io"
	"os"
)

// ImageSource resolves and fetches an image's raw bytes - the
// embedder-supplied policy for wherever an InlineImage's src actually
// points (relative to a document's own location, over http(s), from
// an archive, whatever it needs). Kept out of the library's own hands
// the same way document loading and link resolution already are (see
// cmd/whynot/main.go's loadDocument/resolveLink) - only the seam lives
// here. Always used through an ImageCache (see NewImageCache), never
// called directly by GetInlineLayout, so resolving and fetching an
// image only ever happens once no matter how many times it's asked
// for.
type ImageSource interface {
	// Resolve turns src into a stable, absolute identifier - the cache
	// key, and what a missing- or broken-image's fallback message
	// names. Pure and cheap: no I/O, just interpreting src against
	// whatever base the ImageSource knows about.
	Resolve(src string) (string, error)
	// Open fetches the bytes at a value Resolve returned.
	Open(resolved string) (io.ReadCloser, error)
}

// FileImageSource is the default ImageSource: src is opened exactly as
// written (via the OS's own working-directory-relative resolution),
// with no further resolution - today's pre-existing behavior, used
// unless an embedder opts into WithImageSource.
type FileImageSource struct{}

func (FileImageSource) Resolve(src string) (string, error) { return src, nil }

func (FileImageSource) Open(resolved string) (io.ReadCloser, error) {
	return os.Open(resolved)
}

// ImageCache wraps an ImageSource with the "management of image data"
// the library owns: an image is resolved, fetched, and decoded at
// most once, however many times Load is called for it afterwards -
// every layout rebuild (a resize, a zoom change, a theme change, even
// just hovering a different link) asks again. A Canvas implementation
// never fetches its own copy either - ImageBox carries the already-
// decoded image.Image forward from here, straight into
// Canvas.DrawImage.
//
// Failures are cached too, not just successes: a persistently broken
// or slow src shouldn't retry on every rebuild any more than a working
// one should re-fetch on every rebuild.
//
// Not safe for concurrent use, matching every other cache in this
// codebase (ebitenrenderer.Renderer's own image/font-face caches
// included) - ebiten's single-threaded game loop never needs one to
// be.
type ImageCache struct {
	source ImageSource
	cache  map[string]cachedImage
}

type cachedImage struct {
	img image.Image
	err error
}

func NewImageCache(source ImageSource) *ImageCache {
	return &ImageCache{source: source, cache: map[string]cachedImage{}}
}

// Load resolves src and returns its decoded image, fetching and
// decoding it only the first time a given resolved identifier is
// seen. resolved is still returned when err != nil, wherever Resolve
// itself succeeded - InlineImage's fallback text names it in a
// missing- or broken-image message.
func (c *ImageCache) Load(src string) (resolved string, img image.Image, err error) {
	resolved, err = c.source.Resolve(src)
	if err != nil {
		return resolved, nil, err
	}
	if entry, ok := c.cache[resolved]; ok {
		return resolved, entry.img, entry.err
	}
	img, err = c.fetchAndDecode(resolved)
	c.cache[resolved] = cachedImage{img: img, err: err}
	return resolved, img, err
}

func (c *ImageCache) fetchAndDecode(resolved string) (image.Image, error) {
	rc, err := c.source.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	img, _, err := image.Decode(rc)
	return img, err
}
