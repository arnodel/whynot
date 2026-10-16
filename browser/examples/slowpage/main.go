// Command slowpage serves a Markdown page slowly, a few lines at a time with
// pauses of varying length in between, to see how a viewer shows a page
// that's still arriving: the waiting dots show after a second without new
// text. Run it, then open its address in a viewer:
//
//	go run ./examples/slowpage
//	go run ./cmd/whynot http://localhost:8765/
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

// parts are the page's pieces, each sent after its pause.
var parts = []struct {
	pause time.Duration
	text  string
}{
	{0, "# A slow page\n\nThis page arrives in pieces, with pauses between them. After a second without new text, the waiting dots show under it.\n\n"},
	{300 * time.Millisecond, "A short pause: no dots.\n\n"},
	{300 * time.Millisecond, "Another short one.\n\n"},
	{2 * time.Second, "That was two seconds: the dots showed for about one.\n\n"},
	{500 * time.Millisecond, "Then text streams in steadily, "},
	{200 * time.Millisecond, "a few words "},
	{200 * time.Millisecond, "at a time, "},
	{200 * time.Millisecond, "as an AI writes.\n\n"},
	{4 * time.Second, "Four seconds, the longest wait.\n\n"},
	{1500 * time.Millisecond, "One and a half seconds.\n\n"},
	{800 * time.Millisecond, "- a list\n"},
	{800 * time.Millisecond, "- that grows\n"},
	{800 * time.Millisecond, "- item by item\n\n"},
	{3 * time.Second, "And three seconds before the end. The page is complete now: no more dots.\n"},
}

func main() {
	addr := flag.String("addr", "localhost:8765", "the address to serve on")
	flag.Parse()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		flusher, _ := w.(http.Flusher)
		for _, p := range parts {
			select {
			case <-time.After(p.pause):
			case <-r.Context().Done():
				return // the viewer went elsewhere
			}
			fmt.Fprint(w, p.text)
			if flusher != nil {
				flusher.Flush()
			}
		}
	})
	log.Printf("serving a slow page at http://%s/", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
