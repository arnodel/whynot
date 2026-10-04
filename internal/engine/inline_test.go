package engine

import (
	"errors"
	"image"
	"testing"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/imagecache"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
)

func TestInlineTextStrikeThickness(t *testing.T) {
	styleSheet := stylingtest.Basic()
	ctx := Context{Scale: 2, FaceSelector: fonts.NewGoSelector(), Styles: styleSheet}

	plainNode := (*ast.Node)(nil).AddChild(ast.TagParagraph).AddChild(ast.TagEmphasis)
	plain := (&InlineText{Text: "x", ASTNode: plainNode}).GetInlineLayout(ctx, NaturalWidthMeasure).(*TextBox)
	if plain.StrikeThickness != 0 {
		t.Errorf("non-struck StrikeThickness = %d, want 0", plain.StrikeThickness)
	}

	struckNode := (*ast.Node)(nil).AddChild(ast.TagParagraph).AddChild(ast.TagStrikethrough)
	struck := (&InlineText{Text: "x", ASTNode: struckNode}).GetInlineLayout(ctx, NaturalWidthMeasure).(*TextBox)
	want := int(styleSheet.StrikeThickness(struckNode) * ctx.Scale)
	if struck.StrikeThickness != want {
		t.Errorf("struck StrikeThickness = %d, want %d", struck.StrikeThickness, want)
	}
}

// TestImageGetInlineLayoutWithoutImageCache checks that with no
// image cache the image isn't loaded at all: it renders as its alt text,
// with nothing pending to revisit.
func TestImageGetInlineLayoutWithoutImageCache(t *testing.T) {
	img := &InlineImage{Src: "../../testdata/cat.jpeg", Alt: "a cat"}
	ctx := Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}

	box, ok := img.GetInlineLayout(ctx, NaturalWidthMeasure).(*TextBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *TextBox", img.GetInlineLayout(ctx, NaturalWidthMeasure))
	}
	if box.Text != "a cat" {
		t.Errorf("Text = %q, want the alt text %q", box.Text, "a cat")
	}
	if pending := box.PendingImages(); len(pending) != 0 {
		t.Errorf("PendingImages() = %v, want none", pending)
	}
}

// TestImageGetInlineLayoutScalesBounds checks an image's layout bounds
// are its native pixel size times ctx.Scale, like every other sized
// quantity in the layout system (Context.ScaledMargins and
// friends) - not the file's raw pixel size unconditionally, which would
// leave images pixel-locked against zoom/DPI scale.
func TestImageGetInlineLayoutScalesBounds(t *testing.T) {
	img := &InlineImage{Src: "../../testdata/cat.jpeg", Image: fileImage("../../testdata/cat.jpeg")} // 400x600
	// FaceSelector/Styles: the first call sees the fetch still pending
	// and falls back to text ("(loading image…)"), which needs both.
	ctx := Context{
		Scale:        2,
		ImageCache:   imagecache.NewCache(),
		FaceSelector: fonts.NewGoSelector(),
		Styles:       stylingtest.Basic(),
	}
	img.GetInlineLayout(ctx, NaturalWidthMeasure)
	waitForSettled(t, ctx.ImageCache, img.Image)
	box, ok := img.GetInlineLayout(ctx, NaturalWidthMeasure).(*ImageBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *ImageBox", img.GetInlineLayout(ctx, NaturalWidthMeasure))
	}
	want := image.Rect(0, 0, 800, 1200)
	if box.Rect != want {
		t.Errorf("bounds = %v, want %v", box.Rect, want)
	}
	if box.Image == nil {
		t.Error("img = nil, want the decoded image")
	}
}

// TestImageGetInlineLayoutFitsWidth checks an image whose ctx.Scale-d
// size would still exceed the width it's given is scaled down further,
// preserving aspect ratio (see fitWidth) - CSS's max-width: 100%,
// applied to an inline image, so a document author's own image at its
// native resolution never overflows the page.
func TestImageGetInlineLayoutFitsWidth(t *testing.T) {
	img := &InlineImage{Src: "../../testdata/cat.jpeg", Image: fileImage("../../testdata/cat.jpeg")} // 400x600
	ctx := Context{
		Scale:        1,
		ImageCache:   imagecache.NewCache(),
		FaceSelector: fonts.NewGoSelector(),
		Styles:       stylingtest.Basic(),
	}
	img.GetInlineLayout(ctx, 200)
	waitForSettled(t, ctx.ImageCache, img.Image)
	box, ok := img.GetInlineLayout(ctx, 200).(*ImageBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *ImageBox", img.GetInlineLayout(ctx, 200))
	}
	want := image.Rect(0, 0, 200, 300)
	if box.Rect != want {
		t.Errorf("bounds = %v, want %v", box.Rect, want)
	}
}

// TestImageGetInlineLayoutFallsBackWhenMissing checks a missing/unreadable
// image renders as flagged fallback text (see appendUnsupportedInline)
// instead of a silent zero-size gap, preferring alt text over title over
// a generic message naming the resolved (here: unresolved-any-further,
// since FileImageLoader doesn't resolve) source.
func TestImageGetInlineLayoutFallsBackWhenMissing(t *testing.T) {
	styleSheet := stylingtest.Basic()
	ctx := Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       styleSheet,
		ImageCache:   imagecache.NewCache(),
	}
	fallbackNode := (*ast.Node)(nil).AddChild(ast.TagImage).AddChild(ast.TagUnsupported)

	// All three cases below share the src "nope.png" (only alt/title
	// differ), so waiting once here - before any of them look at the
	// result - is enough: the rest hit the already-settled cache entry
	// directly.
	nope := fileImage("nope.png")
	waitForSettled(t, ctx.ImageCache, nope)

	for _, tc := range []struct {
		name string
		img  *InlineImage
		want string
	}{
		{"alt wins", &InlineImage{Src: "nope.png", Image: nope, Alt: "a lovely cat", Title: "title", FallbackNode: fallbackNode}, "a lovely cat"},
		{"title when no alt", &InlineImage{Src: "nope.png", Image: nope, Title: "a lovely cat", FallbackNode: fallbackNode}, "a lovely cat"},
		{"generic message when neither", &InlineImage{Src: "nope.png", Image: nope, FallbackNode: fallbackNode}, "(image not found: nope.png)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box, ok := tc.img.GetInlineLayout(ctx, NaturalWidthMeasure).(*TextBox)
			if !ok {
				t.Fatalf("GetInlineLayout returned %T, want *TextBox", tc.img.GetInlineLayout(ctx, NaturalWidthMeasure))
			}
			if box.Text != tc.want {
				t.Errorf("Text = %q, want %q", box.Text, tc.want)
			}
			if box.Color != styleSheet.UnsupportedColor {
				t.Errorf("Color = %v, want UnsupportedColor %v", box.Color, styleSheet.UnsupportedColor)
			}
		})
	}
}

// TestImageGetInlineLayoutUnresolved checks that an image whose src
// couldn't be resolved shows why, in its fallback text, without touching
// the cache.
func TestImageGetInlineLayoutUnresolved(t *testing.T) {
	ctx := Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       stylingtest.Basic(),
		ImageCache:   imagecache.NewCache(),
	}
	img := &InlineImage{Src: "a.png", ImageErr: errors.New("no resolver for file: URLs")}
	box, ok := img.GetInlineLayout(ctx, NaturalWidthMeasure).(*TextBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *TextBox", img.GetInlineLayout(ctx, NaturalWidthMeasure))
	}
	if want := "(image not loaded: no resolver for file: URLs)"; box.Text != want {
		t.Errorf("Text = %q, want %q", box.Text, want)
	}
	if changes, _ := ctx.ImageCache.ChangedSince(0); len(changes) != 0 {
		t.Errorf("cache changes = %v, want none", changes)
	}
}

// TestImageGetInlineLayoutAnimated checks that an animated GIF src
// produces an ImageBox with anim set (not img), with bounds scaled
// from the animation's own (shared, per-frame) size.
func TestImageGetInlineLayoutAnimated(t *testing.T) {
	img := &InlineImage{Src: "../../testdata/animated.gif", Image: fileImage("../../testdata/animated.gif")} // 64x64
	ctx := Context{
		Scale:        2,
		ImageCache:   imagecache.NewCache(),
		FaceSelector: fonts.NewGoSelector(),
		Styles:       stylingtest.Basic(),
	}
	img.GetInlineLayout(ctx, NaturalWidthMeasure)
	waitForSettled(t, ctx.ImageCache, img.Image)

	box, ok := img.GetInlineLayout(ctx, NaturalWidthMeasure).(*ImageBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *ImageBox", img.GetInlineLayout(ctx, NaturalWidthMeasure))
	}
	if box.Animation == nil {
		t.Fatal("anim = nil, want the decoded imagecache.Animation")
	}
	if box.Image != nil {
		t.Errorf("img = %v, want nil for an animated GIF", box.Image)
	}
	want := image.Rect(0, 0, 128, 128) // 64x64 native * scale 2
	if box.Rect != want {
		t.Errorf("bounds = %v, want %v", box.Rect, want)
	}
}
