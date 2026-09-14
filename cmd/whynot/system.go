package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
)

// readClipboard returns the clipboard's current text content, via each
// OS's own command-line paste utility rather than a third-party
// clipboard package - ebiten has no clipboard API of its own, and this
// is a small enough need (cmd/whynot's own convenience, not the
// library's) that it isn't worth a module dependency for.
func readClipboard() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard")
	default:
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("no clipboard utility found (tried xclip, xsel)")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// openInBrowser hands rawURL to the OS's own default handler - a real
// web browser, unlike whynot itself - the same per-OS dispatch
// readClipboard uses, so no extra dependency is needed for this either.
// Runs in the background: the launcher command (open/xdg-open/start)
// exits as soon as it's handed the URL off, not when the browser itself
// closes, but spawning it is still enough to briefly block the caller
// on some platforms, so this doesn't wait for it from the game loop.
func openInBrowser(rawURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		// The empty argument is the window title `start` itself expects
		// - without it, start misreads a quoted URL as the title.
		cmd = exec.Command("cmd", "/c", "start", "", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	go func() {
		if err := cmd.Run(); err != nil {
			log.Printf("opening %s in the browser: %v", rawURL, err)
		}
	}()
}
