package markdown

import (
	"testing"
	"time"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/imagecache"
	"github.com/arnodel/whynot/internal/styling"
)

// parse compiles source with plugins, as whynot.Parse does.
func parse(source []byte, plugins ...codeblocks.Plugin) *Result {
	return Compile(source, plugins)
}

// fixedHeightBlock always lays out to a fixed height regardless of width,
// for tests that want exact, predictable heights without depending on
// real font metrics.
type fixedHeightBlock struct {
	height int
}

func (b *fixedHeightBlock) GetBlockLayout(ctx engine.Context, width int) engine.BlockLayout {
	return engine.NewEmptyBox(width, b.height)
}

func (b *fixedHeightBlock) Margins(ctx engine.Context) styling.Margins {
	return styling.Margins{}
}

func (b *fixedHeightBlock) Node() *ast.Node {
	return nil
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
