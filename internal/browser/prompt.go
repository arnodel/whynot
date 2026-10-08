package browser

import (
	"log"
	"net/url"
	"strings"
)

// A link can ask the reader for text: its destination holds the
// placeholder {}, as in [Say something](?say={}). Following it doesn't
// navigate: the app shows a text field instead, and the reader's text
// replaces the placeholder in the link, which is then followed.

// placeholders are how {} appears in a destination: as written, or
// escaped, as in a generated claude: link.
var placeholders = []string{"{}", "%7B%7D", "%7b%7d"}

func hasPlaceholder(dest string) bool {
	for _, p := range placeholders {
		if strings.Contains(dest, p) {
			return true
		}
	}
	return false
}

// Prompting reports whether a link is waiting for the reader's text, which
// the app should ask for, then pass to SubmitPrompt, or CancelPrompt.
func (a *App) Prompting() bool { return a.prompt != "" }

// SubmitPrompt follows the waiting link with text in place of its
// placeholder, escaped for where it is: in the query, or in the path.
func (a *App) SubmitPrompt(text string) {
	dest := a.prompt
	a.prompt = ""
	for _, p := range placeholders {
		i := strings.Index(dest, p)
		if i < 0 {
			continue
		}
		escaped := url.PathEscape(text)
		if strings.Contains(dest[:i], "?") {
			escaped = url.QueryEscape(text)
		}
		// Not Follow, which would take a {} in text for another
		// placeholder.
		resolved, err := a.ResolveLink(dest[:i] + escaped + dest[i+len(p):])
		if err != nil {
			log.Printf("link destination %q: %v", dest, err)
			return
		}
		a.follow(resolved)
		return
	}
}

// CancelPrompt forgets the waiting link.
func (a *App) CancelPrompt() { a.prompt = "" }

// ClipboardText returns the clipboard's text, for pasting into a text
// field.
func ClipboardText() (string, error) { return readClipboard() }
