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
