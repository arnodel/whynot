package browser

import (
	"image"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arnodel/whynot"
)

const (
	testWidth  = 300
	testHeight = 200
)

// fakePanel is a minimal whynot.Panel, mirroring the real ones'
// SetBounds/SetScale/SetView-triggers-relayout behavior closely enough
// for App's own tests, without depending on either rendering backend.
type fakePanel struct {
	view   *whynot.View
	bounds image.Rectangle
	scale  float64

	// lastStyleSheet records SetStyleSheet's most recent argument, so
	// tests can check App.SetTheme actually reached the panel.
	lastStyleSheet whynot.StyleSheet
}

var _ whynot.Panel = (*fakePanel)(nil)

func (p *fakePanel) relayout() {
	if p.view != nil {
		p.view.Layout(p.bounds.Dx(), p.bounds.Dy(), p.scale, 0)
	}
}

func (p *fakePanel) View() *whynot.View          { return p.view }
func (p *fakePanel) SetView(v *whynot.View)      { p.view = v; p.relayout() }
func (p *fakePanel) Bounds() image.Rectangle     { return p.bounds }
func (p *fakePanel) SetBounds(r image.Rectangle) { p.bounds = r; p.relayout() }
func (p *fakePanel) Scale() float64              { return p.scale }
func (p *fakePanel) SetScale(s float64)          { p.scale = s; p.relayout() }
func (p *fakePanel) SetStyleSheet(s whynot.StyleSheet) {
	p.lastStyleSheet = s
	p.view.SetStyleSheet(s)
}
func (p *fakePanel) ScrollDown() { p.view.Scroll(-40 * p.scale) }
func (p *fakePanel) ScrollUp()   { p.view.Scroll(40 * p.scale) }
func (p *fakePanel) PageDown()   { p.view.Scroll(-float64(p.bounds.Dy()) * 0.9) }
func (p *fakePanel) PageUp()     { p.view.Scroll(float64(p.bounds.Dy()) * 0.9) }

// writeTempMD writes content to dir/name and returns its file: URL, via
// the same absFileURL this package uses for a real command-line path -
// so App.Follow/Reload exercise the exact same LoadDocument("file", ...)
// path a real local document would.
func writeTempMD(t *testing.T, dir, name, content string) *url.URL {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	u, err := absFileURL(path)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// newTestApp constructs an App with a fakePanel already showing source,
// laid out and current (as if Open had just been called at startup) -
// deviceScale 1, no toolbar, so width/height match testWidth/testHeight
// exactly.
func newTestApp(t *testing.T, dir string, location *url.URL, source string) *App {
	t.Helper()
	app := NewApp(whynot.NewGoFontFaceSelector(72), whynot.NewDarkStyleSheet(), true)
	view := app.NewView([]byte(source), location)
	app.Panel = &fakePanel{}
	app.Relayout(testWidth, testHeight, 1, 0)
	app.Panel.SetView(view)
	app.Open(location)
	return app
}

func TestSamePage(t *testing.T) {
	a, _ := url.Parse("file:///doc.md#frag1")
	b, _ := url.Parse("file:///doc.md#frag2")
	c, _ := url.Parse("file:///other.md")
	if !samePage(a, b) {
		t.Error("samePage with differing fragments only = false, want true")
	}
	if samePage(a, c) {
		t.Error("samePage with different paths = true, want false")
	}
}

func TestResolveAgainst(t *testing.T) {
	base, _ := url.Parse("file:///a/b/doc.md")
	got, err := resolveAgainst(base, "other.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := "file:///a/b/other.md"; got.String() != want {
		t.Errorf("resolveAgainst(%q) = %q, want %q", "other.md", got.String(), want)
	}
}

func TestResolveLocationArg(t *testing.T) {
	if u, err := ResolveLocationArg("Welcome"); err != nil || u != WelcomeURL {
		t.Errorf("ResolveLocationArg(%q) = %v, %v, want WelcomeURL, nil", "Welcome", u, err)
	}
	if u, err := ResolveLocationArg("https://example.com/x.md"); err != nil || u.String() != "https://example.com/x.md" {
		t.Errorf("ResolveLocationArg(https URL) = %v, %v, want it unchanged, nil", u, err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	u, err := ResolveLocationArg(path)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "file" {
		t.Errorf("ResolveLocationArg(existing file path).Scheme = %q, want \"file\"", u.Scheme)
	}

	if _, err := ResolveLocationArg(filepath.Join(dir, "does-not-exist.md")); err == nil {
		t.Error("ResolveLocationArg(nonexistent path) = nil error, want an error")
	}

	// A file: URL (e.g. from editing an address bar pre-filled with
	// Location().String()) must round-trip, not just a bare path.
	fileURL, err := absFileURL(path)
	if err != nil {
		t.Fatal(err)
	}
	if u, err := ResolveLocationArg(fileURL.String()); err != nil || u.Path != fileURL.Path {
		t.Errorf("ResolveLocationArg(%q) = %v, %v, want Path %q, nil", fileURL, u, err, fileURL.Path)
	}
	if _, err := ResolveLocationArg("file:///no/such/file.md"); err == nil {
		t.Error("ResolveLocationArg(a file: URL to a nonexistent path) = nil error, want an error")
	}

	// A bare domain (no scheme) is guessed as https://.
	if u, err := ResolveLocationArg("example.com/x.md"); err != nil || u.String() != "https://example.com/x.md" {
		t.Errorf("ResolveLocationArg(%q) = %v, %v, want https://example.com/x.md, nil", "example.com/x.md", u, err)
	}

	// A single-letter "scheme" (a Windows drive letter, not a real
	// protocol) must not be mistaken for one, or a Windows absolute
	// path could never resolve as a file - checked by the error
	// message: on this (non-Windows) test machine the path can't
	// actually exist, so some error is expected either way, but it must
	// be the not-a-valid-location one, not "unsupported link scheme
	// \"c\"" (which is what treating "C:" as a real scheme would give).
	_, err = ResolveLocationArg(`C:\nonexistent\path.md`)
	if err == nil {
		t.Error(`ResolveLocationArg("C:\nonexistent\path.md") = nil error, want one`)
	} else if strings.Contains(err.Error(), "unsupported") {
		t.Errorf(`ResolveLocationArg("C:\nonexistent\path.md") = %v, want a plain not-found error, not "unsupported" - "C:" must not be treated as a real scheme`, err)
	}

	if _, err := ResolveLocationArg("not a real location"); err == nil {
		t.Error("ResolveLocationArg(garbage) = nil error, want an error")
	}
}

func TestAppFollowSamePageFragmentScrolls(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "doc.md", "[jump](#target)\n\n"+
		"paragraph\n\n"+repeatLines(40)+"\n\n# Target")
	app := newTestApp(t, dir, loc, "[jump](#target)\n\n"+
		"paragraph\n\n"+repeatLines(40)+"\n\n# Target")

	before := app.Panel.View().VisibleViewBounds(image.Pt(testWidth, testHeight)).Min.Y
	app.Follow("#target")
	after := app.Panel.View().VisibleViewBounds(image.Pt(testWidth, testHeight)).Min.Y

	if after <= before {
		t.Errorf("VisibleViewBounds top after following an in-page anchor = %d, want more than %d", after, before)
	}
	if !app.CanGoBack() {
		t.Error("CanGoBack() = false after an in-page anchor jump, want true")
	}
	if app.Location().Fragment != "target" {
		t.Errorf("Location().Fragment = %q, want \"target\"", app.Location().Fragment)
	}
}

func repeatLines(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "Some text, quite a bit of it actually, more than one line's worth.\n\n"
	}
	return s
}

func TestAppFollowCrossDocumentLoadsAndPushesHistory(t *testing.T) {
	dir := t.TempDir()
	locA := writeTempMD(t, dir, "a.md", "# Doc A\n\n[to b](b.md)")
	writeTempMD(t, dir, "b.md", "# Doc B")
	app := newTestApp(t, dir, locA, "# Doc A\n\n[to b](b.md)")

	if app.CanGoBack() {
		t.Fatal("CanGoBack() = true before any navigation, want false")
	}

	app.Follow("b.md")

	if got := app.Location().Path; filepath.Base(got) != "b.md" {
		t.Errorf("Location() after following b.md = %q, want it to end in b.md", got)
	}
	if !app.CanGoBack() {
		t.Error("CanGoBack() = false after a cross-document Follow, want true")
	}
	if title, _ := app.Panel.View().Title(); title != "Doc B" {
		t.Errorf("current View's Title() = %q, want \"Doc B\"", title)
	}
}

func TestAppNavigateLoadsDocumentAndPushesHistory(t *testing.T) {
	dir := t.TempDir()
	locA := writeTempMD(t, dir, "a.md", "# Doc A")
	writeTempMD(t, dir, "b.md", "# Doc B")
	app := newTestApp(t, dir, locA, "# Doc A")

	// Navigate takes typed/pasted text (a bare path, URL, or "welcome" -
	// see ResolveLocationArg), not an already-resolved location URL.
	pathB := filepath.Join(dir, "b.md")
	if err := app.Navigate(pathB); err != nil {
		t.Fatalf("Navigate(%q) = %v, want nil", pathB, err)
	}

	if title, _ := app.Panel.View().Title(); title != "Doc B" {
		t.Errorf("Title() after Navigate = %q, want \"Doc B\"", title)
	}
	if !app.CanGoBack() {
		t.Error("CanGoBack() = false after Navigate, want true - it should push history like Follow/Paste")
	}
}

func TestAppNavigateInvalidLeavesCurrentDocumentAlone(t *testing.T) {
	dir := t.TempDir()
	locA := writeTempMD(t, dir, "a.md", "# Doc A")
	app := newTestApp(t, dir, locA, "# Doc A")

	err := app.Navigate(filepath.Join(dir, "does-not-exist.md"))
	if err == nil {
		t.Fatal("Navigate to a nonexistent path returned nil error, want one")
	}
	if title, _ := app.Panel.View().Title(); title != "Doc A" {
		t.Errorf("Title() after a failed Navigate = %q, want unchanged \"Doc A\"", title)
	}
	if app.CanGoBack() {
		t.Error("CanGoBack() = true after a failed Navigate, want false - nothing should be pushed")
	}
}

func TestAppBackForward(t *testing.T) {
	dir := t.TempDir()
	locA := writeTempMD(t, dir, "a.md", "# Doc A\n\n[to b](b.md)")
	writeTempMD(t, dir, "b.md", "# Doc B")
	app := newTestApp(t, dir, locA, "# Doc A\n\n[to b](b.md)")

	app.Follow("b.md")
	if title, _ := app.Panel.View().Title(); title != "Doc B" {
		t.Fatalf("Title() after Follow = %q, want \"Doc B\"", title)
	}

	app.Back()
	if title, _ := app.Panel.View().Title(); title != "Doc A" {
		t.Errorf("Title() after Back = %q, want \"Doc A\"", title)
	}
	if app.CanGoBack() {
		t.Error("CanGoBack() = true after Back to the start of history, want false")
	}
	if !app.CanGoForward() {
		t.Error("CanGoForward() = false after Back, want true")
	}

	app.Forward()
	if title, _ := app.Panel.View().Title(); title != "Doc B" {
		t.Errorf("Title() after Forward = %q, want \"Doc B\"", title)
	}
	if app.CanGoForward() {
		t.Error("CanGoForward() = true after Forward to the end of history, want false")
	}

	// Back with a genuinely empty history (a fresh App) is a no-op.
	fresh := newTestApp(t, dir, locA, "# Doc A")
	fresh.Back()
	if title, _ := fresh.Panel.View().Title(); title != "Doc A" {
		t.Error("Back() with empty history changed the current document, want a no-op")
	}
}

func TestAppNewNavigationClearsForward(t *testing.T) {
	dir := t.TempDir()
	locA := writeTempMD(t, dir, "a.md", "# Doc A\n\n[to b](b.md)\n\n[to c](c.md)")
	writeTempMD(t, dir, "b.md", "# Doc B")
	writeTempMD(t, dir, "c.md", "# Doc C")
	app := newTestApp(t, dir, locA, "# Doc A\n\n[to b](b.md)\n\n[to c](c.md)")

	app.Follow("b.md")
	app.Back()
	if !app.CanGoForward() {
		t.Fatal("CanGoForward() = false after Back, want true")
	}

	app.Follow("c.md")
	if app.CanGoForward() {
		t.Error("CanGoForward() = true after a new Follow, want cleared (like a real browser discarding forward history)")
	}
}

func TestAppReloadDoesNotPushHistory(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# Version 1")
	app := newTestApp(t, dir, loc, "# Version 1")

	// Rewrite the file, then Reload should pick up the new content in
	// place, without adding a Back-able history entry.
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Version 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.Reload()

	if title, _ := app.Panel.View().Title(); title != "Version 2" {
		t.Errorf("Title() after Reload = %q, want \"Version 2\"", title)
	}
	if app.CanGoBack() {
		t.Error("CanGoBack() = true after Reload, want false (Reload doesn't push history)")
	}
}

func TestAppSetThemeSyncsDarkThemeAndPanel(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# Doc")
	app := newTestApp(t, dir, loc, "# Doc")

	if !app.DarkTheme() {
		t.Fatal("DarkTheme() = false right after construction with dark=true, want true")
	}

	// NewDarkStyleSheet/NewLightStyleSheet both return the same concrete
	// type (see App.darkTheme's own doc comment), so there's no type to
	// assert on here - check identity with App's own StyleSheet()
	// instead, confirming SetTheme actually reached the panel rather
	// than just updating App's own bookkeeping.
	app.SetTheme(false)
	if app.DarkTheme() {
		t.Error("DarkTheme() = true after SetTheme(false), want false")
	}
	if app.Panel.(*fakePanel).lastStyleSheet != app.StyleSheet() {
		t.Error("panel's StyleSheet after SetTheme(false) != App.StyleSheet(), want them in sync")
	}
	lightSheet := app.StyleSheet()

	app.SetTheme(true)
	if !app.DarkTheme() {
		t.Error("DarkTheme() = false after SetTheme(true), want true")
	}
	if app.Panel.(*fakePanel).lastStyleSheet != app.StyleSheet() {
		t.Error("panel's StyleSheet after SetTheme(true) != App.StyleSheet(), want them in sync")
	}
	if app.StyleSheet() == lightSheet {
		t.Error("StyleSheet() after switching themes twice = the same instance as the light one, want a distinct StyleSheet")
	}
}

func TestAppSetZoomClamps(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# Doc")
	app := newTestApp(t, dir, loc, "# Doc")

	app.SetZoom(10)
	if app.Zoom() != maxZoom {
		t.Errorf("Zoom() after SetZoom(10) = %v, want clamped to maxZoom %v", app.Zoom(), maxZoom)
	}
	app.SetZoom(0)
	if app.Zoom() != minZoom {
		t.Errorf("Zoom() after SetZoom(0) = %v, want clamped to minZoom %v", app.Zoom(), minZoom)
	}

	app.SetZoom(1.5)
	if app.Zoom() != 1.5 {
		t.Errorf("Zoom() after SetZoom(1.5) = %v, want 1.5", app.Zoom())
	}
	if got := app.Panel.Scale(); got != app.deviceScale*1.5 {
		t.Errorf("Panel.Scale() after SetZoom(1.5) = %v, want deviceScale*1.5 = %v", got, app.deviceScale*1.5)
	}
}

func TestAppRelayout(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# Doc")
	app := newTestApp(t, dir, loc, "# Doc")

	app.Relayout(400, 300, 2, 20)

	wantBounds := image.Rect(0, 20, 800, 600)
	if got := app.Panel.Bounds(); got != wantBounds {
		t.Errorf("Panel.Bounds() after Relayout(400,300,2,20) = %v, want %v", got, wantBounds)
	}
	if got, want := app.Panel.Scale(), 2*app.Zoom(); got != want {
		t.Errorf("Panel.Scale() after Relayout = %v, want deviceScale*zoom = %v", got, want)
	}
}

func TestAppOpenFiresOnTitleChange(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# My Title")

	app := NewApp(whynot.NewGoFontFaceSelector(72), whynot.NewDarkStyleSheet(), true)
	view := app.NewView([]byte("# My Title"), loc)
	app.Panel = &fakePanel{}
	app.Relayout(testWidth, testHeight, 1, 0)
	app.Panel.SetView(view)

	var gotTitle string
	var calls int
	app.OnTitleChange = func(title string) { gotTitle = title; calls++ }

	app.Open(loc)

	if calls != 1 {
		t.Fatalf("OnTitleChange called %d time(s), want 1", calls)
	}
	if gotTitle != "My Title" {
		t.Errorf("OnTitleChange title = %q, want \"My Title\"", gotTitle)
	}
}

func TestAppOpenFallsBackToUntitled(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "no heading here")

	app := NewApp(whynot.NewGoFontFaceSelector(72), whynot.NewDarkStyleSheet(), true)
	view := app.NewView([]byte("no heading here"), loc)
	app.Panel = &fakePanel{}
	app.Relayout(testWidth, testHeight, 1, 0)
	app.Panel.SetView(view)

	var gotTitle string
	app.OnTitleChange = func(title string) { gotTitle = title }
	app.Open(loc)

	if gotTitle != "Untitled document" {
		t.Errorf("OnTitleChange title for a heading-less document = %q, want \"Untitled document\"", gotTitle)
	}
}
