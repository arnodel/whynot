package whynot

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/internal/canvastest"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
)

// hscrollTestView returns a View of a document with one code block much
// wider than width, laid out and drawn once, and the code block's area.
func hscrollTestView(t *testing.T, width int) (*View, drawnRegion) {
	t.Helper()
	// Filler after the code block, so the page itself can scroll too.
	source := "Intro.\n\n```\n" + strings.Repeat("wide ", 100) + "\nshort\n```\n\n" +
		strings.Repeat("Filler paragraph.\n\n", 60)
	v := NewView(Parse([]byte(source)), fonts.NewGoSelector(), stylingtest.NoViewMargin())
	v.Layout(width, 400, 1, 0)
	v.Draw(&canvastest.Recorder{Area: image.Rect(0, 0, width, 400)}, 0, 0)
	if len(v.hscroll.regions) != 1 {
		t.Fatalf("got %d scrollable areas after Draw, want 1 (the code block)", len(v.hscroll.regions))
	}
	return v, v.hscroll.regions[0]
}

// drawnTextX draws v and returns where the text starting with prefix was
// drawn.
func drawnTextX(t *testing.T, v *View, prefix string) int {
	t.Helper()
	dst := &canvastest.Recorder{Area: image.Rect(0, 0, v.width, 400)}
	v.Draw(dst, 0, 0)
	for _, dt := range dst.Texts {
		if strings.HasPrefix(dt.S, prefix) {
			return dt.X
		}
	}
	t.Fatalf("no text starting with %q drawn", prefix)
	return 0
}

func TestCodeBlockScrollsOnlyWhenWider(t *testing.T) {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	doc := Parse([]byte("```\n" + strings.Repeat("wide ", 100) + "\n```\n"))
	code := unwrap(doc.root.Blocks[0])

	if _, ok := code.GetBlockLayout(ctx, 300).(*engine.ScrollBox); !ok {
		t.Errorf("code block laid out narrower than its lines: got %T, want *ScrollBox", code.GetBlockLayout(ctx, 300))
	}
	if _, ok := code.GetBlockLayout(ctx, engine.NaturalWidthMeasure).(*engine.ScrollBox); ok {
		t.Error("code block laid out wider than its lines: got a *ScrollBox, want the plain layout")
	}
}

func TestViewScrollSideways(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	at := area.Visible.Min.Add(image.Pt(10, 10))
	before := drawnTextX(t, v, "wide")

	if !scrollHorizontal(v, at.X, at.Y, -50) {
		t.Fatal("scrollHorizontal over the code block = false, want true")
	}
	if got := drawnTextX(t, v, "wide"); got != before-50 {
		t.Errorf("text at x=%d after scrolling by -50, want %d", got, before-50)
	}

	if scrollHorizontal(v, area.Visible.Min.X+10, area.Visible.Min.Y-5, -50) {
		t.Error("scrollHorizontal above the code block = true, want false")
	}

	scrollHorizontal(v, at.X, at.Y, 1e6)
	if got := drawnTextX(t, v, "wide"); got != before {
		t.Errorf("text at x=%d after scrolling far back, want %d (clamped at the start)", got, before)
	}
	scrollHorizontal(v, at.X, at.Y, -1e6)
	maxOffset := area.ContentSize - area.Box.Dx()
	if got := drawnTextX(t, v, "wide"); got != before-maxOffset {
		t.Errorf("text at x=%d after scrolling far forward, want %d (clamped at the end)", got, before-maxOffset)
	}
}

func TestViewHorizontalOffsetSurvivesRelayout(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	before := drawnTextX(t, v, "wide")
	scrollHorizontal(v, area.Visible.Min.X+10, area.Visible.Min.Y+10, -50)

	v.SetStyleSheet(stylingtest.NoViewMargin())
	if got := drawnTextX(t, v, "wide"); got != before-50 {
		t.Errorf("text at x=%d after a relayout, want %d (offset kept)", got, before-50)
	}
}

func TestScrollBoxHitTestFollowsOffset(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	p := area.Visible.Min.Add(image.Pt(5, area.Visible.Dy()-5)) // on "short"
	isShort := func(hit engine.Hit) bool {
		tb, ok := hit.(*engine.TextBox)
		return ok && strings.TrimSpace(tb.Text) == "short"
	}
	if hit, _ := hitAt(v, p.X, p.Y); !isShort(hit) {
		t.Fatalf("HitTest on the second line = %T, want the \"short\" text", hit)
	}
	scrollHorizontal(v, p.X, p.Y, -200)
	if hit, _ := hitAt(v, p.X, p.Y); isShort(hit) {
		t.Error("HitTest still finds \"short\" after scrolling it out of view")
	}
}

func TestScrollBoxFadesAndScrollbar(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	fadesAt := func(dst *canvastest.Recorder) (left, right bool) {
		for _, r := range dst.Rects {
			if r.H != area.Box.Dy() {
				continue
			}
			left = left || r.X == area.Box.Min.X
			// A fade strip ends at the box's edge and is much narrower than it.
			right = right || r.X+r.W == area.Box.Max.X && r.W < area.Box.Dx()/2
		}
		return left, right
	}
	thumbDrawn := func(dst *canvastest.Recorder) bool {
		thumb := v.hscroll.thumb(area)
		for _, r := range dst.Rects {
			if image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H) == thumb {
				return true
			}
		}
		return false
	}
	draw := func() *canvastest.Recorder {
		dst := &canvastest.Recorder{Area: image.Rect(0, 0, 300, 400)}
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

	v.hover(area.Visible.Min.X+10, area.Visible.Min.Y+10)
	if !thumbDrawn(draw()) {
		t.Error("scrollbar not drawn while hovering the code block")
	}
	v.hover(area.Visible.Min.X+10, area.Visible.Min.Y-5)
	v.Layout(300, 400, 1, v.ctx.Time+hscrollRevealHold+hscrollRevealFade)
	if thumbDrawn(draw()) {
		t.Error("scrollbar still drawn once faded out after the pointer left the code block")
	}

	scrollHorizontal(v, area.Visible.Min.X+10, area.Visible.Min.Y+10, -50)
	if left, right := fadesAt(draw()); !left || !right {
		t.Errorf("mid-scroll: left fade %v, right fade %v, want both", left, right)
	}
}

// TestControllerDragsHorizontalScrollbar checks pressing a sideways
// scrollbar's thumb and moving drags it, and a press elsewhere doesn't.
func TestControllerDragsHorizontalScrollbar(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	c := NewController(v, image.Rect(0, 0, 300, 400))
	before := drawnTextX(t, v, "wide")
	thumb := v.hscroll.thumb(area)
	grab := image.Pt(thumb.Min.X+2, thumb.Min.Y+1)

	frame(c, press(grab.X, grab.Y+100))
	if v.hscroll.dragging != nil {
		t.Error("press away from the scrollbar started a drag")
	}
	frame(c, release(grab.X, grab.Y+100))

	frame(c, press(grab.X, grab.Y))
	if v.hscroll.dragging == nil {
		t.Fatal("press on the thumb didn't start a drag")
	}
	frame(c, input.PointerMove{X: grab.X + 40, Y: grab.Y + 30})
	if got := drawnTextX(t, v, "wide"); got >= before {
		t.Errorf("text at x=%d after dragging the thumb right, want less than %d (content scrolled left)", got, before)
	}
	frame(c, release(grab.X+40, grab.Y+30))
	if v.hscroll.dragging != nil {
		t.Error("still dragging after the release")
	}
}

// TestScrollbarStaysOnScreenForTallBlock checks that a code block taller
// than the viewport shows its scrollbar at the bottom of the viewport,
// not at its own off-screen bottom.
func TestScrollbarStaysOnScreenForTallBlock(t *testing.T) {
	source := "```\n" + strings.Repeat("wide ", 100) + "\n" + strings.Repeat("line\n", 100) + "```\n"
	v := NewView(Parse([]byte(source)), fonts.NewGoSelector(), stylingtest.NoViewMargin())
	v.Layout(300, 400, 1, 0)
	v.Draw(&canvastest.Recorder{Area: image.Rect(0, 0, 300, 400)}, 0, 0)
	area := v.hscroll.regions[0]
	if area.Box.Max.Y <= 400 {
		t.Fatalf("test setup: code block ends at y=%d, want below the 400px viewport", area.Box.Max.Y)
	}

	thumb := v.hscroll.thumb(area)
	if thumb.Max.Y > 400 || thumb.Min.Y < 0 {
		t.Errorf("thumb at %v, want within the 400px viewport", thumb)
	}
	c := NewController(v, image.Rect(0, 0, 300, 400))
	frame(c, press(thumb.Min.X+2, thumb.Min.Y+1))
	if v.hscroll.dragging == nil {
		t.Error("press on the on-screen thumb didn't start a drag")
	}
}

// TestTableScrollsWhenTooWide checks that a table whose cells can't wrap
// narrow enough - one holds an unbreakable word - scrolls sideways,
// while one that fits doesn't.
func TestTableScrollsWhenTooWide(t *testing.T) {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	table := func(cell string) engine.Block {
		return unwrap(Parse([]byte("| A | B |\n|---|---|\n| " + cell + " | x |\n")).root.Blocks[0])
	}
	if _, ok := table(strings.Repeat("unbreakable", 20)).GetBlockLayout(ctx, 300).(*engine.ScrollBox); !ok {
		t.Error("table with an unbreakable word wider than the page: not a *ScrollBox")
	}
	if _, ok := table("short").GetBlockLayout(ctx, 300).(*engine.ScrollBox); ok {
		t.Error("table that fits: got a *ScrollBox, want the plain table")
	}
}

// pageTop is how far down the page v is scrolled.
func pageTop(v *View) int {
	return v.VisibleViewBounds(image.Pt(v.width, 400)).Min.Y
}

// toucher drives a Controller with one finger.
type toucher struct {
	c   *Controller
	pos image.Point
	now time.Duration
}

func newToucher(v *View) *toucher {
	return &toucher{c: NewController(v, image.Rect(0, 0, 300, 400)), now: time.Second}
}

func (tc *toucher) start(p image.Point) {
	tc.pos = p
	tc.c.Frame([]input.Event{input.TouchStart{ID: 1, X: p.X, Y: p.Y}}, tc.now)
}

// drag moves the finger by (dx, dy), 16ms after the previous frame.
func (tc *toucher) drag(dx, dy int) {
	tc.pos = tc.pos.Add(image.Pt(dx, dy))
	tc.now += 16 * time.Millisecond
	tc.c.Frame([]input.Event{input.TouchMove{ID: 1, X: tc.pos.X, Y: tc.pos.Y}}, tc.now)
}

func (tc *toucher) end() {
	tc.c.Frame([]input.Event{input.TouchEnd{ID: 1}}, tc.now)
}

// coast runs a frame without input, 16ms later.
func (tc *toucher) coast() {
	tc.now += 16 * time.Millisecond
	tc.c.Frame(nil, tc.now)
}

func TestTouchPansCodeBlockSideways(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	tc := newToucher(v)
	start := area.Visible.Min.Add(image.Pt(10, 10))
	x, top := drawnTextX(t, v, "wide"), pageTop(v)

	tc.start(start)
	tc.drag(-4, 1)
	if got := drawnTextX(t, v, "wide"); got != x || pageTop(v) != top {
		t.Fatalf("moved before passing the lock distance: text x %d (want %d), page top %d (want %d)", got, x, pageTop(v), top)
	}
	tc.drag(-20, 2)
	if got := drawnTextX(t, v, "wide"); got != x-24 {
		t.Errorf("text x = %d after a mostly sideways drag, want %d (all movement so far applied)", got, x-24)
	}
	tc.drag(-10, -30)
	if got := drawnTextX(t, v, "wide"); got != x-34 || pageTop(v) != top {
		t.Errorf("once locked sideways: text x %d (want %d), page top %d (want %d, unchanged)", got, x-34, pageTop(v), top)
	}
}

func TestTouchOnCodeBlockMostlyVerticalScrollsPage(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	tc := newToucher(v)
	start := area.Visible.Min.Add(image.Pt(10, 10))
	x, top := drawnTextX(t, v, "wide"), pageTop(v)

	tc.start(start)
	tc.drag(3, -20)
	if got := pageTop(v); got != top+20 {
		t.Errorf("page top = %d after a mostly vertical drag, want %d", got, top+20)
	}
	if got := drawnTextX(t, v, "wide"); got != x {
		t.Errorf("code block scrolled sideways (text x %d, want %d) by a vertical drag", got, x)
	}
}

func TestTouchOffCodeBlockScrollsPageImmediately(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	tc := newToucher(v)
	top := pageTop(v)

	tc.start(image.Pt(10, area.Visible.Min.Y-5)) // just above the code block
	tc.drag(0, -3)
	if got := pageTop(v); got != top+3 {
		t.Errorf("page top = %d after a small drag off the code block, want %d (no lock delay)", got, top+3)
	}
}

func TestTouchSidewaysFlingCoasts(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	tc := newToucher(v)
	start := area.Visible.Min.Add(image.Pt(10, 10))

	tc.start(start)
	tc.drag(-20, 0)
	tc.drag(-20, 0)
	tc.end()
	released := drawnTextX(t, v, "wide")
	if !tc.c.Animating() {
		t.Fatal("Animating() = false right after a sideways fling, want true")
	}
	tc.coast()
	if got := drawnTextX(t, v, "wide"); got >= released {
		t.Errorf("text x = %d after a frame of coasting, want less than %d (still moving left)", got, released)
	}
}

func TestTouchPanRevealsScrollbarThenFades(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	tc := newToucher(v)
	start := area.Visible.Min.Add(image.Pt(10, 10))
	tc.start(start)
	tc.drag(-30, 0)
	tc.end()

	s := v.hscroll
	if got := s.barOpacity(area.Source, v.ctx.Time); got != 1 {
		t.Errorf("scrollbar opacity right after panning = %v, want 1", got)
	}
	mid := v.ctx.Time + hscrollRevealHold + hscrollRevealFade/2
	if got := s.barOpacity(area.Source, mid); got <= 0 || got >= 1 {
		t.Errorf("scrollbar opacity halfway through fading = %v, want between 0 and 1", got)
	}
	v.Layout(300, 400, 1, v.ctx.Time+hscrollRevealHold+hscrollRevealFade)
	if got := s.barOpacity(area.Source, v.ctx.Time); got != 0 {
		t.Errorf("scrollbar opacity after the fade = %v, want 0", got)
	}
	if s.animating(v.ctx.Time) {
		t.Error("scrollbar still animating after the fade ended")
	}
}

// TestTouchEndUnhoversPannedBlock checks that a touch pan doesn't leave
// its block hovered - which would keep its scrollbar at full opacity
// instead of fading - even though the Panel passed the touch through
// HoverAndClick while the finger was down.
func TestTouchEndUnhoversPannedBlock(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	tc := newToucher(v)
	start := area.Visible.Min.Add(image.Pt(10, 10))

	tc.start(start)
	tc.drag(-30, 0)
	tc.end()

	fadeMid := v.ctx.Time + hscrollRevealHold + hscrollRevealFade/2
	if got := v.hscroll.barOpacity(area.Source, fadeMid); got >= 1 {
		t.Errorf("scrollbar opacity partway through the fade = %v, want less than 1 (not stuck hovered)", got)
	}
}

// TestTouchVerticalScrollOnBlockShowsNoScrollbar checks that a touch
// starting on a sideways-scrolling block but scrolling the page doesn't
// show the block's scrollbar - even though the Panel passes the press
// through HoverAndClick (for a tap on a link).
func TestTouchVerticalScrollOnBlockShowsNoScrollbar(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	tc := newToucher(v)
	start := area.Visible.Min.Add(image.Pt(10, 10))

	tc.start(start)
	tc.drag(0, -30)
	if got := v.hscroll.barOpacity(area.Source, v.ctx.Time); got != 0 {
		t.Errorf("scrollbar opacity while scrolling the page = %v, want 0", got)
	}
}

// TestTouchEndStartsScrollbarFade checks that after a sideways pan, the
// scrollbar fades out from when the finger lifts, even if it rested
// longer than the fade takes before lifting.
func TestTouchEndStartsScrollbarFade(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	tc := newToucher(v)
	start := area.Visible.Min.Add(image.Pt(10, 10))

	tc.start(start)
	tc.drag(-30, 0)
	// The finger rests (no frames - Gio draws none without events), then
	// lifts after longer than the whole reveal.
	v.Layout(300, 400, 1, v.ctx.Time+2*(hscrollRevealHold+hscrollRevealFade))
	tc.end()

	if got := v.hscroll.barOpacity(area.Source, v.ctx.Time); got != 1 {
		t.Errorf("scrollbar opacity when the finger lifts = %v, want 1 (fading from here)", got)
	}
}

// TestHoverScrollbarFadesWhenIdle checks the desktop rules: moving the
// pointer over a block reveals its scrollbar, which fades once the
// pointer rests; moving again, or scrolling, brings it back; and it stays
// while the pointer is on the scrollbar itself.
func TestHoverScrollbarFadesWhenIdle(t *testing.T) {
	v, area := hscrollTestView(t, 300)
	s := v.hscroll
	over := area.Visible.Min.Add(image.Pt(10, 10))
	later := func() { v.Layout(300, 400, 1, v.ctx.Time+hscrollRevealHold+hscrollRevealFade) }
	opacity := func() float64 { return s.barOpacity(area.Source, v.ctx.Time) }

	v.hover(over.X, over.Y)
	if got := opacity(); got != 1 {
		t.Errorf("opacity after moving over the block = %v, want 1", got)
	}
	later()
	v.hover(over.X, over.Y) // same position: the pointer rests
	if got := opacity(); got != 0 {
		t.Errorf("opacity after resting on the block = %v, want 0 (faded)", got)
	}
	v.hover(over.X+1, over.Y)
	if got := opacity(); got != 1 {
		t.Errorf("opacity after moving again = %v, want 1", got)
	}
	later()
	scrollHorizontal(v, over.X+1, over.Y, -10)
	if got := opacity(); got != 1 {
		t.Errorf("opacity after scrolling the block = %v, want 1", got)
	}

	bar := s.thumb(area).Min.Add(image.Pt(1, 1))
	v.hover(bar.X, bar.Y)
	later()
	v.hover(bar.X, bar.Y)
	if got := opacity(); got != 1 {
		t.Errorf("opacity while resting on the scrollbar = %v, want 1", got)
	}
}

// unwrap strips any MarginBlocks around b, to reach the block itself.
func unwrap(b engine.Block) engine.Block {
	for {
		mb, ok := b.(*engine.MarginBlock)
		if !ok {
			return b
		}
		b = mb.Block
	}
}

// scrollHorizontal scrolls the sideways-scrolling block at (x, y), in
// the View's coordinates, by dx (positive moves its content right),
// reporting whether there was one.
func scrollHorizontal(v *View, x, y int, dx float64) bool {
	return v.hscroll.scrollAt(image.Pt(x, y), dx, v.ctx.Time) != nil
}
