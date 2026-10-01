// Package images is how whynot gets the images a document shows: a View
// is given a Source, which turns an image's src into an AsyncImage it
// can fetch. Loading is the library's job: each image is fetched and
// decoded at most once, in the background, so a slow fetch never blocks
// layout or drawing.
package images

import (
	"io"
	"os"
)

// Source turns an image's src (its literal Markdown destination) into a
// fetchable AsyncImage: the embedder's policy for wherever that src
// actually points (relative to a document's own location, over
// http(s), from an archive, whatever it needs), kept out of the
// library's hands the same way document loading and link resolution
// are.
//
// One Source serves a whole View, asked about however many srcs the
// document's images contain - unlike an AsyncImage, which describes one
// specific image. Image is called on every layout, so it should be
// cheap (no I/O, just interpreting src against whatever base the Source
// knows about): the slow work belongs in the returned AsyncImage's
// Fetch, called at most once per distinct Key.
type Source interface {
	Image(src string) (AsyncImage, error)
}

// FileSource is the default Source: src is opened exactly as written,
// relative to the working directory, with no further resolution.
type FileSource struct{}

func (FileSource) Image(src string) (AsyncImage, error) {
	return AsyncImage{Key: src, Fetch: func() (io.ReadCloser, error) { return os.Open(src) }}, nil
}

// AsyncImage is one slow-to-produce image, cached by Key. A plain
// struct rather than an interface: Fetch is a closure, which can
// capture whatever state a producer needs (a resolved URL, a diagram's
// type and source text, ...). An ordinary Markdown ![]() image gets one
// from the View's Source; a code block plugin wanting more control than
// a GET against a resolved src (e.g. an HTTP POST, as kroki.Renderer
// uses) builds one directly.
type AsyncImage struct {
	// Key uniquely identifies this image for caching - e.g. a diagram's
	// type and source text concatenated, so recompiling identical source
	// (a resize, a reload) reuses the cached result instead of
	// re-fetching.
	Key string
	// Fetch performs the actual (possibly slow) work, returning encoded
	// image bytes in any format image.Decode has registered (PNG, JPEG
	// and GIF are). Called at most once per Key, on a background
	// goroutine.
	Fetch func() (io.ReadCloser, error)
}
