package whynot

import (
	"image"
	"strings"
	"testing"
)

// hscrollTestView returns a View of a document with one code block much
// wider than width, laid out and drawn once, and the code block's area.
func hscrollTestView(t *testing.T, width int) (*View, hscrollArea) {
	t.Helper()
	source := "Intro.\n\n```\n" + strings.Repeat("wide ", 100) + "\nshort\n```\n"
	v := NewView(Parse([]byte(source)), NewGoFontFaceSelector(72), WithStyleSheet(noMarginStyleSheet()))
	v.Layout(width, 400, 1, 0)
	v.Draw(&recordingCanvas{bounds: image.Rect(0, 0, width, 400)}, 0, 0)
	if len(v.ctx.hscroll.areas) != 1 {
		t.Fatalf("got %d scrollable areas after Draw, want 1 (the code block)", len(v.ctx.hscroll.areas))
	}
	return v, v.ctx.hscroll.areas[0]
}

// drawnTextX draws v and returns where the text starting with prefix was
// drawn.
func drawnTextX(t *testing.T, v *View, prefix string) int {
	t.Helper()
	dst := &recordingCanvas{bounds: image.Rect(0, 0, v.width, 400)}
	v.Draw(dst, 0, 0)
	for _, dt := range dst.texts {
		if strings.HasPrefix(dt.s, prefix) {
			return dt.x
		}
	}
	t.Fatalf("no text starting with %q drawn", prefix)
	return 0
}

func TestCodeBlockScrollsOnlyWhenWider(t *testing.T) {
	ctx := RenderingContext{Scale: 1, FaceSelector: NewGoFontFaceSelector(72), StyleSheet: NewDarkStyleSheet()}
	doc := Parse([]byte("```\n" + strings.Repeat("wide ", 100) + "\n```\n"))
	code := unwrap(doc.root.blocks[0])

	if _, ok := code.GetBlockLayout(ctx, 300).(*ScrollBox); !ok {
		t.Errorf("code block laid out narrower than its lines: got %T, want *ScrollBox", code.GetBlockLayout(ctx, 300))
	}
	if _, ok := code.GetBlockLayout(ctx, naturalWidthMeasure).(*ScrollBox); ok {
		t.Error("code block laid out wider than its lines: got a *ScrollBox, want the plain layout")
	}
}

func TestViewScrollHorizontal(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	at := area.visible.Min.Add(image.Pt(10, 10))
	before := drawnTextX(t, v, "wide")

	if !v.ScrollHorizontal(at.X, at.Y, -50) {
		t.Fatal("ScrollHorizontal over the code block = false, want true")
	}
	if got := drawnTextX(t, v, "wide"); got != before-50 {
		t.Errorf("text at x=%d after scrolling by -50, want %d", got, before-50)
	}

	if v.ScrollHorizontal(area.visible.Min.X+10, area.visible.Min.Y-5, -50) {
		t.Error("ScrollHorizontal above the code block = true, want false")
	}

	v.ScrollHorizontal(at.X, at.Y, 1e6)
	if got := drawnTextX(t, v, "wide"); got != before {
		t.Errorf("text at x=%d after scrolling far back, want %d (clamped at the start)", got, before)
	}
	v.ScrollHorizontal(at.X, at.Y, -1e6)
	maxOffset := area.contentWidth - area.box.Dx()
	if got := drawnTextX(t, v, "wide"); got != before-maxOffset {
		t.Errorf("text at x=%d after scrolling far forward, want %d (clamped at the end)", got, before-maxOffset)
	}
}

func TestViewHorizontalOffsetSurvivesRelayout(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	before := drawnTextX(t, v, "wide")
	v.ScrollHorizontal(area.visible.Min.X+10, area.visible.Min.Y+10, -50)

	v.SetStyleSheet(noMarginStyleSheet())
	if got := drawnTextX(t, v, "wide"); got != before-50 {
		t.Errorf("text at x=%d after a relayout, want %d (offset kept)", got, before-50)
	}
}

func TestScrollBoxHitTestFollowsOffset(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	p := area.visible.Min.Add(image.Pt(5, area.visible.Dy()-5)) // on "short"
	isShort := func(hit Hit) bool {
		tb, ok := hit.(*TextBox)
		return ok && strings.TrimSpace(tb.Text) == "short"
	}
	if hit, _ := v.HitTest(p.X, p.Y); !isShort(hit) {
		t.Fatalf("HitTest on the second line = %T, want the \"short\" text", hit)
	}
	v.ScrollHorizontal(p.X, p.Y, -200)
	if hit, _ := v.HitTest(p.X, p.Y); isShort(hit) {
		t.Error("HitTest still finds \"short\" after scrolling it out of view")
	}
}

func TestScrollBoxFadesAndScrollbar(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	fadeWidth := hscrollFadeWidth // scale 1
	fadesAt := func(dst *recordingCanvas) (left, right bool) {
		for _, r := range dst.rects {
			if r.h != area.box.Dy() {
				continue
			}
			left = left || r.x == area.box.Min.X
			right = right || r.x+r.w == area.box.Max.X && r.x >= area.box.Max.X-fadeWidth
		}
		return left, right
	}
	thumbDrawn := func(dst *recordingCanvas) bool {
		thumb := v.ctx.hscroll.thumb(area)
		for _, r := range dst.rects {
			if image.Rect(r.x, r.y, r.x+r.w, r.y+r.h) == thumb {
				return true
			}
		}
		return false
	}
	draw := func() *recordingCanvas {
		dst := &recordingCanvas{bounds: image.Rect(0, 0, 300, 400)}
		v.Draw(dst, 0, 0)
		return dst
	}

	dst := draw()
	if left, right := fadesAt(dst); left || !right {
		t.Errorf("at the start: left fade %v, right fade %v, want only the right one", left, right)
	}
	if thumbDrawn(dst) {
		t.Error("scrollbar drawn without hovering, want it hidden")
	}

	v.Hover(area.visible.Min.X+10, area.visible.Min.Y+10)
	if !thumbDrawn(draw()) {
		t.Error("scrollbar not drawn while hovering the code block")
	}
	v.Hover(area.visible.Min.X+10, area.visible.Min.Y-5)
	if thumbDrawn(draw()) {
		t.Error("scrollbar still drawn after the pointer left the code block")
	}

	v.ScrollHorizontal(area.visible.Min.X+10, area.visible.Min.Y+10, -50)
	if left, right := fadesAt(draw()); !left || !right {
		t.Errorf("mid-scroll: left fade %v, right fade %v, want both", left, right)
	}
}

func TestInteractionDragsHorizontalScrollbar(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	in := &Interaction{View: v, Bounds: image.Rect(0, 0, 300, 400)}
	before := drawnTextX(t, v, "wide")
	thumb := v.ctx.hscroll.thumb(area)
	grab := image.Pt(thumb.Min.X+2, thumb.Min.Y+1)

	if in.DragHorizontalScrollbar(grab.X, grab.Y+100, true, true) {
		t.Error("press away from the scrollbar started a drag")
	}
	in.DragHorizontalScrollbar(grab.X, grab.Y+100, false, false)

	if !in.DragHorizontalScrollbar(grab.X, grab.Y, true, true) {
		t.Fatal("press on the thumb didn't start a drag")
	}
	if !in.DragHorizontalScrollbar(grab.X+40, grab.Y+30, true, false) {
		t.Fatal("moving while pressed isn't part of the drag")
	}
	if got := drawnTextX(t, v, "wide"); got >= before {
		t.Errorf("text at x=%d after dragging the thumb right, want less than %d (content scrolled left)", got, before)
	}
	if !in.DragHorizontalScrollbar(grab.X+40, grab.Y+30, false, false) {
		t.Error("the release ending the drag isn't reported as part of it")
	}
	if in.DragHorizontalScrollbar(grab.X+40, grab.Y+30, true, false) {
		t.Error("still dragging after the release")
	}
}

// TestScrollbarStaysOnScreenForTallBlock checks that a code block taller
// than the viewport shows its scrollbar at the bottom of the viewport,
// not at its own off-screen bottom.
func TestScrollbarStaysOnScreenForTallBlock(t *testing.T) {
	source := "```\n" + strings.Repeat("wide ", 100) + "\n" + strings.Repeat("line\n", 100) + "```\n"
	v := NewView(Parse([]byte(source)), NewGoFontFaceSelector(72), WithStyleSheet(noMarginStyleSheet()))
	v.Layout(300, 400, 1, 0)
	v.Draw(&recordingCanvas{bounds: image.Rect(0, 0, 300, 400)}, 0, 0)
	area := v.ctx.hscroll.areas[0]
	if area.box.Max.Y <= 400 {
		t.Fatalf("test setup: code block ends at y=%d, want below the 400px viewport", area.box.Max.Y)
	}

	thumb := v.ctx.hscroll.thumb(area)
	if thumb.Max.Y > 400 || thumb.Min.Y < 0 {
		t.Errorf("thumb at %v, want within the 400px viewport", thumb)
	}
	in := &Interaction{View: v, Bounds: image.Rect(0, 0, 300, 400)}
	if !in.DragHorizontalScrollbar(thumb.Min.X+2, thumb.Min.Y+1, true, true) {
		t.Error("press on the on-screen thumb didn't start a drag")
	}
}

// TestTableScrollsWhenTooWide checks that a table whose cells can't wrap
// narrow enough - one holds an unbreakable word - scrolls sideways,
// while one that fits doesn't.
func TestTableScrollsWhenTooWide(t *testing.T) {
	ctx := RenderingContext{Scale: 1, FaceSelector: NewGoFontFaceSelector(72), StyleSheet: NewDarkStyleSheet()}
	table := func(cell string) Block {
		return unwrap(Parse([]byte("| A | B |\n|---|---|\n| " + cell + " | x |\n")).root.blocks[0])
	}
	if _, ok := table(strings.Repeat("unbreakable", 20)).GetBlockLayout(ctx, 300).(*ScrollBox); !ok {
		t.Error("table with an unbreakable word wider than the page: not a *ScrollBox")
	}
	if _, ok := table("short").GetBlockLayout(ctx, 300).(*ScrollBox); ok {
		t.Error("table that fits: got a *ScrollBox, want the plain table")
	}
}
