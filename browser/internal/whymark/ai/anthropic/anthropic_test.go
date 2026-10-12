package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/arnodel/whynot/browser/internal/whymark/ai"
)

// server answers each request with an event stream of the text in pieces,
// stopping for stopReason, and keeps the last request's body.
func server(t *testing.T, pieces []string, stopReason string, body *map[string]any) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			t.Errorf("the API key is %q", r.Header.Get("X-Api-Key"))
		}
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		event := func(name, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
		event("message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)
		event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		for _, p := range pieces {
			text, _ := json.Marshal(p)
			event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+string(text)+`}}`)
		}
		event("content_block_stop", `{"type":"content_block_stop","index":0}`)
		event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"`+stopReason+`","stop_sequence":null},"usage":{"output_tokens":5}}`)
		event("message_stop", `{"type":"message_stop"}`)
	}))
	t.Cleanup(s.Close)
	return s
}

// TestAsk checks a request as sent, and its answer streamed and returned.
func TestAsk(t *testing.T) {
	var body map[string]any
	s := server(t, []string{"Hello", ", reader."}, "end_turn", &body)
	p := New("test-key", option.WithBaseURL(s.URL), option.WithMaxRetries(0))
	var w strings.Builder
	text, err := p.Ask(context.Background(), ai.Request{
		Model:    "claude-sonnet-5",
		System:   "Be kind.",
		Messages: []ai.Message{{Role: "user", Content: "Hi"}, {Role: "assistant", Content: "Hello!"}, {Role: "user", Content: "Again"}},
		Effort:   "low",
		Schema:   map[string]any{"type": "object"},
	}, &w)
	if err != nil || text != "Hello, reader." || w.String() != text {
		t.Errorf("Ask = %q, %v, streaming %q", text, err, w.String())
	}
	sent, _ := json.Marshal(body)
	for _, want := range []string{
		`"model":"claude-sonnet-5"`, `"system":[{"text":"Be kind.","type":"text"}]`, `"role":"assistant"`,
		`"thinking":{"type":"adaptive"}`, `"effort":"low"`, `"format":{"schema":{"type":"object"},"type":"json_schema"}`, `"stream":true`,
	} {
		if !strings.Contains(string(sent), want) {
			t.Errorf("the request doesn't have %s: %s", want, sent)
		}
	}
}

// TestIncomplete checks that an answer cut off at its length limit
// returns what there was, and an error wrapping ai.ErrIncomplete.
func TestIncomplete(t *testing.T) {
	var body map[string]any
	s := server(t, []string{"Once upon"}, "max_tokens", &body)
	p := New("test-key", option.WithBaseURL(s.URL), option.WithMaxRetries(0))
	text, err := p.Ask(context.Background(), ai.Request{Model: "claude-haiku-4-5", Messages: []ai.Message{{Role: "user", Content: "Go"}}}, nil)
	if text != "Once upon" || !errors.Is(err, ai.ErrIncomplete) {
		t.Errorf("Ask = %q, %v, want what there was, and incomplete", text, err)
	}
	if _, ok := body["thinking"]; ok {
		t.Errorf("a request without an effort thinks: %v", body)
	}
}
