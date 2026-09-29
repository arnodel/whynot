package browser

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestFetchDocumentErrorKinds checks the errors App uses to decide what to
// do with a link: an HTML page is a webPageError (open it in a web
// browser), and a request that gets no response at all is a requestError
// - which only the browser build turns into a webPageError.
func TestFetchDocumentErrorKinds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page.html" {
			w.Header().Set("Content-Type", "text/html")
		} else {
			w.Header().Set("Content-Type", "text/markdown")
		}
		w.Write([]byte("# Hello"))
	}))
	get := func(path string) error {
		t.Helper()
		u, err := url.Parse(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fetchDocument(u)
		return err
	}

	if err := get("/doc.md"); err != nil {
		t.Errorf("Markdown document: err = %v, want nil", err)
	}
	var pageErr *webPageError
	if err := get("/page.html"); !errors.As(err, &pageErr) {
		t.Errorf("HTML page: err = %v, want a *webPageError", err)
	}

	server.Close()
	err := get("/doc.md")
	var reqErr *requestError
	if !errors.As(err, &reqErr) {
		t.Errorf("unreachable server: err = %v, want a *requestError", err)
	}
	if errors.As(err, &pageErr) {
		t.Errorf("unreachable server: err = %v, is a *webPageError, want only the browser build to treat it as one", err)
	}
}
