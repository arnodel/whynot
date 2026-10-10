package browser

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/arnodel/whynot"
)

// TestLoadDocumentChecksMediaType checks what loadDocument does with
// what it fetches: Markdown or other text is a document, an HTML page is
// a webPageError (App opens it in a web browser), and anything else is
// an error.
func TestLoadDocumentChecksMediaType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page.html":
			w.Header().Set("Content-Type", "text/html")
		case "/image.png":
			w.Header().Set("Content-Type", "image/png")
		case "/doc.md":
			w.Header().Set("Content-Type", "text/markdown")
		}
		w.Write([]byte("# Hello"))
	}))
	defer server.Close()
	registry := NewRegistry(nil)
	load := func(path string) error {
		t.Helper()
		u, err := url.Parse(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = loadDocument(whynot.NewParser(), registry, u)
		return err
	}

	if err := load("/doc.md"); err != nil {
		t.Errorf("Markdown document: err = %v, want nil", err)
	}
	var pageErr *webPageError
	if err := load("/page.html"); !errors.As(err, &pageErr) {
		t.Errorf("HTML page: err = %v, want a *webPageError", err)
	}
	if err := load("/image.png"); err == nil || errors.As(err, &pageErr) {
		t.Errorf("image: err = %v, want an error other than *webPageError", err)
	}
}

func TestCheckMarkdown(t *testing.T) {
	location := &url.URL{Scheme: "file", Path: "/doc"}
	markdown := []byte("# Title\n\nSome text.")
	readme := []byte("<p align=\"center\">\n  <img src=\"logo.png\">\n</p>\n\n# Project")
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	var pageErr *webPageError
	for _, c := range []struct {
		name      string
		mediaType string
		data      []byte
		wantErr   bool
		wantPage  bool
	}{
		{"declared Markdown", "text/markdown", markdown, false, false},
		{"other text", "text/plain", markdown, false, false},
		{"declared HTML", "text/html", markdown, true, true},
		{"declared image", "image/png", png, true, false},
		{"unknown Markdown", "", markdown, false, false},
		{"unknown Markdown starting with HTML", "", readme, false, false},
		{"unknown binary", "", png, true, false},
	} {
		err := checkMarkdown(c.mediaType, c.data, location)
		if (err != nil) != c.wantErr || errors.As(err, &pageErr) != c.wantPage {
			t.Errorf("%s: err = %v, want error %v, web page %v", c.name, err, c.wantErr, c.wantPage)
		}
	}
}

// TestLoadDocumentWelcome checks the welcome page is served by the
// registry, like any other document.
func TestLoadDocumentWelcome(t *testing.T) {
	doc, err := loadDocument(whynot.NewParser(), NewRegistry(nil), WelcomeURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Title(); !ok {
		t.Error("welcome page has no title")
	}
}
