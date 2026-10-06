//go:build !js

package browser

import (
	"context"
	"image"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/styles/simpletheme"
)

const (
	testWidth  = 300
	testHeight = 200
)

// usesStyleSheet reports whether v looks styled by s, comparing the
// colors a View reports with those of a fresh View using s, since a
// StyleSheet is opaque.
func usesStyleSheet(v *whynot.View, s whynot.StyleSheet) bool {
	ref := whynot.NewView(whynot.Parse(nil), whynot.WithStyleSheet(s))
	return v.HighlightColor() == ref.HighlightColor() &&
		v.ScrollbarColor(false, false) == ref.ScrollbarColor(false, false)
}

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

// newTestApp constructs an App with a Panel already showing source,
// laid out and current (as if Open had just been called at startup) -
// deviceScale 1, no toolbar, so width/height match testWidth/testHeight
// exactly.
func newTestApp(t *testing.T, dir string, location *url.URL, source string) *App {
	t.Helper()
	app := NewApp(fonts.NewGoSelector(), simpletheme.DarkStyleSheet, true, NewRegistry(nil))
	view := app.NewView([]byte(source), location)
	app.Panel = whynot.NewPanel(view, image.Rectangle{})
	app.Relayout(testWidth, testHeight, 1, 0)
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

	before, _ := app.Panel.View().VisibleRange()
	app.Follow("#target")
	after, _ := app.Panel.View().VisibleRange()

	if after <= before {
		t.Errorf("VisibleRange start after following an in-page anchor = %v, want more than %v", after, before)
	}
	if !app.CanGoBack() {
		t.Error("CanGoBack() = false after an in-page anchor jump, want true")
	}
	if app.Location().Fragment != "target" {
		t.Errorf("Location().Fragment = %q, want \"target\"", app.Location().Fragment)
	}
}

// TestAppHandleEvents checks HandleEvents follows a LinkClick to another
// document, and an AnchorClick within the current one, with history.
func TestAppHandleEvents(t *testing.T) {
	dir := t.TempDir()
	source := "[other](other.md)\n\n" + repeatLines(40) + "\n\n# Target"
	loc := writeTempMD(t, dir, "doc.md", source)
	writeTempMD(t, dir, "other.md", "# Other")
	app := newTestApp(t, dir, loc, source)

	app.HandleEvents([]whynot.Event{whynot.AnchorClick{ID: "target"}})
	if start, _ := app.Panel.View().VisibleRange(); start == 0 {
		t.Error("an AnchorClick didn't scroll to its heading")
	}
	if !app.CanGoBack() {
		t.Error("CanGoBack() = false after an AnchorClick, want true")
	}

	app.HandleEvents([]whynot.Event{whynot.LinkClick{Destination: "other.md"}})
	if got := filepath.Base(app.Location().Path); got != "other.md" {
		t.Errorf("Location after a LinkClick to other.md = %v, want other.md", app.Location())
	}
}

// TestAppHoverDest checks HoverDest is the hovered link resolved against
// the document's location, and clears when the document changes.
func TestAppHoverDest(t *testing.T) {
	dir := t.TempDir()
	source := "A paragraph with [a link](other.md) in it."
	loc := writeTempMD(t, dir, "doc.md", source)
	writeTempMD(t, dir, "other.md", "# Other")
	app := newTestApp(t, dir, loc, source)

	x, y := linkPos(t, app.Panel.View())
	if got := app.HoverDest(); got != "" {
		t.Errorf("HoverDest before hovering = %q, want none", got)
	}
	app.Panel.Frame([]input.Event{input.PointerMove{X: x, Y: y}}, 0)
	want, _ := app.ResolveLink("other.md")
	if got := app.HoverDest(); got != want.String() {
		t.Errorf("HoverDest over the link = %q, want %q", got, want)
	}

	app.Follow("other.md")
	if got := app.HoverDest(); got != "" {
		t.Errorf("HoverDest after following the link = %q, want none", got)
	}
}

// linkPos returns a point on a link in v.
func linkPos(t *testing.T, v *whynot.View) (x, y int) {
	t.Helper()
	b := v.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			if _, ok := v.LinkAt(x, y); ok {
				return x, y
			}
		}
	}
	t.Fatal("test setup: no link found")
	return 0, 0
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
	if title, _ := app.Panel.View().Document().Title(); title != "Doc B" {
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

	if title, _ := app.Panel.View().Document().Title(); title != "Doc B" {
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
	if title, _ := app.Panel.View().Document().Title(); title != "Doc A" {
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
	if title, _ := app.Panel.View().Document().Title(); title != "Doc B" {
		t.Fatalf("Title() after Follow = %q, want \"Doc B\"", title)
	}

	app.Back()
	if title, _ := app.Panel.View().Document().Title(); title != "Doc A" {
		t.Errorf("Title() after Back = %q, want \"Doc A\"", title)
	}
	if app.CanGoBack() {
		t.Error("CanGoBack() = true after Back to the start of history, want false")
	}
	if !app.CanGoForward() {
		t.Error("CanGoForward() = false after Back, want true")
	}

	app.Forward()
	if title, _ := app.Panel.View().Document().Title(); title != "Doc B" {
		t.Errorf("Title() after Forward = %q, want \"Doc B\"", title)
	}
	if app.CanGoForward() {
		t.Error("CanGoForward() = true after Forward to the end of history, want false")
	}

	// Back with a genuinely empty history (a fresh App) is a no-op.
	fresh := newTestApp(t, dir, locA, "# Doc A")
	fresh.Back()
	if title, _ := fresh.Panel.View().Document().Title(); title != "Doc A" {
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

	if title, _ := app.Panel.View().Document().Title(); title != "Version 2" {
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

	// Check SetTheme reached the panel's View, not just App's own
	// bookkeeping.
	app.SetTheme(false)
	if app.DarkTheme() {
		t.Error("DarkTheme() = true after SetTheme(false), want false")
	}
	if !usesStyleSheet(app.Panel.View(), app.StyleSheet()) {
		t.Error("panel's StyleSheet after SetTheme(false) != App.StyleSheet(), want them in sync")
	}
	lightSheet := app.StyleSheet()

	app.SetTheme(true)
	if !app.DarkTheme() {
		t.Error("DarkTheme() = false after SetTheme(true), want true")
	}
	if !usesStyleSheet(app.Panel.View(), app.StyleSheet()) {
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
	if got := app.Panel.Zoom(); got != 1.5 {
		t.Errorf("Panel.Zoom() after SetZoom(1.5) = %v, want 1.5", got)
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
	if got := app.Panel.Scale(); got != 2 {
		t.Errorf("Panel.Scale() after Relayout = %v, want deviceScale = 2", got)
	}
	if got, want := app.Panel.Zoom(), app.Zoom(); got != want {
		t.Errorf("Panel.Zoom() after Relayout = %v, want %v", got, want)
	}
}

func TestAppOpenFiresOnTitleChange(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# My Title")

	app := NewApp(fonts.NewGoSelector(), simpletheme.DarkStyleSheet, true, NewRegistry(nil))
	view := app.NewView([]byte("# My Title"), loc)
	app.Panel = whynot.NewPanel(view, image.Rectangle{})
	app.Relayout(testWidth, testHeight, 1, 0)

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

func TestAppShowHideTOC(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# Doc A\n\n# One\n\ntext\n\n# Two\n\ntext")
	app := newTestApp(t, dir, loc, "# Doc A\n\n# One\n\ntext\n\n# Two\n\ntext")

	var windowTitle string
	app.OnTitleChange = func(title string) { windowTitle = title }

	if !app.CanShowTOC() {
		t.Fatal("CanShowTOC() = false for a document with headings, want true")
	}
	if app.TOCShowing() {
		t.Fatal("TOCShowing() = true before ShowTOC, want false")
	}

	app.ShowTOC()
	if !app.TOCShowing() {
		t.Fatal("TOCShowing() = false after ShowTOC, want true")
	}
	if title, _ := app.Panel.View().Document().Title(); title != "Table of contents" {
		t.Errorf("Title() while TOC is showing = %q, want \"Table of contents\"", title)
	}
	// The window title stays the document's own, not the TOC view's -
	// see updateWindowTitle's own doc comment.
	if windowTitle != "Doc A - TOC" {
		t.Errorf("window title while TOC is showing = %q, want \"Doc A - TOC\"", windowTitle)
	}

	app.HideTOC()
	if app.TOCShowing() {
		t.Error("TOCShowing() = true after HideTOC, want false")
	}
	if title, _ := app.Panel.View().Document().Title(); title != "Doc A" {
		t.Errorf("Title() after HideTOC = %q, want \"Doc A\"", title)
	}
	if windowTitle != "Doc A" {
		t.Errorf("window title after HideTOC = %q, want \"Doc A\"", windowTitle)
	}
}

func TestAppShowTOCNoOpWithoutHeadings(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "Just a paragraph, no heading anywhere.")
	app := newTestApp(t, dir, loc, "Just a paragraph, no heading anywhere.")

	if app.CanShowTOC() {
		t.Fatal("CanShowTOC() = true for a document with no headings, want false")
	}
	app.ShowTOC()
	if app.TOCShowing() {
		t.Error("TOCShowing() = true after ShowTOC on a heading-less document, want false")
	}
}

func TestAppFollowInTOCJumpsAndCloses(t *testing.T) {
	dir := t.TempDir()
	source := "# Doc A\n\nintro\n\n" + repeatLines(40) + "\n\n# Target\n\nend"
	loc := writeTempMD(t, dir, "a.md", source)
	app := newTestApp(t, dir, loc, source)

	beforeShow := app.Panel.View().ScrollPosition()
	app.ShowTOC()

	// Simulate clicking the "Target" entry - the TOC view's own links
	// are always "#id" destinations into the real document.
	app.Follow("#target")

	if app.TOCShowing() {
		t.Error("TOCShowing() = true after following a TOC entry, want false")
	}
	if got := app.Panel.View().ScrollPosition(); got == beforeShow {
		t.Error("ScrollPosition after following a TOC entry is unchanged, want it to have jumped to the target heading")
	}
	if !app.CanGoBack() {
		t.Error("CanGoBack() = false after following a TOC entry, want true")
	}
	if app.Location().Fragment != "target" {
		t.Errorf("Location().Fragment after following a TOC entry = %q, want \"target\"", app.Location().Fragment)
	}
}

func TestAppBackDismissesTOC(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# Doc A\n\n# One")
	app := newTestApp(t, dir, loc, "# Doc A\n\n# One")

	beforeShow := app.Panel.View().ScrollPosition()
	app.ShowTOC()

	if !app.CanGoBack() {
		t.Error("CanGoBack() = false while the TOC is showing, want true (Back should dismiss it)")
	}

	app.Back()

	if app.TOCShowing() {
		t.Error("TOCShowing() = true after Back, want false")
	}
	if title, _ := app.Panel.View().Document().Title(); title != "Doc A" {
		t.Errorf("Title() after Back dismissed the TOC = %q, want \"Doc A\"", title)
	}
	if got := app.Panel.View().ScrollPosition(); got != beforeShow {
		t.Error("ScrollPosition after Back dismissed the TOC changed, want it left exactly where it was before ShowTOC")
	}
	// Back shouldn't have consumed a real history entry - there was
	// none to begin with.
	if app.CanGoBack() {
		t.Error("CanGoBack() = true after Back dismissed the TOC with otherwise-empty history, want false")
	}
}

func TestAppForwardReloadNavigateDisabledDuringTOC(t *testing.T) {
	dir := t.TempDir()
	locA := writeTempMD(t, dir, "a.md", "# Doc A\n\n# One")
	writeTempMD(t, dir, "b.md", "# Doc B")
	app := newTestApp(t, dir, locA, "# Doc A\n\n# One")

	// Build up real forward history before opening the TOC.
	app.Follow("b.md")
	app.Back()
	if !app.CanGoForward() {
		t.Fatal("test setup: CanGoForward() = false, want true")
	}

	app.ShowTOC()

	if app.CanGoForward() {
		t.Error("CanGoForward() = true while the TOC is showing, want false")
	}
	if app.CanReload() {
		t.Error("CanReload() = true while the TOC is showing, want false")
	}

	app.Forward()
	if !app.TOCShowing() {
		t.Error("Forward() while the TOC was showing left it, want it to have stayed a no-op")
	}

	app.Reload()
	if !app.TOCShowing() {
		t.Error("Reload() while the TOC was showing left it, want it to have stayed a no-op")
	}

	if err := app.Navigate(filepath.Join(dir, "b.md")); err != nil {
		t.Errorf("Navigate() while the TOC was showing = %v, want nil (a no-op)", err)
	}
	if !app.TOCShowing() {
		t.Error("Navigate() while the TOC was showing left it, want it to have stayed a no-op")
	}
}

func TestAppOpenFallsBackToUntitled(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "no heading here")

	app := NewApp(fonts.NewGoSelector(), simpletheme.DarkStyleSheet, true, NewRegistry(nil))
	view := app.NewView([]byte("no heading here"), loc)
	app.Panel = whynot.NewPanel(view, image.Rectangle{})
	app.Relayout(testWidth, testHeight, 1, 0)

	var gotTitle string
	app.OnTitleChange = func(title string) { gotTitle = title }
	app.Open(loc)

	if gotTitle != "Untitled document" {
		t.Errorf("OnTitleChange title for a heading-less document = %q, want \"Untitled document\"", gotTitle)
	}
}

// pipeResolver serves slow: documents from a pipe the test writes to,
// recording whether the fetch was cancelled.
type pipeResolver struct {
	r         *io.PipeReader
	cancelled chan struct{}
}

func (*pipeResolver) Schemes() []string { return []string{"slow"} }

func (p *pipeResolver) Resolve(*url.URL) (fetch.Source, error) { return p, nil }

func (p *pipeResolver) Key() string { return "" }

func (p *pipeResolver) Fetch(ctx context.Context) (io.ReadCloser, string, error) {
	go func() {
		<-ctx.Done()
		close(p.cancelled)
	}()
	return p.r, "text/markdown", nil
}

// TestAppShowsDocumentAsItArrives follows a link to a document that
// arrives in pieces, checks Update shows each piece, then goes Back and
// checks the load was cancelled.
func TestAppShowsDocumentAsItArrives(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "a.md", "# Doc A\n")
	r, w := io.Pipe()
	res := &pipeResolver{r: r, cancelled: make(chan struct{})}
	app := NewApp(fonts.NewGoSelector(), simpletheme.DarkStyleSheet, true, NewRegistry(nil, res))
	app.Panel = whynot.NewPanel(app.NewView(nil, start), image.Rectangle{})
	app.Relayout(testWidth, testHeight, 1, 0)
	if err := app.Open(start); err != nil {
		t.Fatal(err)
	}

	go io.WriteString(w, "# Doc B\n")
	app.Follow("slow:b")
	if !app.Loading() {
		t.Fatal("Loading() = false while the document is still arriving")
	}
	for title := ""; title != "Doc B"; title, _ = app.Panel.View().Document().Title() {
		app.builtAt = time.Time{}
		app.Update()
	}

	app.Back()
	if app.Loading() {
		t.Error("Loading() = true after going Back")
	}
	select {
	case <-res.cancelled:
	case <-time.After(time.Second):
		t.Error("going Back didn't cancel the load")
	}
}
