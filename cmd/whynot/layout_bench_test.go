package main

import (
	"os"
	"testing"
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
