// External test package: benchmarks that need to actually draw (not just
// lay out) depend on ebitenbackend, which itself depends on whynot - an
// import cycle if this file were `package whynot` like box_test.go.
package whynot_test

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/backends/ebitenbackend"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/images"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/imagecache"
	"github.com/arnodel/whynot/styles/simpletheme"
)

func benchmarkGetBlockLayout(b *testing.B, path string) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := whynot.DocumentRoot(whynot.Parse(source))
	ctx := engine.Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       simpletheme.DarkStyleSheet.Styles(),
		ImageCache:   imagecache.NewCache(images.FileSource{}),
	}
	const width = 1024

	// Warm the font-face cache once, same as a real run after the first frame,
	// so the benchmark measures steady-state GetBlockLayout cost, not font parsing.
	block.GetBlockLayout(ctx, width)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block.GetBlockLayout(ctx, width)
	}
}

func BenchmarkGetBlockLayout(b *testing.B) {
	benchmarkGetBlockLayout(b, "testdata/test.md")
}

func BenchmarkGetBlockLayoutLarge(b *testing.B) {
	benchmarkGetBlockLayout(b, "testdata/test-large.md")
}

// benchmarkBoxBoundsWarm measures repeated Bounds() calls on an already-built
// tree, i.e. what Draw() does on every frame between resizes. If Bounds()
// memoization is working, this should cost about the same regardless of
// document size, since a warm call is just a stored-field read.
func benchmarkBoxBoundsWarm(b *testing.B, path string) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := whynot.DocumentRoot(whynot.Parse(source))
	ctx := engine.Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       simpletheme.DarkStyleSheet.Styles(),
		ImageCache:   imagecache.NewCache(images.FileSource{}),
	}
	box := block.GetBlockLayout(ctx, 1024)

	box.Bounds() // warm the cache

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		box.Bounds()
	}
}

func BenchmarkBoxBoundsWarm(b *testing.B) {
	benchmarkBoxBoundsWarm(b, "testdata/test.md")
}

func BenchmarkBoxBoundsWarmLarge(b *testing.B) {
	benchmarkBoxBoundsWarm(b, "testdata/test-large.md")
}

// benchmarkStackBoxDraw measures DrawBlockLayout cost with the viewport scrolled to
// a given fraction of the document's height, i.e. what a real frame costs:
// only the visible content gets drawn, regardless of total document size.
func benchmarkStackBoxDraw(b *testing.B, path string, offsetFraction float64) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := whynot.DocumentRoot(whynot.Parse(source))
	ctx := engine.Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       simpletheme.DarkStyleSheet.Styles(),
		ImageCache:   imagecache.NewCache(images.FileSource{}),
	}
	const width = 1024
	const viewportHeight = 768

	box := block.GetBlockLayout(ctx, width)
	totalHeight := box.Bounds().Dy()
	offsetY := -int(float64(totalHeight-viewportHeight) * offsetFraction)

	dst := ebitenbackend.New().NewCanvas(ebiten.NewImage(width, viewportHeight))

	engine.DrawBlockLayout(box, dst, 0, offsetY, 0) // warm caches

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.DrawBlockLayout(box, dst, 0, offsetY, 0)
	}
}

func BenchmarkStackBoxDrawTop(b *testing.B) {
	benchmarkStackBoxDraw(b, "testdata/test-large.md", 0)
}

func BenchmarkStackBoxDrawBottom(b *testing.B) {
	benchmarkStackBoxDraw(b, "testdata/test-large.md", 1)
}

// benchmarkStackBoxDrawOffscreen scrolls the whole document above the
// viewport, so nothing is visible and nothing ever triggers the early
// "past the viewport" break: every top-level child gets a Bounds() check
// and a skipped Draw(). This isolates the scan-and-skip cost with zero
// real drawing mixed in.
func benchmarkStackBoxDrawOffscreen(b *testing.B, path string) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := whynot.DocumentRoot(whynot.Parse(source))
	ctx := engine.Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       simpletheme.DarkStyleSheet.Styles(),
		ImageCache:   imagecache.NewCache(images.FileSource{}),
	}
	const width = 1024
	const viewportHeight = 768

	box := block.GetBlockLayout(ctx, width)
	totalHeight := box.Bounds().Dy()
	offsetY := -(totalHeight + 100000)

	dst := ebitenbackend.New().NewCanvas(ebiten.NewImage(width, viewportHeight))

	engine.DrawBlockLayout(box, dst, 0, offsetY, 0) // warm caches

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.DrawBlockLayout(box, dst, 0, offsetY, 0)
	}
}

func BenchmarkStackBoxDrawOffscreen(b *testing.B) {
	benchmarkStackBoxDrawOffscreen(b, "testdata/test.md")
}

func BenchmarkStackBoxDrawOffscreenLarge(b *testing.B) {
	benchmarkStackBoxDrawOffscreen(b, "testdata/test-large.md")
}

// benchmarkStackBoxDrawUnculled sizes dst to cover the entire document, so
// its Bounds() (the "viewport") overlaps every child: nothing gets culled,
// and every single word gets a real DrawInline call. This is what Draw()
// cost like before viewport culling, for comparison against a
// viewport-sized draw of the same document.
func benchmarkStackBoxDrawUnculled(b *testing.B, path string) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := whynot.DocumentRoot(whynot.Parse(source))
	ctx := engine.Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       simpletheme.DarkStyleSheet.Styles(),
		ImageCache:   imagecache.NewCache(images.FileSource{}),
	}
	const width = 1024

	box := block.GetBlockLayout(ctx, width)
	totalHeight := box.Bounds().Dy()
	dst := ebitenbackend.New().NewCanvas(ebiten.NewImage(width, totalHeight))

	engine.DrawBlockLayout(box, dst, 0, 0, 0) // warm caches

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.DrawBlockLayout(box, dst, 0, 0, 0)
	}
}

func BenchmarkStackBoxDrawUnculled(b *testing.B) {
	benchmarkStackBoxDrawUnculled(b, "testdata/test.md")
}

func BenchmarkStackBoxDrawUnculledLarge(b *testing.B) {
	benchmarkStackBoxDrawUnculled(b, "testdata/test-large.md")
}

// benchmarkStackBoxDrawCold measures a single Draw() call on a freshly built
// (cold) tree: the top-level slots are laid out lazily, so every one the
// Draw touches is built and measured for the first time inside the timed
// call. The culling scan calls Bounds() on every preceding sibling, even
// skipped ones, so scrolled down this includes laying out everything above
// the viewport.
func benchmarkStackBoxDrawCold(b *testing.B, path string, offsetFraction float64) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	rawBlock := whynot.DocumentRoot(whynot.Parse(source))
	ctx := engine.Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       simpletheme.DarkStyleSheet.Styles(),
		ImageCache:   imagecache.NewCache(images.FileSource{}),
	}
	const width = 1024
	const viewportHeight = 768

	// Learn the total height once, from a throwaway tree, so offsetY can be
	// fixed before the timed loop without warming the trees we're about to
	// measure.
	probeBox := rawBlock.GetBlockLayout(ctx, width)
	totalHeight := probeBox.Bounds().Dy()
	offsetY := -int(float64(totalHeight-viewportHeight) * offsetFraction)

	dst := ebitenbackend.New().NewCanvas(ebiten.NewImage(width, viewportHeight))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		box := rawBlock.GetBlockLayout(ctx, width) // fresh tree: no slot laid out yet
		b.StartTimer()

		engine.DrawBlockLayout(box, dst, 0, offsetY, 0)
	}
}

func BenchmarkStackBoxDrawColdTop(b *testing.B) {
	benchmarkStackBoxDrawCold(b, "testdata/test-large.md", 0)
}

func BenchmarkStackBoxDrawColdBottom(b *testing.B) {
	benchmarkStackBoxDrawCold(b, "testdata/test-large.md", 1)
}
