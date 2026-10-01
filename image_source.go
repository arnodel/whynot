package whynot

import (
	"io"
	"os"

	"github.com/arnodel/whynot/internal/images"
)

// ImageSource turns an image's src (an InlineImage's literal Markdown
// destination) into a fetchable AsyncImage - the embedder-supplied
// policy for wherever that src actually points (relative to a
// document's own location, over http(s), from an archive, whatever it
// needs). Kept out of the library's own hands the same way document
// loading and link resolution already are (see
// cmd/whynot/main.go's loadDocument/resolveLink) - only the seam lives
// here. The View calls it through its image cache, never directly from
// layout.
//
// One ImageSource is configured once per View and asked about however
// many different, not-yet-seen src strings that document's images turn
// out to contain - unlike an AsyncImage, which already describes one
// specific image and needs no src parameter to say so. Image itself
// should be cheap/pure (no I/O, just interpreting src against whatever
// base the ImageSource knows about) - the real, possibly slow work
// belongs in the returned AsyncImage's own Fetch, called at most once
// per distinct key.
type ImageSource interface {
	Image(src string) (AsyncImage, error)
}

// FileImageSource is the default ImageSource: src is opened exactly as
// written (via the OS's own working-directory-relative resolution),
// with no further resolution - today's pre-existing behavior, used
// unless an embedder opts into WithImageSource.
type FileImageSource struct{}

func (FileImageSource) Image(src string) (AsyncImage, error) {
	return AsyncImage{Key: src, Fetch: func() (io.ReadCloser, error) { return os.Open(src) }}, nil
}

// AsyncImage is what the View's image cache loads: a slow-to-produce, cacheable-by-key image, already fully self-contained
// - unlike ImageSource, it describes one specific image, not a family of
// them reached via some later src parameter. A plain struct rather than
// an interface: Fetch is already a closure, which can capture whatever
// state a producer needs (a resolved URL, a diagram's own type and
// source text, ...) - there's no real polymorphism an interface would
// add here that a closure doesn't already give for free. An ordinary
// Markdown ![]() image gets one from the View's own ImageSource; a
// CodeBlockPlugin wanting more control than a GET against a resolved src
// (e.g. an HTTP POST, as kroki.Renderer uses) builds one directly, for
// use with NewDiagramBlock.
type AsyncImage struct {
	// Key uniquely identifies this image for the image cache's
	// dedup/caching - e.g. the diagram's type and source text
	// concatenated, so recompiling identical source (a resize, a
	// reload) reuses the cached result instead of re-fetching.
	Key string
	// Fetch performs the actual (possibly slow) work, returning encoded
	// image bytes in any format image.Decode already registers. Called
	// at most once per Key, on a background goroutine.
	Fetch func() (io.ReadCloser, error)
}

// newImageCache returns an image cache loading srcs through source.
func newImageCache(source ImageSource) *images.Cache {
	return images.NewCache(func(src string) (string, images.Fetch, error) {
		img, err := source.Image(src)
		return img.Key, img.Fetch, err
	})
}
