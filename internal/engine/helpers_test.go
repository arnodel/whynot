package engine

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"os"
	"testing"
	"time"

	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/imagecache"
	"github.com/arnodel/whynot/internal/styling"
)

// fixedHeightBlock always lays out to a fixed height regardless of width,
// for tests that want exact, predictable heights without depending on
// real font metrics.
type fixedHeightBlock struct {
	height int
}

func (b *fixedHeightBlock) GetBlockLayout(ctx Context, width int) BlockLayout {
	return NewEmptyBox(width, b.height)
}

func (b *fixedHeightBlock) Margins(ctx Context) styling.Margins {
	return styling.Margins{}
}

func (b *fixedHeightBlock) Node() *ast.Node {
	return nil
}

func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
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

// funcSource is a fetch.Source fetching with fetch, under key.
type funcSource struct {
	key   string
	fetch func(context.Context) (io.ReadCloser, error)
}

func (s funcSource) Key() string { return s.key }

func (s funcSource) Fetch(ctx context.Context) (io.ReadCloser, string, error) {
	rc, err := s.fetch(ctx)
	return rc, "", err
}

// fileImage is a fetch.Source for the file at path, keyed by path.
func fileImage(path string) fetch.Source {
	return funcSource{key: path, fetch: func(context.Context) (io.ReadCloser, error) { return os.Open(path) }}
}
