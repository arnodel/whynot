package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeMessagesAPI is a Messages API that replies to every request with
// reply, as one text delta, recording each request's body and beta
// header.
type fakeMessagesAPI struct {
	*httptest.Server
	bodies []map[string]any
	betas  []string
}

func newFakeMessagesAPI(t *testing.T, reply string) *fakeMessagesAPI {
	api := &fakeMessagesAPI{}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		api.bodies = append(api.bodies, body)
		api.betas = append(api.betas, r.Header.Get("anthropic-beta"))
		delta, _ := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"delta": map[string]string{"type": "text_delta", "text": reply},
		})
		fmt.Fprintf(w, "data: %s\n\n", delta)
	}))
	t.Cleanup(api.Close)
	return api
}

// TestAnthropicRequests checks the request made for each tier: its model,
// an effort but not for Haiku, which takes none, and fallbacks with their
// beta header for Opus 5.
func TestAnthropicRequests(t *testing.T) {
	api := newFakeMessagesAPI(t, "ok")
	ai := &Anthropic{Endpoint: api.URL, Balanced: "claude-sonnet-x"}
	for _, c := range []struct {
		model, wantModel string
		wantEffort       bool
		wantFallbacks    bool
	}{
		{"", "claude-sonnet-x", true, false},
		{"fast", "claude-haiku-4-5", false, false},
		{"smart", "claude-opus-5", true, true},
		{"claude-opus-4-8", "claude-opus-4-8", true, false},
	} {
		reply, err := ai.Start(context.Background(), AIRequest{
			Model: c.model, Effort: "low",
			Messages: []AIMessage{{Role: "user", Content: "hi"}},
			Schema:   map[string]any{"type": "object"},
		})
		if err != nil {
			t.Fatal(err)
		}
		text, _ := io.ReadAll(reply)
		reply.Close()
		body, beta := api.bodies[len(api.bodies)-1], api.betas[len(api.betas)-1]
		config, _ := body["output_config"].(map[string]any)
		_, effort := config["effort"]
		_, fallbacks := body["fallbacks"]
		if string(text) != "ok" || body["model"] != c.wantModel || effort != c.wantEffort || fallbacks != c.wantFallbacks ||
			(beta != "") != c.wantFallbacks || config["format"] == nil {
			t.Errorf("model %q: reply %q, body %v, beta %q; want model %s, effort %v, fallbacks %v, and a format",
				c.model, text, body, beta, c.wantModel, c.wantEffort, c.wantFallbacks)
		}
	}
}

// TestLuaAIReturns checks that ai{...} with returns sends the shape as a
// JSON schema, and returns the reply as a Lua value, and that a shape
// that isn't an object is wrapped in one.
func TestLuaAIReturns(t *testing.T) {
	api := newFakeMessagesAPI(t, `{"question": "Is it alive?", "guessing": false, "suspects": ["cat", "dog"], "left": 19}`)
	_, page := loadPage(t, &LuaResolver{AI: &Anthropic{Endpoint: api.URL}}, writeScript(t, `
function page()
  local move = ai{
    prompt = "Your move.", model = "smart", effort = "low",
    returns = {question = "string", guessing = "boolean", suspects = {"string"}, left = "integer"},
  }
  out(move.question, " ", tostring(move.guessing), " ", move.suspects[2], " ", math.type(move.left))
end`), "")
	if want := "Is it alive? false dog integer"; page != want {
		t.Errorf("page = %q, want %q", page, want)
	}
	format := api.bodies[0]["output_config"].(map[string]any)["format"].(map[string]any)
	schema, _ := json.Marshal(format["schema"])
	for _, want := range []string{
		`"required":["guessing","left","question","suspects"]`,
		`"suspects":{"items":{"type":"string"},"type":"array"}`,
		`"additionalProperties":false`,
	} {
		if !strings.Contains(string(schema), want) {
			t.Errorf("schema = %s, want it to contain %s", schema, want)
		}
	}

	api = newFakeMessagesAPI(t, `{"value": 7}`)
	_, page = loadPage(t, &LuaResolver{AI: &Anthropic{Endpoint: api.URL}}, writeScript(t, `
function page() out(ai{prompt = "A number.", returns = "integer"} + 1) end`), "")
	if page != "8" {
		t.Errorf("page = %q, want 8: a wrapped integer", page)
	}
}

func TestLuaAIErrors(t *testing.T) {
	api := newFakeMessagesAPI(t, "not JSON")
	for _, c := range []struct{ name, call, want string }{
		{"no prompt", `ai{}`, "ai: no prompt"},
		{"write with returns", `ai.write{prompt = "x", returns = "string"}`, "ai.write: no returns"},
		{"unknown type", `ai{prompt = "x", returns = {a = "text"}}`, `unknown type "text"`},
		{"bad list", `ai{prompt = "x", returns = {"string", "number"}}`, "just one shape"},
		{"not JSON", `ai{prompt = "x", returns = {a = "string"}}`, "isn't the JSON asked for"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, page := loadPage(t, &LuaResolver{AI: &Anthropic{Endpoint: api.URL}}, writeScript(t, "function page() "+c.call+" end"), "")
			if !strings.Contains(page, c.want) {
				t.Errorf("page = %q, want it to mention %q", page, c.want)
			}
		})
	}
}
