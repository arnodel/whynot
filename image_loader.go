package whynot

import (
	"io"
	"os"
)

// ImageLoader resolves and opens an image's source for reading -
// InlineImage.GetInlineLayout's only means of getting at an image's
// bytes. Resolution and fetching are kept out of the library's own
// hands the same way document loading and link resolution already are
// (see cmd/whynot/main.go's loadDocument/resolveLink): an embedder
// supplies the policy (relative to a document's own location, over
// http(s), from an archive, cached, whatever it needs) via
// WithImageLoader; the library only defines the seam.
type ImageLoader interface {
	// Open resolves src and returns its bytes, plus a stable, absolute
	// identifier for it (used as Canvas.DrawImage's src and a
	// renderer's cache key, so two different documents' identically
	// named relative image never collide). resolved is returned even
	// when err != nil, wherever a resolved identifier could still be
	// computed - InlineImage's fallback text names it in a missing- or
	// broken-image message.
	Open(src string) (resolved string, rc io.ReadCloser, err error)
}

// FileImageLoader is the default ImageLoader: src is opened exactly as
// written, via the OS's own (working-directory-relative) path
// resolution - today's pre-existing behavior, unchanged for any caller
// that doesn't opt into WithImageLoader.
type FileImageLoader struct{}

func (FileImageLoader) Open(src string) (string, io.ReadCloser, error) {
	rc, err := os.Open(src)
	return src, rc, err
}
