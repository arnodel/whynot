package browser

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/input"
	"golang.org/x/image/font"
)

// recorder is a canvas.Canvas that records what's drawn on it, instead
// of drawing. A canvas from Clip records into the one it was clipped from.
type recorder struct {
	Area  image.Rectangle
	Texts []recordedText
	Rects []recordedRect
	root  *recorder
}

type recordedText struct {
	S    string
	X, Y int
}

type recordedRect struct {
	R     image.Rectangle
	Color color.Color
}

func (c *recorder) out() *recorder {
	if c.root != nil {
		return c.root
	}
	return c
}

func (c *recorder) Bounds() image.Rectangle { return c.Area }
func (c *recorder) DrawText(s string, face font.Face, x, y int, clr color.Color) {
	c.out().Texts = append(c.out().Texts, recordedText{s, x, y})
}
func (c *recorder) DrawImage(img image.Image, x, y, width, height int) {}
func (c *recorder) DrawRect(x, y, w, h int, clr color.Color) {
	r := image.Rect(x, y, x+w, y+h).Intersect(c.Area)
	c.out().Rects = append(c.out().Rects, recordedRect{r, clr})
}
func (c *recorder) Clip(r image.Rectangle) canvas.Canvas {
	return &recorder{Area: r.Intersect(c.Area), root: c.out()}
}

// separatorColor is the stacks' separator color in these tests, unlike
// anything a View draws.
var separatorColor = color.RGBA{R: 1, G: 2, B: 3, A: 255}

// stackOf returns a stack of documents with the given numbers of
// paragraphs, laid out in a 300x200 bounds.
func stackOf(paragraphs ...int) *ViewStack {
	var views []*whynot.View
	for i, n := range paragraphs {
		var b strings.Builder
		fmt.Fprintf(&b, "# Document %d\n\n", i)
		for j := range n {
			fmt.Fprintf(&b, "Paragraph %d of document %d, long enough to wrap onto a second line here.\n\n", j, i)
		}
		views = append(views, whynot.NewView(whynot.Parse([]byte(b.String()))))
	}
	s := NewViewStack(color.Black, separatorColor, views...)
	s.SetBounds(image.Rect(0, 0, 300, 200))
	return s
}

// fullHeight returns the height of v's whole content, without moving it.
func fullHeight(v *whynot.View) float64 {
	return float64(v.DocumentBounds(math.MaxInt32).Dy())
}

// absolute returns how far s is scrolled from its very start, from fully
// laid-out heights, independently of the stack's own walking.
func absolute(s *ViewStack) float64 {
	child, offset := s.Position()
	y := offset
	for i := range child {
		y += fullHeight(s.views[i])
	}
	return y
}

func drawStack(s *ViewStack) {
	s.Draw(&recorder{Area: s.bounds}, 0)
}

// TestViewStackScrollsExactly checks, over a random walk of small and
// large steps both ways, that every step moves the stack by exactly the
// step, across children, and stops at the very start, and with its end at
// the bottom.
func TestViewStackScrollsExactly(t *testing.T) {
	s := stackOf(3, 25, 1, 0, 12)
	total := 0.0
	for _, v := range s.views {
		total += fullHeight(v)
	}
	limit := total - float64(s.bounds.Dy())
	r := rand.New(rand.NewPCG(1, 2))
	want := 0.0
	for step := range 500 {
		dy := float64(r.IntN(400) - 200)
		if step%25 == 0 {
			dy *= 10
		}
		s.ScrollBy(dy)
		want = math.Max(0, math.Min(limit, want+dy))
		if got := absolute(s); math.Abs(got-want) > 1e-6 {
			t.Fatalf("step %d, by %v: scrolled to %v, want %v", step, dy, got, want)
		}
	}
}

// TestViewStackDraws checks that the children's regions tile the stack
// from its top to its bottom.
func TestViewStackDraws(t *testing.T) {
	s := stackOf(3, 25, 1, 12)
	for _, dy := range []float64{0, 130, 300, 1500, -700, 99999} {
		s.ScrollBy(dy)
		drawStack(s)
		y := 0
		for i, r := range s.regions {
			if r.Empty() {
				continue
			}
			if r.Min.Y != y {
				t.Errorf("after %v: child %d is drawn from %d, want %d, where the one above ends", dy, i, r.Min.Y, y)
			}
			y = r.Max.Y
		}
		if y != 200 {
			t.Errorf("after %v: the children cover down to %d, want the stack's bottom, 200", dy, y)
		}
	}
}

// TestViewStackReveal checks that Reveal shows a short child with the least
// scrolling, puts the start of a tall one starting partway down at the
// top, and brings back one scrolled past.
func TestViewStackReveal(t *testing.T) {
	s := stackOf(2, 0, 40, 1) // child 1, a heading alone, is shorter than the stack
	s.Reveal(1)
	drawStack(s)
	if r := s.regions[1]; float64(r.Dy()) < fullHeight(s.views[1])-1 {
		t.Errorf("a short child: drawn in %v, want all of it", r)
	}
	s.Reveal(2)
	if child, offset := s.Position(); child != 2 || offset != 0 {
		t.Errorf("a tall child: at child %d, %v in, want child 2's start", child, offset)
	}
	s.ScrollToEnd()
	s.Reveal(0)
	if child, offset := s.Position(); child != 0 || offset != 0 {
		t.Errorf("a child scrolled past: at child %d, %v in, want child 0's start", child, offset)
	}
}

// TestViewStackGrowthAboveKeepsPlace checks that a child above the one
// being read can grow without moving what's shown.
func TestViewStackGrowthAboveKeepsPlace(t *testing.T) {
	doc, w := whynot.NewParser().Stream()
	io.WriteString(w, "# A reply still coming\n\nFirst line.\n\n")
	second := whynot.NewView(whynot.Parse([]byte(strings.Repeat("A paragraph of the page being read.\n\n", 30))))
	s := NewViewStack(color.Black, separatorColor, whynot.NewView(doc), second)
	s.SetBounds(image.Rect(0, 0, 300, 200))
	draw := func() *recorder {
		r := &recorder{Area: image.Rect(0, 0, 300, 200)}
		s.Draw(r, 0)
		return r
	}
	draw()
	s.Reveal(1)
	if c, o := s.Position(); c != 1 || o != 0 {
		t.Fatalf("Reveal of a tall child starting partway down: at child %d, %v in, want child 1's start", c, o)
	}
	s.ScrollBy(120)
	before := draw()
	child, offset := s.Position()
	for i := range 10 {
		fmt.Fprintf(w, "More of the reply, line %d.\n\n", i)
	}
	after := draw()
	if c, o := s.Position(); c != child || o != offset {
		t.Errorf("the first child grew, and the position moved from child %d, %v to child %d, %v", child, offset, c, o)
	}
	if fmt.Sprint(after.Texts) != fmt.Sprint(before.Texts) {
		t.Error("the first child grew above, and what's shown changed")
	}
	w.Close()
}

// TestViewStackResize checks that a resize keeps the stack in the same
// child, at about the same proportion through it, and that it still scrolls
// exactly.
func TestViewStackResize(t *testing.T) {
	s := stackOf(3, 25, 1, 12)
	s.ScrollBy(2500)
	child, offset := s.Position()
	ratio := offset / fullHeight(s.views[child])
	for _, width := range []int{180, 520, 300} {
		s.SetBounds(image.Rect(0, 0, width, 200))
		drawStack(s)
		c, o := s.Position()
		if c != child {
			t.Fatalf("at width %d: the stack moved from child %d to child %d", width, child, c)
		}
		if got := o / fullHeight(s.views[c]); math.Abs(got-ratio) > 0.05 {
			t.Errorf("at width %d: %.2f of the way through the child, want about %.2f", width, got, ratio)
		}
		before := absolute(s)
		s.ScrollBy(333)
		if got := absolute(s); math.Abs(got-before-333) > 1e-6 {
			t.Errorf("at width %d: scrolling by 333 moved by %v", width, got-before)
		}
		s.ScrollBy(-333)
	}
}

// TestStackControllerScrolls checks that the wheel scrolls the stack, by
// its vertical part, in logical pixels.
func TestStackControllerScrolls(t *testing.T) {
	s := stackOf(3, 25, 12)
	c := NewStackController(s)
	drawStack(s)
	before := absolute(s)
	c.Frame([]input.Event{input.Wheel{X: 100, Y: 100, DY: 75}}, 0)
	if got := absolute(s) - before; got != 75 {
		t.Errorf("a wheel of 75 scrolled the stack by %v, want 75", got)
	}
	c.Frame([]input.Event{input.Wheel{X: 1000, Y: 100, DY: 75}}, 0)
	if got := absolute(s) - before; got != 75 {
		t.Errorf("a wheel outside the stack scrolled it, to %v", got)
	}
}

// TestStackControllerClicks checks that a click on a link in a child below
// the top is reported, with that child.
func TestStackControllerClicks(t *testing.T) {
	views := []*whynot.View{
		whynot.NewView(whynot.Parse([]byte(strings.Repeat("Some text.\n\n", 4)))),
		whynot.NewView(whynot.Parse([]byte("[A link](https://example.com)\n"))),
	}
	s := NewViewStack(color.Black, separatorColor, views...)
	s.SetBounds(image.Rect(0, 0, 300, 400))
	c := NewStackController(s)
	drawStack(s)
	second := s.regions[1]
	for y := second.Min.Y; y < second.Max.Y; y++ {
		for x := second.Min.X; x < second.Min.X+120; x++ {
			if _, ok := views[1].LinkAt(x, y); !ok {
				continue
			}
			events := c.Frame([]input.Event{
				input.PointerMove{X: x, Y: y},
				input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary, Down: true},
				input.PointerButton{X: x, Y: y, Button: input.ButtonPrimary, Down: false},
			}, 0)
			if len(events) != 1 || events[0].Child != 1 {
				t.Fatalf("clicking the link reported %+v, want a link click in child 1", events)
			}
			if link, ok := events[0].Event.(whynot.LinkClick); !ok || link.Destination != "https://example.com" {
				t.Errorf("reported %+v, want the link", events[0].Event)
			}
			return
		}
	}
	t.Fatal("no link found in the second child")
}

// TestViewStackSeparators checks a line is drawn across the stack's full
// width where each child drawn meets the next, and nowhere else.
func TestViewStackSeparators(t *testing.T) {
	s := stackOf(1, 0, 1, 30)
	s.SetBounds(image.Rect(0, 0, 300, 800))
	r := &recorder{Area: s.bounds}
	s.Draw(r, 0)
	var lines []int
	for _, rect := range r.Rects {
		if rect.Color != separatorColor {
			continue
		}
		if rect.R.Min.X != 0 || rect.R.Max.X != 300 || rect.R.Dy() != 1 {
			t.Errorf("a separator is drawn in %v, want a line 1 pixel high across the stack's width, 300", rect.R)
		}
		lines = append(lines, rect.R.Min.Y)
	}
	var want []int
	for i := 1; i < len(s.regions); i++ {
		if !s.regions[i-1].Empty() && !s.regions[i].Empty() {
			want = append(want, s.regions[i].Min.Y)
		}
	}
	if len(want) < 2 || fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Errorf("separators at %v, want them at the tops of the children below the first, %v", lines, want)
	}
}
