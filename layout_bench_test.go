// External test package: benchmarks that need to actually draw (not just
// lay out) depend on ebitenrenderer, which itself depends on whynot - an
// import cycle if this file were `package whynot` like box_test.go.
package whynot_test

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

func benchmarkGetBox(b *testing.B, path string) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := whynot.Parse(source)
	ctx := whynot.RenderingContext{
		Scale:        1,
		FaceSelector: whynot.NewGoFontFaceSelector(72),
		StyleSheet:   whynot.NewDarkStyleSheet(),
	}
	const width = 1024

	// Warm the font-face cache once, same as a real run after the first frame,
	// so the benchmark measures steady-state GetBox cost, not font parsing.
	block.GetBox(ctx, width)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block.GetBox(ctx, width)
	}
}

func BenchmarkGetBox(b *testing.B) {
	benchmarkGetBox(b, "testdata/test.md")
}

func BenchmarkGetBoxLarge(b *testing.B) {
	benchmarkGetBox(b, "testdata/test-large.md")
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
	block := whynot.Parse(source)
	ctx := whynot.RenderingContext{
		Scale:        1,
		FaceSelector: whynot.NewGoFontFaceSelector(72),
		StyleSheet:   whynot.NewDarkStyleSheet(),
	}
	box := block.GetBox(ctx, 1024)

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

// benchmarkStackBoxDraw measures DrawBox cost with the viewport scrolled to
// a given fraction of the document's height, i.e. what a real frame costs:
// only the visible content gets drawn, regardless of total document size.
func benchmarkStackBoxDraw(b *testing.B, path string, offsetFraction float64) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := whynot.Parse(source)
	ctx := whynot.RenderingContext{
		Scale:        1,
		FaceSelector: whynot.NewGoFontFaceSelector(72),
		StyleSheet:   whynot.NewDarkStyleSheet(),
	}
	const width = 1024
	const viewportHeight = 768

	box := block.GetBox(ctx, width)
	totalHeight := box.Bounds().Dy()
	offsetY := -int(float64(totalHeight-viewportHeight) * offsetFraction)

	dst := ebitenrenderer.New().NewCanvas(ebiten.NewImage(width, viewportHeight))

	whynot.DrawBox(box, dst, 0, offsetY) // warm caches

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		whynot.DrawBox(box, dst, 0, offsetY)
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
	block := whynot.Parse(source)
	ctx := whynot.RenderingContext{
		Scale:        1,
		FaceSelector: whynot.NewGoFontFaceSelector(72),
		StyleSheet:   whynot.NewDarkStyleSheet(),
	}
	const width = 1024
	const viewportHeight = 768

	box := block.GetBox(ctx, width)
	totalHeight := box.Bounds().Dy()
	offsetY := -(totalHeight + 100000)

	dst := ebitenrenderer.New().NewCanvas(ebiten.NewImage(width, viewportHeight))

	whynot.DrawBox(box, dst, 0, offsetY) // warm caches

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		whynot.DrawBox(box, dst, 0, offsetY)
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
	block := whynot.Parse(source)
	ctx := whynot.RenderingContext{
		Scale:        1,
		FaceSelector: whynot.NewGoFontFaceSelector(72),
		StyleSheet:   whynot.NewDarkStyleSheet(),
	}
	const width = 1024

	box := block.GetBox(ctx, width)
	totalHeight := box.Bounds().Dy()
	dst := ebitenrenderer.New().NewCanvas(ebiten.NewImage(width, totalHeight))

	whynot.DrawBox(box, dst, 0, 0) // warm caches

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		whynot.DrawBox(box, dst, 0, 0)
	}
}

func BenchmarkStackBoxDrawUnculled(b *testing.B) {
	benchmarkStackBoxDrawUnculled(b, "testdata/test.md")
}

func BenchmarkStackBoxDrawUnculledLarge(b *testing.B) {
	benchmarkStackBoxDrawUnculled(b, "testdata/test-large.md")
}

// benchmarkStackBoxDrawCold measures a single Draw() call on a freshly built
// (cold) tree: TextBlock/ListItemHeadBlock content is pre-warmed by
// splitBoxes during GetBox(), but CodeBlock content isn't (no line-splitting
// needed), so it stays genuinely cold until something calls Bounds()/Draw()
// on it.
// The culling scan calls Bounds() on every preceding sibling to check
// overlap, even skipped ones, so this checks whether scanning past
// off-screen code blocks on the way to a scrolled-down viewport re-triggers
// real font.BoundString measurement that a warm scan wouldn't pay for.
func benchmarkStackBoxDrawCold(b *testing.B, path string, offsetFraction float64) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	rawBlock := whynot.Parse(source)
	ctx := whynot.RenderingContext{
		Scale:        1,
		FaceSelector: whynot.NewGoFontFaceSelector(72),
		StyleSheet:   whynot.NewDarkStyleSheet(),
	}
	const width = 1024
	const viewportHeight = 768

	// Learn the total height once, from a throwaway tree, so offsetY can be
	// fixed before the timed loop without warming the trees we're about to
	// measure.
	probeBox := rawBlock.GetBox(ctx, width)
	totalHeight := probeBox.Bounds().Dy()
	offsetY := -int(float64(totalHeight-viewportHeight) * offsetFraction)

	dst := ebitenrenderer.New().NewCanvas(ebiten.NewImage(width, viewportHeight))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		box := rawBlock.GetBox(ctx, width) // fresh tree: cold CodeBlock caches
		b.StartTimer()

		whynot.DrawBox(box, dst, 0, offsetY)
	}
}

func BenchmarkStackBoxDrawColdTop(b *testing.B) {
	benchmarkStackBoxDrawCold(b, "testdata/test-large.md", 0)
}

func BenchmarkStackBoxDrawColdBottom(b *testing.B) {
	benchmarkStackBoxDrawCold(b, "testdata/test-large.md", 1)
}
