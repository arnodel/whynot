package browser

import (
	"image"
	"image/color"
	"log"
	"net/url"
	"slices"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// journal is journal mode's state: the stack showing the trail of pages,
// and what each of its children is.
type journal struct {
	stack      *ViewStack
	controller *StackController
	pages      []trailPage
}

// trailPage is a page of the trail: where it was loaded from, and the
// first history entry for it, or -1 for the current page.
type trailPage struct {
	location *url.URL
	first    int
}

// commandStep and pageOverlap are how far ScrollUp and ScrollDown move,
// in logical pixels, and the fraction of a page PageUp and PageDown keep
// in view, as a Panel's do.
const (
	commandStep = 40
	pageOverlap = 0.1
)

// Journal reports whether journal mode is on (see ToggleJournal).
func (a *App) Journal() bool { return a.journal != nil }

// ToggleJournal turns journal mode on or off. In journal mode, the pages
// history went through are shown above the current one, oldest first, as
// one scrollable column: each page in its own View, the same one it has
// alone, so turning it on or off loses nothing. Turning it on keeps the
// current page where it was, with the trail above it.
//
// A link followed from an earlier page goes on from that page: the pages
// that came after it are dropped from history. A no-op while the TOC is
// showing.
func (a *App) ToggleJournal() {
	if a.tocDocView != nil {
		return
	}
	a.arrival = arrival{}
	if a.journal != nil {
		a.journal = nil
		a.Panel.SetBounds(a.bounds)
		a.Panel.SetScale(a.deviceScale)
		a.Panel.SetZoom(a.zoom)
		a.Panel.SetView(a.Panel.View()) // its scrollbar back
		return
	}
	a.journal = &journal{}
	a.restack()
}

// restack rebuilds the journal's stack from the trail, keeping the page
// at its top there if it's still in the trail, else putting the current
// page there, each at its own scroll position. A no-op outside journal
// mode.
func (a *App) restack() {
	j := a.journal
	if j == nil {
		return
	}
	var top *whynot.View
	if j.stack != nil {
		top = j.stack.views[j.stack.top]
	}
	var views []*whynot.View
	j.pages = nil
	add := func(v *whynot.View, location *url.URL, first int) {
		if n := len(views); n > 0 && views[n-1] == v {
			// An in-page jump: the same page, now at location.
			j.pages[n-1].location = location
			return
		}
		views = append(views, v)
		j.pages = append(j.pages, trailPage{location: location, first: first})
	}
	for i, e := range a.history {
		add(e.view, e.location, i)
	}
	add(a.Panel.View(), a.location, -1)
	j.pages[len(j.pages)-1].first = -1

	background, separator := a.stackColors()
	for _, v := range views {
		a.prepare(v)
	}
	j.stack = NewViewStack(background, separator, views...)
	j.stack.top = len(views) - 1
	if i := slices.Index(views, top); i >= 0 {
		j.stack.top = i
	}
	j.stack.SetBounds(a.bounds)
	j.controller = NewStackController(j.stack)
}

// prepare gives v, a page of the journal, the current scale, zoom and
// StyleSheet, without its own scrollbar.
func (a *App) prepare(v *whynot.View) {
	v.SetStyleSheet(a.styleSheet)
	v.SetScale(a.deviceScale)
	v.SetZoom(a.zoom)
	v.SetScrollbar(false)
}

// stackColors returns the colors of the current theme the journal's
// stack draws with itself.
func (a *App) stackColors() (background, separator color.Color) {
	theme := simpletheme.Light()
	if a.darkTheme {
		theme = simpletheme.Dark()
	}
	return theme.BackgroundColor, theme.ThematicBreakColor
}

// Frame applies a frame's input, at now, to what's shown: the current
// page, or in journal mode the stack, and follows the links clicked (see
// HandleEvents).
func (a *App) Frame(events []input.Event, now time.Duration) {
	j := a.journal
	if j == nil {
		a.HandleEvents(a.Panel.Frame(events, now))
		return
	}
	for _, e := range j.controller.Frame(events, now) {
		if e.Child < 0 || e.Child == len(j.pages)-1 {
			a.HandleEvents([]whynot.Event{e.Event})
			continue
		}
		switch ev := e.Event.(type) {
		case whynot.LinkClick:
			a.followFrom(e.Child, ev.Destination)
		case whynot.AnchorClick:
			a.arrival = arrival{}
			j.stack.scrollToFragment(e.Child, ev.ID)
		}
		if a.journal != j {
			return // the trail changed: the rest is stale
		}
	}
}

// followFrom follows dest, a link in page i of the trail, an earlier one
// than the current page: history goes back to that page, and on to the
// new one. A link to that page itself scrolls the stack there.
func (a *App) followFrom(i int, dest string) {
	page := a.journal.pages[i]
	resolved, err := resolveAgainst(page.location, dest)
	if err != nil {
		log.Printf("link destination %q: %v", dest, err)
		return
	}
	if samePage(page.location, resolved) {
		a.arrival = arrival{}
		a.journal.stack.scrollToFragment(i, resolved.Fragment)
		return
	}
	doc, err := a.Load(resolved)
	if err != nil {
		if !openIfWebPage(err, resolved) {
			log.Printf("loading %s: %v", resolved, err)
		}
		return
	}
	// Back at page i, as if the pages after it hadn't been visited.
	view := a.journal.stack.views[i]
	a.history, a.future = a.history[:page.first], nil
	a.Panel.SetView(view)
	a.location = page.location
	a.show(doc, resolved)
}

// Draw draws what's shown, at now: the current page, or in journal mode
// the stack.
func (a *App) Draw(dst canvas.Canvas, now time.Duration) {
	if j := a.journal; j != nil {
		j.stack.Draw(dst, now)
		return
	}
	a.Panel.Draw(dst, now)
}

// Animating reports whether frames must keep coming even without input,
// as for a fling or a scrollbar fading out.
func (a *App) Animating() bool {
	if j := a.journal; j != nil {
		return j.controller.Animating()
	}
	return a.Panel.Animating()
}

// ViewAt returns the View shown at (x, y): the current page's, or in
// journal mode, the page's there, if any.
func (a *App) ViewAt(x, y int) (*whynot.View, bool) {
	j := a.journal
	if j == nil {
		return a.Panel.View(), image.Pt(x, y).In(a.Panel.Bounds())
	}
	if i := j.controller.childAt(image.Pt(x, y)); i >= 0 {
		return j.stack.views[i], true
	}
	return nil, false
}

// ScrollUp, ScrollDown, PageUp and PageDown scroll what's shown as a
// Panel's methods do, as the reader asked: the end of a document still
// arriving stops being followed.
func (a *App) ScrollUp()   { a.scrollBy(-commandStep*a.deviceScale, a.Panel.ScrollUp) }
func (a *App) ScrollDown() { a.scrollBy(commandStep*a.deviceScale, a.Panel.ScrollDown) }
func (a *App) PageUp()     { a.scrollBy(-a.page(), a.Panel.PageUp) }
func (a *App) PageDown()   { a.scrollBy(a.page(), a.Panel.PageDown) }

// page is how far PageUp and PageDown move.
func (a *App) page() float64 {
	return float64(a.bounds.Dy()) * (1 - pageOverlap)
}

// scrollBy scrolls the journal's stack by dy, or calls alone outside
// journal mode.
func (a *App) scrollBy(dy float64, alone func()) {
	a.arrival = arrival{}
	if j := a.journal; j != nil {
		j.stack.ScrollBy(dy)
		return
	}
	alone()
}

// ScrollLeft and ScrollRight scroll a block wider than the page, such as
// a code block, as a Panel's do.
func (a *App) ScrollLeft() {
	if j := a.journal; j != nil {
		j.controller.ScrollLeft()
		return
	}
	a.Panel.ScrollLeft()
}

func (a *App) ScrollRight() {
	if j := a.journal; j != nil {
		j.controller.ScrollRight()
		return
	}
	a.Panel.ScrollRight()
}
