package kroki

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRendererCanHandle(t *testing.T) {
	r := Renderer{}
	if !r.CanHandle("mermaid") {
		t.Error(`CanHandle("mermaid") = false, want true`)
	}
	if r.CanHandle("go") {
		t.Error(`CanHandle("go") = true, want false`)
	}
}

// TestDiagramImageKeyDistinguishesCodeAndType checks Key() differs for
// different source text or diagram type - ImageCache's own caching
// relies on this to tell distinct diagrams apart.
func TestDiagramImageKeyDistinguishesCodeAndType(t *testing.T) {
	r := Renderer{}
	a := r.Image("mermaid", "graph TD; A-->B;")
	b := r.Image("mermaid", "graph TD; A-->C;")
	if a.Key() == b.Key() {
		t.Errorf("Key() for different source text matched: %q", a.Key())
	}
	if a.Key() != r.Image("mermaid", "graph TD; A-->B;").Key() {
		t.Error("Key() differed for identical (language, code) - want a stable cache key")
	}
}

// TestDiagramImageFetchPostsExpectedRequest checks Fetch's request
// shape against a fake Kroki server, rather than the real network: the
// diagram type in the URL path, PNG as the requested format, and the
// diagram source as JSON in the body.
func TestDiagramImageFetchPostsExpectedRequest(t *testing.T) {
	const pngBytes = "not a real PNG, just a fixed body to check round-tripping"
	var gotPath, gotMethod, gotContentType string
	var gotBody struct {
		DiagramSource string `json:"diagram_source"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		gotMethod = req.Method
		gotContentType = req.Header.Get("Content-Type")
		if err := json.NewDecoder(req.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.Write([]byte(pngBytes))
	}))
	defer server.Close()

	r := Renderer{BaseURL: server.URL}
	img := r.Image("mermaid", "graph TD; A-->B;")
	rc, err := img.Fetch()
	if err != nil {
		t.Fatalf("Fetch() = _, %v, want nil error", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/mermaid/png" {
		t.Errorf("path = %q, want \"/mermaid/png\"", gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want \"application/json\"", gotContentType)
	}
	if gotBody.DiagramSource != "graph TD; A-->B;" {
		t.Errorf("diagram_source = %q, want the diagram's own source", gotBody.DiagramSource)
	}
	if string(got) != pngBytes {
		t.Errorf("Fetch() body = %q, want %q", got, pngBytes)
	}
}

// TestDiagramImageFetchNonOKStatus checks that a non-200 response is
// reported as an error rather than silently returned as if it were
// image data.
func TestDiagramImageFetchNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "bad diagram source", http.StatusBadRequest)
	}))
	defer server.Close()

	r := Renderer{BaseURL: server.URL}
	img := r.Image("mermaid", "not valid mermaid")
	if _, err := img.Fetch(); err == nil {
		t.Error("Fetch() with a 400 response = nil error, want one")
	}
}
