//go:build darwin && !ios

package browser

import (
	"os"
	"strings"

	"github.com/ebitengine/purego/objc"
)

var canDictate = inAppBundle()

// inAppBundle reports whether the running executable is an app bundle's.
func inAppBundle() bool {
	exe, err := os.Executable()
	return err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}

// Through purego, so that the ebiten viewer needs no cgo.
var (
	selSharedApplication  = objc.RegisterName("sharedApplication")
	selPerformOnMain      = objc.RegisterName("performSelectorOnMainThread:withObject:waitUntilDone:")
	selStartDictation     = objc.RegisterName("startDictation:")
	selKeyWindow          = objc.RegisterName("keyWindow")
	selFirstResponder     = objc.RegisterName("firstResponder")
	selMakeFirstResponder = objc.RegisterName("makeFirstResponder:")
)

func sharedApplication() objc.ID {
	return objc.ID(objc.GetClass("NSApplication")).Send(selSharedApplication)
}

// startDictation sends startDictation: to the application, as AppKit's
// Start Dictation menu item does, on the main thread, where AppKit wants
// it.
func startDictation() {
	sharedApplication().Send(selPerformOnMain, selStartDictation, objc.ID(0), false)
}

// refocus makes the key window drop its first responder and take it back,
// on the main thread.
func refocus() {
	win := sharedApplication().Send(selKeyWindow)
	if win == 0 {
		return
	}
	responder := win.Send(selFirstResponder)
	win.Send(selPerformOnMain, selMakeFirstResponder, objc.ID(0), false)
	win.Send(selPerformOnMain, selMakeFirstResponder, responder, false)
}
