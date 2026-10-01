package giorenderer

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/op"

	"github.com/arnodel/whynot/fonts"
)

func TestImageOpCachedByIdentity(t *testing.T) {
	r := New()
	img1 := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img2 := image.NewRGBA(image.Rect(0, 0, 4, 4))

	r.imageOp(img1)
	if len(r.imageCache) != 1 {
		t.Fatalf("imageCache len = %d after one image, want 1", len(r.imageCache))
	}
	r.imageOp(img1)
	if len(r.imageCache) != 1 {
		t.Errorf("imageCache len = %d after re-asking for the same image, want still 1", len(r.imageCache))
	}
	r.imageOp(img2)
	if len(r.imageCache) != 2 {
		t.Errorf("imageCache len = %d after a second distinct image, want 2", len(r.imageCache))
	}
}

// TestGlyphForSpaceIsSkipped pins a real bug: face.Glyph(' ') returns
// ok=true with an empty (0x0) dr, not ok=false - painting a zero-sized
// paint.ImageOp for it produced visible garbage (the previous glyph,
// redrawn small) rather than nothing. glyphFor must treat an empty dr
// the same as !ok, so DrawText only advances the pen for a space
// instead of adding a paint op at all.
func TestGlyphForSpaceIsSkipped(t *testing.T) {
	face, err := fonts.NewGoSelector().SelectFace(fonts.TextStyle{Size: 16}, 72)
	if err != nil {
		t.Fatal(err)
	}
	r := New()
	if _, ok := r.glyphFor(face, ' ', color.White); ok {
		t.Error("glyphFor(' ') ok = true, want false (nothing to paint)")
	}
	if len(r.glyphCache) != 0 {
		t.Errorf("glyphCache len = %d after glyphFor(' '), want 0 (not cached)", len(r.glyphCache))
	}
}

func TestGlyphCachedByFaceRuneColor(t *testing.T) {
	face, err := fonts.NewGoSelector().SelectFace(fonts.TextStyle{Size: 16}, 72)
	if err != nil {
		t.Fatal(err)
	}
	r := New()

	if _, ok := r.glyphFor(face, 'A', color.White); !ok {
		t.Fatal("glyphFor('A') ok = false, want true for a bundled Go font")
	}
	if len(r.glyphCache) != 1 {
		t.Fatalf("glyphCache len = %d after one glyph, want 1", len(r.glyphCache))
	}

	r.glyphFor(face, 'A', color.White)
	if len(r.glyphCache) != 1 {
		t.Errorf("glyphCache len = %d after re-asking for the same (face, rune, color), want still 1", len(r.glyphCache))
	}

	r.glyphFor(face, 'B', color.White)
	if len(r.glyphCache) != 2 {
		t.Errorf("glyphCache len = %d after a different rune, want 2", len(r.glyphCache))
	}

	r.glyphFor(face, 'A', color.Black)
	if len(r.glyphCache) != 3 {
		t.Errorf("glyphCache len = %d after the same rune in a different color, want 3", len(r.glyphCache))
	}
}

func TestCanvasClipBounds(t *testing.T) {
	var ops op.Ops
	c := New().NewCanvas(&ops, image.Rect(0, 0, 100, 100))
	clipped := c.Clip(image.Rect(50, -10, 200, 40))
	if want := image.Rect(50, 0, 100, 40); clipped.Bounds() != want {
		t.Errorf("Clip bounds = %v, want %v (intersected with the canvas)", clipped.Bounds(), want)
	}
	if again := clipped.Clip(image.Rect(0, 0, 60, 60)).Bounds(); again != image.Rect(50, 0, 60, 40) {
		t.Errorf("nested Clip bounds = %v, want %v", again, image.Rect(50, 0, 60, 40))
	}
}
