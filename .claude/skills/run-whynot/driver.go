// Command driver launches cmd/whynot as a headless exp/vmhost guest, optionally scrolls it by
// injecting mouse-wheel input, and dumps the rendered frame as a PNG. It is the agent-facing way to
// run and screenshot whynot without a visible window - see SKILL.md in this directory.
//
// Written against ebiten v2.10.0-alpha.12 (the version pinned in go.mod); exp/vmhost is
// experimental, so if go.mod's ebiten version changes, re-verify this against the matching copy of
// the run-ebitengine-app-headless skill vendored at
// $(go env GOMODCACHE)/github.com/hajimehoshi/ebiten/v2@<version>/.agents/skills/run-ebitengine-app-headless/.
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/vmhost"
)

var (
	pkg         = flag.String("pkg", "./cmd/whynot", "package path of the guest app (repo-root-relative)")
	source      = flag.String("source", "cmd/whynot/test.md", "markdown file to render, passed to the guest as its file argument")
	out         = flag.String("out", "frame.png", "PNG output path for the final frame")
	logicalW    = flag.Int("w", 1024, "logical screen width in device-independent pixels")
	logicalH    = flag.Int("h", 768, "logical screen height in device-independent pixels")
	wheelDy     = flag.Float64("wheel-dy", -50, "wheel dy re-injected on every tick of the scroll phase (negative scrolls down)")
	scrollTicks = flag.Int("scroll-ticks", 0, "number of ticks to hold the wheel at -wheel-dy before releasing it; 0 skips scrolling")
	ticks       = flag.Int("ticks", 5, "settle ticks to run after scrolling, immediately before capturing the frame")
	cursorX     = flag.Float64("cursor-x", -1, "cursor x in device-independent pixels (same space as -w/-h) to move to before capturing; negative skips moving the cursor")
	cursorY     = flag.Float64("cursor-y", -1, "cursor y in device-independent pixels (same space as -w/-h) to move to before capturing; negative skips moving the cursor")
	click       = flag.Bool("click", false, "press and release the left mouse button (at -cursor-x/-cursor-y) after scrolling, before the settle ticks - e.g. to follow a link under the cursor")
	pressKey    = flag.String("key", "", "name of an ebiten.Key (e.g. \"Backspace\") to press and release after scrolling, before the settle ticks - e.g. to trigger a back action")
	debugHit    = flag.Bool("debug-hit", false, "pass -debug-hit through to the guest, so the captured frame shows the red HitTest outline at the cursor")
)

// driver is the host: an ebiten.Game that drives one guest and composites its final frame into its
// own (hidden) screen image. It does all its work in a single Update, then terminates.
type driver struct {
	guest    *vmhost.GuestSession
	screen   *ebiten.Image
	dumped   bool
	closed   bool
	closeErr error
}

func (d *driver) Update() error {
	defer func() {
		if err := d.guest.Close(); err != nil {
			d.closeErr = err
		}
		d.closed = true
	}()

	// Create the host-owned screen and size the guest to it. SetOutsideScreen must precede
	// AdvanceTicks. The guest renders at the host's device scale factor, so the image is sized in
	// physical pixels.
	scale := ebiten.Monitor().DeviceScaleFactor()
	d.screen = ebiten.NewImage(int(float64(*logicalW)*scale), int(float64(*logicalH)*scale))
	if err := d.guest.SetOutsideScreen(d.screen); err != nil {
		return err
	}

	if *cursorX >= 0 && *cursorY >= 0 {
		d.guest.MoveCursor(*cursorX, *cursorY)
	}

	// Wheel deltas are consumed and reset every tick (like a real wheel event), unlike a held key -
	// so re-inject before each individual tick to simulate the wheel being held down for a run,
	// rather than injecting once before a big AdvanceTicks(N).
	for i := 0; i < *scrollTicks; i++ {
		d.guest.ScrollWheel(0, *wheelDy)
		d.guest.AdvanceTicks(1)
	}

	// Press and release each span at least one tick apart - inpututil's
	// IsMouseButtonJustPressed/IsKeyJustPressed (what cmd/whynot's own
	// input handling uses) only fire on the tick a press is first seen,
	// so a same-tick press+release could be missed entirely.
	if *click {
		d.guest.PressMouseButton(ebiten.MouseButtonLeft)
		d.guest.AdvanceTicks(1)
		d.guest.ReleaseMouseButton(ebiten.MouseButtonLeft)
		d.guest.AdvanceTicks(1)
	}
	if *pressKey != "" {
		var key ebiten.Key
		if err := key.UnmarshalText([]byte(*pressKey)); err != nil {
			return fmt.Errorf("-key %q: %w", *pressKey, err)
		}
		d.guest.PressKey(key)
		d.guest.AdvanceTicks(1)
		d.guest.ReleaseKey(key)
		d.guest.AdvanceTicks(1)
	}

	d.guest.AdvanceTicks(*ticks)

	// Render the final frame. WaitFrame blocks until every queued tick has run and the frame is
	// rendered, so the screen reflects the end of the run.
	d.guest.AdvanceFrame()
	if !d.guest.WaitFrame() {
		return ebiten.Termination
	}
	if !d.guest.CompositeFrame() {
		return errors.New("compositing the guest frame failed")
	}

	if err := d.dump(); err != nil {
		return err
	}
	d.dumped = true
	return ebiten.Termination
}

// dump writes the current screen to a PNG. whynot never fills a full-canvas background (only
// glyphs, rules, and cell borders draw anything), so untouched pixels come back from ReadPixels as
// transparent black rather than the opaque black a real window shows there (a live window ignores
// alpha and just displays raw RGB, which happens to be black for an untouched pixel). ReadPixels
// returns premultiplied-alpha RGBA, so "premultiplied color composited over opaque black" is just
// the premultiplied RGB bytes as-is - forcing alpha to 255 without touching RGB reproduces exactly
// what the real window shows, including correctly darkened antialiased glyph edges. Skipping this
// leaves a PNG whose background various viewers render inconsistently (transparent black composited
// to white, gray, or black depending on the viewer), rather than the app's actual appearance.
func (d *driver) dump() error {
	b := d.screen.Bounds()
	img := image.NewRGBA(b)
	d.screen.ReadPixels(img.Pix)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func (d *driver) Draw(screen *ebiten.Image) {
	// Nothing to present: the host window is hidden, and the guest's frame is composited into
	// d.screen in Update.
}

func (d *driver) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	if err := xmain(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func xmain() error {
	flag.Parse()

	// A short temp dir keeps the unix socket path within the OS limit (~104 bytes on macOS).
	dir, err := os.MkdirTemp("", "vg")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	ln, err := net.Listen("unix", filepath.Join(dir, "g.sock"))
	if err != nil {
		return err
	}
	defer func() {
		_ = ln.Close()
	}()

	endpoint, err := vmhost.EndpointURLFromAddr(ln.Addr())
	if err != nil {
		return err
	}

	// Build the guest with the ebitenginevm tag so its RunGame connects to this host instead of
	// opening a window. cmd/whynot's own source is unchanged.
	guestBin := filepath.Join(dir, "guest")
	build := exec.Command("go", "build", "-tags", "ebitenginevm", "-o", guestBin, *pkg)
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building the guest failed: %w", err)
	}

	var guestArgs []string
	if *debugHit {
		// Flags must precede the positional source path - cmd/whynot's
		// own flag.Parse() stops parsing flags at the first non-flag
		// argument.
		guestArgs = append(guestArgs, "-debug-hit")
	}
	if *source != "" {
		abs, err := filepath.Abs(*source)
		if err != nil {
			return err
		}
		guestArgs = append(guestArgs, abs)
	}
	cmd := exec.Command(guestBin, guestArgs...)
	cmd.Env = append(os.Environ(), "EBITENGINE_VM_ENDPOINT="+endpoint)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting the guest failed: %w", err)
	}
	var processDone bool
	defer func() {
		if processDone {
			return
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if dl, ok := ln.(interface{ SetDeadline(time.Time) error }); ok {
		if err := dl.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return err
		}
	}
	conn, err := ln.Accept()
	if err != nil {
		return fmt.Errorf("accepting the guest failed: %w", err)
	}

	guest, err := vmhost.NewGuestSession(conn, &vmhost.NewGuestSessionOptions{IdleTimeout: 10 * time.Second})
	if err != nil {
		return err
	}

	ebiten.SetWindowVisible(false)
	d := &driver{guest: guest}
	runErr := ebiten.RunGame(d)

	if d.closeErr != nil {
		slog.Warn("closing the guest session failed", "err", d.closeErr)
	}
	if !d.closed {
		_ = cmd.Process.Kill()
	}
	if err := cmd.Wait(); err != nil {
		slog.Warn("waiting for the guest process failed", "err", err)
	}
	processDone = true

	if runErr != nil {
		return fmt.Errorf("host run failed: %w", runErr)
	}
	if err := guest.Err(); err != nil && !errors.Is(err, ebiten.Termination) {
		return fmt.Errorf("guest session failed: %w", err)
	}
	if !d.dumped {
		if err := guest.Err(); err != nil {
			return fmt.Errorf("the guest ended before the frame was captured: %w", err)
		}
		return errors.New("no frame was captured; the run must advance at least one tick")
	}
	slog.Info("wrote frame", "path", *out, "ticks", *ticks, "scrollTicks", *scrollTicks)
	return nil
}
