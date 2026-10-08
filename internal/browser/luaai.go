package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	rt "github.com/arnodel/golua/runtime"
)

// A lua: script asks its AI with
//
//	ai{prompt = ..., messages = ..., context = ..., history = ...,
//	   system = ..., model = ..., effort = ..., returns = ...}
//
// which returns the reply, and ai.write{...}, which also writes it into
// the page as it arrives. ai.system and ai.model are the script's
// defaults. messages is a conversation so far, a list of {role = "user"
// or "ai", content = ...}, sent as turns: prompt, context and history,
// if any, make one more turn, the reader's, and the conversation must
// end with one. Without returns the reply is text; with it, the reply follows
// its shape and is a Lua value: "string", "number", "integer" or
// "boolean" for a value of that type, {shape} for a list of shape, and a
// table of shapes for an object with those fields.

// luaAIWritePrompt comes first in the system prompt of ai.write{...},
// and luaAIAskPrompt in ai{...}'s, before the script's own (ai.system)
// and the call's (its system field).
const (
	luaAIWritePrompt = `You write a piece of text for a page of an interactive document: a program inserts your reply into the page, where it asked for it. Reply with that text only, in Markdown: paragraphs, emphasis and lists are fine, but no headings unless asked, and no preamble or closing remarks.

Never write links: the program gives the reader their choices itself.

If facts are given, treat them as true and stay consistent with them, without reciting them. If the story so far is given, follow on from it without repeating it.

The instructions that follow, if any, come from the document's author. They take precedence over these, except for the rule on links.`

	luaAIAskPrompt = `A program running an interactive document asks you something: a judgment, a decision or a short answer that it will use itself, rather than show as it is. Reply with exactly what it asks for, with no preamble or explanation.

If facts are given, treat them as true. If the story so far is given, take it into account.

The instructions that follow, if any, come from the document's author.`
)

// ai makes the ai{...} or ai.write{...} call whose arguments are c's: the
// ai table, with the script's defaults, the call's spec, and whether to
// write the reply to out as it arrives.
func (r *LuaResolver) ai(ctx context.Context, c *rt.GoCont, out io.Writer) (rt.Value, error) {
	defaults, _ := c.Arg(0).TryTable()
	spec, err := c.TableArg(1)
	if err != nil {
		return rt.NilValue, err
	}
	write := rt.Truth(c.Arg(2))
	field := func(t *rt.Table, name string) string {
		if t == nil {
			return ""
		}
		s, _ := t.Get(rt.StringValue(name)).TryString()
		return s
	}
	get := func(name string) rt.Value { return spec.Get(rt.StringValue(name)) }

	prompt := field(spec, "prompt")
	turns, err := luaTurns(get("messages"))
	if err != nil {
		return rt.NilValue, err
	}
	if prompt == "" && len(turns) == 0 {
		return rt.NilValue, errors.New("ai: no prompt, and no messages")
	}
	req := AIRequest{Model: field(spec, "model"), Effort: field(spec, "effort")}
	if req.Model == "" {
		req.Model = field(defaults, "model")
	}
	req.System = luaAIAskPrompt
	if write {
		req.System = luaAIWritePrompt
	}
	for _, extra := range []string{field(defaults, "system"), field(spec, "system")} {
		if strings.TrimSpace(extra) != "" {
			req.System += "\n\n" + extra
		}
	}

	var message strings.Builder
	if history := get("history"); !history.IsNil() {
		list, ok := history.TryTable()
		if !ok {
			return rt.NilValue, errors.New("ai: history isn't a list")
		}
		message.WriteString("The story so far:\n\n")
		for i := int64(1); i <= list.Len(); i++ {
			s, _ := list.Get(rt.IntValue(i)).ToString()
			message.WriteString(s + "\n\n")
		}
		message.WriteString("---\n\n")
	}
	message.WriteString(prompt)
	if facts := get("context"); !facts.IsNil() {
		data, err := fromLua(facts, 0)
		if err != nil {
			return rt.NilValue, fmt.Errorf("ai: context: %w", err)
		}
		encoded, err := json.MarshalIndent(toJSON(data), "", "  ")
		if err != nil {
			return rt.NilValue, fmt.Errorf("ai: context: %w", err)
		}
		fmt.Fprintf(&message, "\n\nFacts:\n\n```json\n%s\n```", encoded)
	}
	req.Messages = turns
	if last := strings.TrimSpace(message.String()); last != "" {
		req.Messages = append(req.Messages, AIMessage{Role: "user", Content: last})
	}
	if req.Messages[len(req.Messages)-1].Role != "user" {
		return rt.NilValue, errors.New("ai: the conversation must end with the user's turn")
	}

	// A shape that isn't an object is wrapped in one, which the API
	// requires.
	wrapped := false
	if returns := get("returns"); !returns.IsNil() {
		if write {
			return rt.NilValue, errors.New("ai.write: no returns: it writes text")
		}
		if req.Schema, err = luaSchema(returns, 0); err != nil {
			return rt.NilValue, fmt.Errorf("ai: returns: %w", err)
		}
		if req.Schema["type"] != "object" {
			req.Schema, wrapped = objectSchema(map[string]any{"value": req.Schema}), true
		}
	}

	text, err := r.reply(ctx, req, write, out)
	if err != nil {
		return rt.NilValue, err
	}
	if req.Schema == nil {
		return rt.StringValue(text), nil
	}
	var reply any
	if err := json.Unmarshal([]byte(text), &reply); err != nil {
		return rt.NilValue, fmt.Errorf("ai: the reply isn't the JSON asked for: %w", err)
	}
	if wrapped {
		reply = reply.(map[string]any)["value"]
	}
	return jsonToLua(reply), nil
}

// reply returns the AI's reply to req, remembered by aiKey, writing it to
// out as it arrives if write is set. A write cut off or declined writes a
// note instead of failing.
func (r *LuaResolver) reply(ctx context.Context, req AIRequest, write bool, out io.Writer) (string, error) {
	key := aiKey(req)
	r.mu.Lock()
	text, cached := r.texts[key]
	r.mu.Unlock()
	if cached {
		if write {
			if _, err := io.WriteString(out, text); err != nil {
				return "", err
			}
		}
		return text, nil
	}

	body, err := r.AI.Start(ctx, req)
	if err != nil {
		return "", err
	}
	defer body.Close()
	var b strings.Builder
	err = readText(body, func(s string) error {
		b.WriteString(s)
		if write {
			_, err := io.WriteString(out, s)
			return err
		}
		return nil
	})
	if err != nil {
		if write && (errors.Is(err, ErrAICutOff) || errors.Is(err, ErrAIDeclined)) {
			_, err := fmt.Fprintf(out, "\n\n*(%s.)*", err)
			return b.String(), err
		}
		return "", fmt.Errorf("ai: %w", err)
	}
	text = b.String()
	r.mu.Lock()
	if r.texts == nil {
		r.texts = map[string]string{}
	}
	r.texts[key] = text
	r.mu.Unlock()
	return text, nil
}

// luaTurns turns an ai{...} call's messages into turns: a list of
// {role = "user" or "ai", content = ...}.
func luaTurns(messages rt.Value) ([]AIMessage, error) {
	if messages.IsNil() {
		return nil, nil
	}
	list, ok := messages.TryTable()
	if !ok {
		return nil, errors.New("ai: messages isn't a list")
	}
	var turns []AIMessage
	for i := int64(1); i <= list.Len(); i++ {
		turn, ok := list.Get(rt.IntValue(i)).TryTable()
		if !ok {
			return nil, fmt.Errorf("ai: message %d isn't a table", i)
		}
		content, _ := turn.Get(rt.StringValue("content")).ToString()
		switch role, _ := turn.Get(rt.StringValue("role")).TryString(); role {
		case "user":
			turns = append(turns, AIMessage{Role: "user", Content: content})
		case "ai":
			turns = append(turns, AIMessage{Role: "assistant", Content: content})
		default:
			return nil, fmt.Errorf("ai: message %d's role is %q: want user or ai", i, role)
		}
	}
	return turns, nil
}

// aiKey identifies a request by everything it sends.
func aiKey(req AIRequest) string {
	data, _ := json.Marshal(req)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// luaSchema turns the shape of an ai{...} call's returns into a JSON
// schema.
func luaSchema(shape rt.Value, depth int) (map[string]any, error) {
	if depth > luaMaxDepth {
		return nil, errors.New("shapes nested too deeply, or containing themselves")
	}
	if s, ok := shape.TryString(); ok {
		switch s {
		case "string", "number", "integer", "boolean":
			return map[string]any{"type": s}, nil
		}
		return nil, fmt.Errorf("unknown type %q: want string, number, integer or boolean", s)
	}
	t, ok := shape.TryTable()
	if !ok {
		return nil, fmt.Errorf("a shape is a type name or a table, not a %s", shape.TypeName())
	}
	if first := t.Get(rt.IntValue(1)); !first.IsNil() {
		if k, _, _ := t.Next(rt.IntValue(1)); !k.IsNil() || t.Len() != 1 {
			return nil, errors.New("a list's shape is a table with just one shape, for its items")
		}
		items, err := luaSchema(first, depth+1)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	}
	properties := map[string]any{}
	for k, v, _ := t.Next(rt.NilValue); !k.IsNil(); k, v, _ = t.Next(k) {
		name, ok := k.TryString()
		if !ok {
			return nil, errors.New("an object's fields are named by strings")
		}
		schema, err := luaSchema(v, depth+1)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		properties[name] = schema
	}
	if len(properties) == 0 {
		return nil, errors.New("an object needs at least one field")
	}
	return objectSchema(properties), nil
}

// objectSchema returns the schema of an object with all of properties.
func objectSchema(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	sort.Strings(required)
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

// jsonToLua turns a decoded JSON value into a Lua value: objects and
// arrays into tables, and whole numbers into integers.
func jsonToLua(v any) rt.Value {
	switch v := v.(type) {
	case string:
		return rt.StringValue(v)
	case bool:
		return rt.BoolValue(v)
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
			return rt.IntValue(int64(v))
		}
		return rt.FloatValue(v)
	case []any:
		t := rt.NewTable()
		for i, item := range v {
			t.Set(rt.IntValue(int64(i+1)), jsonToLua(item))
		}
		return rt.TableValue(t)
	case map[string]any:
		t := rt.NewTable()
		for k, item := range v {
			t.Set(rt.StringValue(k), jsonToLua(item))
		}
		return rt.TableValue(t)
	}
	return rt.NilValue
}
