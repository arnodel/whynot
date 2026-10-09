package whynot

import "github.com/arnodel/whynot/internal/engine"

// DocumentRoot exposes a Document's block tree to the external test
// package (layout_bench_test.go), which can't be an internal test: it
// imports ebitenbackend, which imports whynot.
func DocumentRoot(d *Document) engine.Block {
	return &engine.StackBlock{Blocks: blocksOf(d.finished)}
}
