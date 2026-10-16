package browser

// CanDictate reports whether StartDictation works here: on macOS, when the
// viewer runs from an app bundle. macOS's dictation can't reach an app
// that isn't one: it freezes it for a few seconds and inserts nothing.
func CanDictate() bool { return canDictate }

// StartDictation starts the system's dictation, into the text field that
// has the keyboard, as the dictation key does: the viewer focuses its
// prompt first. The system shows a microphone at the caret, which stops
// it when clicked. It can be called from any goroutine. A no-op unless
// CanDictate.
func StartDictation() {
	if canDictate {
		startDictation()
	}
}

// StopDictation stops the system's dictation, leaving what it wrote in the
// text field, by taking the keyboard from the field and giving it back:
// dictation ends when its field loses the keyboard. It works in the ebiten
// viewer, not in Gio's, whose dictation goes on. The system doesn't say
// whether it's dictating, so callers keep track. It can be called from any
// goroutine. A no-op unless CanDictate.
func StopDictation() {
	if canDictate {
		refocus()
	}
}
