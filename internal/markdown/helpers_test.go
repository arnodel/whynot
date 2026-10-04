package markdown

import (
	"context"
	"errors"
	"io"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/styling"
)

// parse compiles source with plugins, as whynot.Parse does.
func parse(source []byte, plugins ...codeblocks.Plugin) *Result {
	return Compile(source, Options{Plugins: plugins})
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

// keySource is a fetch.Source that only has a key: fetching it fails.
type keySource string

func (s keySource) Key() string { return string(s) }

func (s keySource) Fetch(context.Context) (io.ReadCloser, string, error) {
	return nil, "", errors.New("keySource can't be fetched")
}
