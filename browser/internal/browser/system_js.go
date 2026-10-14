package browser

import (
	"errors"
	"log"
	"syscall/js"
)

// readClipboard is system_notjs.go's counterpart. There's no clipboard
// access here: the browser's own API (navigator.clipboard.readText) is
// asynchronous and asks the user for permission.
func readClipboard() (string, error) {
	return "", errors.New("reading the clipboard isn't supported in the browser")
}

// writeClipboard puts text on the clipboard, with the browser's API, which
// works only on a secure (https) page, shortly after a user action, such
// as the key press or click that asked for it. The browser answers later,
// so a refusal is only logged: nil means the request was made.
func writeClipboard(text string) error {
	clipboard := js.Global().Get("navigator").Get("clipboard")
	if clipboard.IsUndefined() {
		return errors.New("the clipboard isn't available here")
	}
	var done, refused js.Func
	release := func() { done.Release(); refused.Release() }
	done = js.FuncOf(func(this js.Value, args []js.Value) any {
		release()
		return nil
	})
	refused = js.FuncOf(func(this js.Value, args []js.Value) any {
		log.Printf("copying to the clipboard: refused by the browser: %s", args[0].Call("toString").String())
		release()
		return nil
	})
	clipboard.Call("writeText", text).Call("then", done, refused)
	return nil
}

// openInBrowser opens rawURL in a new tab. Browsers only allow that
// shortly after a user action (e.g. the click on a link), so a slow
// page load before this is called can get it blocked as a popup.
func openInBrowser(rawURL string) {
	tab := js.Global().Call("open", rawURL, "_blank")
	if tab.IsNull() {
		log.Printf("opening %s in a new tab: blocked by the browser", rawURL)
		return
	}
	// Don't let the opened page script this one via window.opener.
	tab.Set("opener", js.Null())
}
