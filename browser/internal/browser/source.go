package browser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/arnodel/whynot"
)

// SourceShowing reports whether the source is showing (see ShowSource).
func (a *App) SourceShowing() bool { return a.overlay == sourceOverlay }

// ShowSource shows the current document's source over it: its Markdown,
// as it arrived. For a whymark page, that's what its turn wrote, and the
// whymark document's own source follows, Lua and all. It replaces the
// table of contents or the actions page, if one is showing.
func (a *App) ShowSource() {
	if a.overlay == sourceOverlay {
		return
	}
	docView := a.Panel.View()
	if a.overlaid != nil {
		docView = a.overlaid
	}
	a.showOverlay(sourceOverlay, a.sourcePage(docView.Document()))
}

// ToggleSource shows the source, or hides it if it's showing.
func (a *App) ToggleSource() {
	if a.overlay == sourceOverlay {
		a.hideOverlay()
		return
	}
	a.ShowSource()
}

// sourcePage returns the Markdown of the page ShowSource shows for doc.
func (a *App) sourcePage(doc *whynot.Document) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Source\n\n%s\n\n", codeFence("text", a.location.String()))
	p := a.pageOf(doc)
	switch {
	case p != nil:
		fmt.Fprintf(&b, "## Page\n\n%s\n\n", codeFence("markdown", string(p.source.bytes())))
		document, err := a.fetchSource(p.header.Location)
		if err != nil {
			document = []byte(fmt.Sprintf("Fetching %s: %v", p.header.Location, err))
		}
		fmt.Fprintf(&b, "## Document\n\n%s\n", codeFence("markdown", string(document)))
	case a.sources[doc] != nil:
		b.WriteString(codeFence("markdown", string(a.sources[doc].bytes())))
	default:
		b.WriteString("This page's source isn't kept.\n")
	}
	return b.Bytes()
}

// fetchSource returns the source at location. A whymark document's isn't
// kept by the browser, which hands the document to the engine; it's
// fetched again, rather than shown from the engine's copy, so that it's
// the source as it is now.
func (a *App) fetchSource(location *url.URL) ([]byte, error) {
	u := *location
	u.RawQuery, u.Fragment, u.RawFragment = "", "", ""
	src, err := a.registry.Resolve(&u, "")
	if err != nil {
		return nil, err
	}
	body, _, err := src.Fetch(context.Background())
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

// codeFence returns text as a fenced code block in the given language,
// with a fence longer than any run of backquotes in text.
func codeFence(language, text string) string {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + language + "\n" + strings.TrimSuffix(text, "\n") + "\n" + fence
}
