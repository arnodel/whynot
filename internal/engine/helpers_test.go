package engine

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/arnodel/whynot/internal/ast"
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
