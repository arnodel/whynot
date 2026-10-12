package ai

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/arnodel/whynot/browser/internal/whymark"
	"github.com/arnodel/whynot/fetch"
)

// fakeProvider answers "answer N", N counting its answers, or the JSON in
// json for requests with a schema; with incomplete set, it stops halfway.
type fakeProvider struct {
	mu         sync.Mutex
	requests   []Request
	json       string
	incomplete bool
}

func (f *fakeProvider) Ask(ctx context.Context, req Request, w io.Writer) (string, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	answer := fmt.Sprintf("answer %d", len(f.requests))
	f.mu.Unlock()
	if req.Schema != nil {
		answer = f.json
	}
	if f.incomplete {
		answer = answer[:len(answer)/2]
	}
	if w != nil {
		io.WriteString(w, answer)
	}
	if f.incomplete {
		return answer, fmt.Errorf("cut off: %w", ErrIncomplete)
	}
	return answer, nil
}

func (f *fakeProvider) last() Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func (f *fakeProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// testEngine returns an Engine with the AI, serving files, by name, from a
// temporary directory, and a function taking a turn of one and returning
// its page.
func testEngine(t *testing.T, files map[string]string, options ...whymark.Option) (*whymark.Engine, *fakeProvider, func(string) string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	provider := &fakeProvider{}
	e := whymark.NewEngine(fetch.NewRegistry(fetch.FileResolver{Root: root}), append(options, whymark.WithCapabilities(&AI{
		Provider: provider,
		Models:   map[string]string{"fast": "small-model", "smart": "big-model"},
		Default:  "fast",
	}))...)
	take := func(name string) string {
		t.Helper()
		location := &url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, name))}
		r, err := e.Turn(context.Background(), whymark.Request{Location: location})
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		br := bufio.NewReader(r)
		if _, err := whymark.ReadHeader(br); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(br)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	return e, provider, take
}

const usesAI = "---\nwhymark:\n  version: 1\n  uses:\n    ai: optional\n---\n"

func wants(t *testing.T, what, page string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(page, w) {
			t.Errorf("%s: the page doesn't have %q:\n%s", what, w, page)
		}
	}
}

func wantsNot(t *testing.T, what, page string, wantNot ...string) {
	t.Helper()
	for _, w := range wantNot {
		if strings.Contains(page, w) {
			t.Errorf("%s: the page has %q:\n%s", what, w, page)
		}
	}
}

// TestBlocks checks ai blocks, whose lines make a prompt and whose answer
// is written where they are, and without ai blocks, shown only without the
// AI.
func TestBlocks(t *testing.T) {
	body := "# Shop\n\n```lua\nlocal oil = true\nai = require \"ai\"\nif ai then ai.system = \"Be terse.\" end\n```\n\n" +
		"Before.\n\n```ai\nDescribe the shop.\n`if oil then`\nThey hold oil.\n`end`\n```\n```without ai\nIt's quiet.\n```\n\nAfter.\n"
	allow := true
	_, provider, take := testEngine(t, map[string]string{
		"shop.md":   usesAI + body,
		"plain.md":  "---\nwhymark:\n  version: 1\n---\n" + strings.ReplaceAll(body, "ai = require \"ai\"", "ai = false"),
		"twice.md":  usesAI + "# Twice\n\n```ai\nSay something.\n```\n\n```ai\nSay something.\n```\n",
		"silent.md": usesAI + "# Silent\n\n```without ai\nNo AI here.\n```\n",
	}, whymark.WithGrant(func(*url.URL, string) bool { return allow }))
	page := take("shop.md")
	wants(t, "with the AI", page, "Before.\n\nanswer 1", "After.")
	wantsNot(t, "with the AI", page, "It's quiet", "Describe")
	req := provider.last()
	if got := req.Messages[0].Content; got != "Describe the shop.\nThey hold oil." {
		t.Errorf("the prompt is %q", got)
	}
	if !strings.HasPrefix(req.System, writeSystem) || !strings.HasSuffix(req.System, "\n\nBe terse.") || req.Model != "small-model" {
		t.Errorf("the request is %+v, want the host's and the document's system prompt, and the fast model", req)
	}
	wants(t, "a new page", take("shop.md"), "answer 2")
	wants(t, "the same call twice in a page", take("twice.md"), "answer 3", "answer 4")
	if n := provider.count(); n != 4 {
		t.Errorf("the provider was asked %d times, want 4", n)
	}

	plain := take("plain.md")
	wants(t, "not declared, so code", plain, "```ai\nDescribe the shop.", "```without ai\nIt's quiet.\n```", "After.")
	wantsNot(t, "not declared", plain, "answer")

	allow = false
	wants(t, "not granted", take("shop.md"), "It's quiet.")
	wants(t, "nothing to ask", take("silent.md"), "No AI here.")
	if n := provider.count(); n != 4 {
		t.Errorf("without the AI, the provider was asked %d more times", n-4)
	}
}

// TestCalls checks ai{...} and ai.write{...}: keyed answers, shared across
// pages, conversations, models and returned shapes.
func TestCalls(t *testing.T) {
	_, provider, take := testEngine(t, map[string]string{
		"keyed.md": usesAI + "# Keyed\n\n```lua\nlocal ai = require \"ai\"\nturn.write(ai{prompt = \"Name a town.\", key = \"town\"} .. \" / \" .. ai{prompt = \"Name a town.\"})\n```\n",
		"chat.md": usesAI + "# Chat\n\n```lua\nlocal ai = require \"ai\"\n" +
			"ai.write{messages = {{role = \"user\", content = \"Hi\"}, {role = \"ai\", content = \"Hello!\"}}, prompt = \"How are you?\", model = \"smart\", system = \"Be kind.\"}\n```\n",
		"shape.md": usesAI + "# Shape\n\n```lua\nlocal ai = require \"ai\"\n" +
			"local r = ai{prompt = \"Judge.\", returns = {mood = {one_of = {\"calm\", \"angry\"}}, score = \"integer\", tags = {\"string\"}}}\n" +
			"turn.write(r.mood .. \" \" .. math.type(r.score) .. \" \" .. table.concat(r.tags, \",\"))\n```\n",
		"bool.md":  usesAI + "# Bool\n\n```lua\nlocal ai = require \"ai\"\nturn.write(tostring(ai{prompt = \"Yes?\", returns = \"boolean\"}))\n```\n",
		"bad.md":   usesAI + "# Bad\n\n```lua\nlocal ai = require \"ai\"\nai{messages = {{role = \"ai\", content = \"Hi\"}}}\n```\n",
		"empty.md": usesAI + "# Empty\n\n```lua\nlocal ai = require \"ai\"\nai{}\n```\n",
	})
	wants(t, "keyed", take("keyed.md"), "answer 1 / answer 2")
	wants(t, "keyed, on a new page", take("keyed.md"), "answer 1 / answer 3")

	wants(t, "a conversation", take("chat.md"), "answer 4")
	req := provider.last()
	if len(req.Messages) != 3 || req.Messages[1] != (Message{Role: "assistant", Content: "Hello!"}) || req.Messages[2].Content != "How are you?" {
		t.Errorf("the messages are %+v", req.Messages)
	}
	if req.Model != "big-model" || !strings.HasSuffix(req.System, "Be kind.") {
		t.Errorf("the request is %+v, want the smart model and the call's system prompt", req)
	}

	provider.json = `{"mood": "calm", "score": 3, "tags": ["a", "b"]}`
	wants(t, "an object", take("shape.md"), "calm integer a,b")
	if s := provider.last().Schema; s["type"] != "object" || !strings.HasPrefix(provider.last().System, askSystem) {
		t.Errorf("the schema is %v, want an object, asked with the host's prompt for answers", s)
	}
	provider.json = `{"value": true}`
	wants(t, "a wrapped value", take("bool.md"), "true")

	wants(t, "a conversation ending with the AI", take("bad.md"), "The page stopped", "user's turn")
	wants(t, "no prompt", take("empty.md"), "The page stopped", "no prompt")
}

// TestIncomplete checks that an answer that stops before its end stays on
// the page, said to be incomplete, and isn't recorded.
func TestIncomplete(t *testing.T) {
	_, provider, take := testEngine(t, map[string]string{
		"cut.md": usesAI + "# Cut\n\n```lua\nlocal ai = require \"ai\"\nai.write{prompt = \"Go on.\", key = \"k\"}\n```\n",
	})
	provider.incomplete = true
	wants(t, "cut off", take("cut.md"), "answ", "stopped before its end")
	provider.incomplete = false
	wants(t, "again", take("cut.md"), "answer 2")
}
