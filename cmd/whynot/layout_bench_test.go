package main

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func benchmarkGetBox(b *testing.B, path string) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := parseMarkdown(source)
	ctx := RenderingContext{
		Scale:        1,
		FaceSelector: NewGoFontFaceSelector(72),
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
	benchmarkGetBox(b, "test.md")
}

func BenchmarkGetBoxLarge(b *testing.B) {
	benchmarkGetBox(b, "test-large.md")
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
	block := parseMarkdown(source)
	ctx := RenderingContext{
		Scale:        1,
		FaceSelector: NewGoFontFaceSelector(72),
	}
	box := block.GetBox(ctx, 1024)

	box.Bounds() // warm the cache

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		box.Bounds()
	}
}

func BenchmarkBoxBoundsWarm(b *testing.B) {
	benchmarkBoxBoundsWarm(b, "test.md")
}

func BenchmarkBoxBoundsWarmLarge(b *testing.B) {
	benchmarkBoxBoundsWarm(b, "test-large.md")
}

// benchmarkStackBoxDraw measures DrawBox cost with the viewport scrolled to
// a given fraction of the document's height, i.e. what a real frame costs:
// only the visible content gets drawn, regardless of total document size.
func benchmarkStackBoxDraw(b *testing.B, path string, offsetFraction float64) {
	source, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	block := parseMarkdown(source)
	ctx := RenderingContext{
		Scale:        1,
		FaceSelector: NewGoFontFaceSelector(72),
	}
	const width = 1024
	const viewportHeight = 768

	box := block.GetBox(ctx, width)
	totalHeight := box.Bounds().Dy()
	offsetY := -int(float64(totalHeight-viewportHeight) * offsetFraction)

	dst := ebiten.NewImage(width, viewportHeight)

	DrawBox(box, dst, 0, offsetY) // warm caches

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DrawBox(box, dst, 0, offsetY)
	}
}

func BenchmarkStackBoxDrawTop(b *testing.B) {
	benchmarkStackBoxDraw(b, "test-large.md", 0)
}

func BenchmarkStackBoxDrawBottom(b *testing.B) {
	benchmarkStackBoxDraw(b, "test-large.md", 1)
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
	block := parseMarkdown(source)
	ctx := RenderingContext{
		Scale:        1,
		FaceSelector: NewGoFontFaceSelector(72),
	}
	const width = 1024

	box := block.GetBox(ctx, width)
	totalHeight := box.Bounds().Dy()
	dst := ebiten.NewImage(width, totalHeight)

	DrawBox(box, dst, 0, 0) // warm caches

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DrawBox(box, dst, 0, 0)
	}
}

func BenchmarkStackBoxDrawUnculled(b *testing.B) {
	benchmarkStackBoxDrawUnculled(b, "test.md")
}

func BenchmarkStackBoxDrawUnculledLarge(b *testing.B) {
	benchmarkStackBoxDrawUnculled(b, "test-large.md")
}
