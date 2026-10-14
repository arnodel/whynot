// Package ai is the AI capability of whymark documents: a document
// declaring "ai" in its front matter, and allowed to use it, reaches it
// with require "ai", whose module asks an AI, through a Provider.
//
// The module is a table, callable as ai{...} to ask and return the
// answer, and with ai.write{...} to ask and write the answer into the page
// as it arrives (and return it). An ai block is ai.write{prompt = text},
// with the block's text. Both take:
//
//   - prompt: the question, a string;
//   - messages: a conversation so far, a list of {role = "user" or "ai",
//     content = ...}, to which prompt adds the reader's last turn;
//   - system: instructions, after the host's and the module's own system
//     field, a document's default;
//   - model: a tier ("fast", "balanced" or "smart"), or a model id the
//     provider knows; the module's model field is the default;
//   - effort: how hard to think, if the provider supports it;
//   - returns, for ai{...} only: the shape of the answer, which is then a
//     Lua value: "string", "number", "integer" or "boolean" for a value of
//     that type, {shape} for a list of shape, a table of shapes for an
//     object with those fields, and {one_of = {...}} for one of a list of
//     strings;
//   - key: shares the answer across the document (see below).
//
// An answer belongs to the page that asked it: it's recorded by the page's
// id and the call, so a new page, even for the same request, gets new
// answers, and the page made again, the same ones. A call with a key gets
// the answer recorded for that key and call, wherever in the document
// it's made. Only complete answers are recorded.
package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	rt "github.com/arnodel/golua/runtime"

	"github.com/arnodel/whynot/browser/internal/whymark"
)

// A Provider answers requests: an AI, reached through its API, or a test's
// canned answers.
type Provider interface {
	// Ask answers req, writing the answer to w as it arrives if w isn't
	// nil, and returns it whole. An answer that stops before its end
	// (cut off at its length limit, or declined) returns what there was,
	// and an error wrapping ErrIncomplete.
	Ask(ctx context.Context, req Request, w io.Writer) (string, error)
}

// ErrIncomplete is wrapped by the error of an answer that stopped before
// its end.
var ErrIncomplete = errors.New("the answer stopped before its end")

// A Request is what's asked of a Provider.
type Request struct {
	// Model is the provider's model id, resolved from a tier.
	Model string `json:"model"`
	// System is the instructions, the host's first.
	System string `json:"system"`
	// Messages is the conversation, ending with the user's turn.
	Messages []Message `json:"messages"`
	// Effort is how hard to think, "" for the provider's default.
	Effort string `json:"effort,omitempty"`
	// Schema is the JSON schema of the answer, an object; nil for text.
	Schema map[string]any `json:"schema,omitempty"`
}

// A Message is a turn of a conversation.
type Message struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// The host's instructions come first in the system prompt, before the
// document's and the call's, for writing text into a page, and for an
// answer the program uses itself.
const (
	writeSystem = `You write a piece of text for a page of an interactive document: a program inserts your reply into the page, where it asked for it. Reply with that text only, in Markdown: paragraphs, emphasis and lists are fine, but no headings unless asked, and no preamble or closing remarks.

Never write links: the program gives the reader their choices itself.

The instructions that follow, if any, come from the document's author. They take precedence over these, except for the rule on links.`

	askSystem = `A program running an interactive document asks you something: a judgment, a decision or a short answer that it will use itself, rather than show as it is. Reply with exactly what it asks for, with no preamble or explanation.

The instructions that follow, if any, come from the document's author.`
)

// AI is the AI capability: register it in an engine's Capabilities as
// "ai". It's safe for concurrent use.
type AI struct {
	Provider Provider
	// Models maps tiers ("fast", "balanced", "smart") to the provider's
	// model ids; a model a document names that isn't a tier is passed on
	// as it is. Default is the tier, or model, of calls naming none.
	Models  map[string]string
	Default string

	mu      sync.Mutex
	answers map[[sha256.Size]byte]string
}

var _ whymark.Capability = (*AI)(nil)

// Name is "ai".
func (a *AI) Name() string { return "ai" }

// Module makes the ai module for a turn.
func (a *AI) Module(tc *whymark.TurnContext) (rt.Value, error) {
	m := rt.NewTable()
	meta := rt.NewTable()
	calls := 0 // this turn's, for recording answers by the page and call
	call := func(t *rt.Thread, spec *rt.Table, write bool) (rt.Value, error) {
		calls++
		return a.ask(tc, m, spec, write, calls)
	}
	ask := rt.NewGoFunction(func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		spec, err := c.TableArg(1)
		if err != nil {
			return nil, fmt.Errorf("ai{...}: %w", err)
		}
		v, err := call(t, spec, false)
		if err != nil {
			return nil, err
		}
		return c.PushingNext1(t.Runtime, v), nil
	}, "ai", 2, false)
	write := rt.NewGoFunction(func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		spec, err := c.TableArg(0)
		if err != nil {
			return nil, fmt.Errorf("ai.write{...}: %w", err)
		}
		v, err := call(t, spec, true)
		if err != nil {
			return nil, err
		}
		return c.PushingNext1(t.Runtime, v), nil
	}, "write", 1, false)
	block := rt.NewGoFunction(func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		text, err := c.StringArg(2)
		if err != nil {
			return nil, fmt.Errorf("ai block: %w", err)
		}
		spec := rt.NewTable()
		spec.Set(rt.StringValue("prompt"), rt.StringValue(text))
		if _, err := call(t, spec, true); err != nil {
			return nil, err
		}
		return c.Next(), nil
	}, "__block", 3, false)
	// They charge the turn for the answers they return, and reach no
	// files; asking the provider is the host's to allow (see
	// whymark.Capability).
	rt.SolemnlyDeclareCompliance(rt.ComplyCpuSafe|rt.ComplyMemSafe|rt.ComplyIoSafe, ask, write, block)
	meta.Set(rt.StringValue("__call"), rt.FunctionValue(ask))
	m.Set(rt.StringValue("write"), rt.FunctionValue(write))
	m.Set(rt.StringValue("__block"), rt.FunctionValue(block))
	m.SetMetatable(meta)
	return rt.TableValue(m), nil
}

// ask answers a call, spec, of the module m in the turn tc, the n-th of
// the turn, writing the answer into the page if write is set.
func (a *AI) ask(tc *whymark.TurnContext, m, spec *rt.Table, write bool, n int) (rt.Value, error) {
	name := "ai"
	if write {
		name = "ai.write"
	}
	req, wrapped, err := a.request(m, spec, write)
	if err != nil {
		return rt.NilValue, fmt.Errorf("%s{...}: %w", name, err)
	}
	userKey := field(spec, "key")
	key := answerKey(tc, req, userKey, n)

	var w io.Writer
	if write {
		w = writerFunc(tc.Write)
	}
	text, ok := a.recorded(key)
	if ok {
		if w != nil {
			if _, err := io.WriteString(w, text); err != nil {
				return rt.NilValue, err
			}
		}
	} else {
		text, err = a.Provider.Ask(tc.Context, req, w)
		switch {
		case errors.Is(err, ErrIncomplete) && write:
			// What there was stays on the page, unrecorded.
			if _, err := fmt.Fprintf(w, "\n\n*(%s.)*", err); err != nil {
				return rt.NilValue, err
			}
			tc.Runtime.RequireBytes(len(text))
			return rt.StringValue(text), nil
		case err != nil:
			return rt.NilValue, fmt.Errorf("%s{...}: %w", name, err)
		}
		a.record(key, text)
	}
	if req.Schema == nil {
		tc.Runtime.RequireBytes(len(text))
		return rt.StringValue(text), nil
	}
	var answer any
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return rt.NilValue, fmt.Errorf("ai{...}: the answer isn't the JSON asked for: %w", err)
	}
	if wrapped {
		answer = answer.(map[string]any)["value"]
	}
	return jsonToLua(tc.Runtime, answer), nil
}

// request makes the provider's request for a call, spec, of the module m.
// wrapped says the answer's shape is wrapped in an object, as providers
// require one, under "value".
func (a *AI) request(m, spec *rt.Table, write bool) (req Request, wrapped bool, err error) {
	req.System = askSystem
	if write {
		req.System = writeSystem
	}
	for _, extra := range []string{field(m, "system"), field(spec, "system")} {
		if strings.TrimSpace(extra) != "" {
			req.System += "\n\n" + extra
		}
	}
	model := field(spec, "model")
	if model == "" {
		model = field(m, "model")
	}
	if model == "" {
		model = a.Default
	}
	if resolved, ok := a.Models[model]; ok {
		model = resolved
	}
	req.Model, req.Effort = model, field(spec, "effort")

	if req.Messages, err = messages(spec.Get(rt.StringValue("messages"))); err != nil {
		return req, false, err
	}
	if prompt := strings.TrimSpace(field(spec, "prompt")); prompt != "" {
		req.Messages = append(req.Messages, Message{Role: "user", Content: prompt})
	}
	if len(req.Messages) == 0 {
		return req, false, errors.New("no prompt, and no messages")
	}
	if req.Messages[len(req.Messages)-1].Role != "user" {
		return req, false, errors.New("the conversation must end with the user's turn")
	}
	if returns := spec.Get(rt.StringValue("returns")); !returns.IsNil() {
		if write {
			return req, false, errors.New("no returns: it writes text")
		}
		if req.Schema, err = schema(returns, 0); err != nil {
			return req, false, fmt.Errorf("returns: %w", err)
		}
		if req.Schema["type"] != "object" {
			req.Schema, wrapped = objectSchema(map[string]any{"value": req.Schema}), true
		}
	}
	return req, wrapped, nil
}

// messages turns a call's messages into a conversation.
func messages(v rt.Value) ([]Message, error) {
	if v.IsNil() {
		return nil, nil
	}
	list, ok := v.TryTable()
	if !ok {
		return nil, errors.New("messages isn't a list")
	}
	var out []Message
	for i := int64(1); i <= list.Len(); i++ {
		turn, ok := list.Get(rt.IntValue(i)).TryTable()
		if !ok {
			return nil, fmt.Errorf("message %d isn't a table", i)
		}
		content := field(turn, "content")
		switch role, _ := turn.Get(rt.StringValue("role")).TryString(); role {
		case "user":
			out = append(out, Message{Role: "user", Content: content})
		case "ai":
			out = append(out, Message{Role: "assistant", Content: content})
		default:
			return nil, fmt.Errorf("message %d's role is %q: want user or ai", i, role)
		}
	}
	return out, nil
}

// field returns t's field name as a string, or "" if it's nil, or neither
// a string nor a number.
func field(t *rt.Table, name string) string {
	v := t.Get(rt.StringValue(name))
	if v.IsNil() {
		return ""
	}
	s, _ := v.ToString()
	return s
}

// answerKey is what an answer is recorded by: the document and the call,
// with the author's key if there's one, else the page and the call's
// place among the page's.
func answerKey(tc *whymark.TurnContext, req Request, userKey string, n int) [sha256.Size]byte {
	k := struct {
		Document, Key, Page string
		Call                int
		Request             Request
	}{Document: tc.Document.String(), Key: userKey, Request: req}
	if userKey == "" {
		k.Page, k.Call = tc.PageID, n
	}
	data, _ := json.Marshal(k)
	return sha256.Sum256(data)
}

func (a *AI) recorded(key [sha256.Size]byte) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	text, ok := a.answers[key]
	return text, ok
}

func (a *AI) record(key [sha256.Size]byte, text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.answers == nil {
		a.answers = map[[sha256.Size]byte]string{}
	}
	a.answers[key] = text
}

// writerFunc is an io.Writer of a function writing strings.
type writerFunc func(string) error

func (f writerFunc) Write(p []byte) (int, error) {
	if err := f(string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
