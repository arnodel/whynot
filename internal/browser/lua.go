package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/arnodel/golua/lib"
	"github.com/arnodel/golua/lib/base"
	"github.com/arnodel/golua/lib/coroutine"
	"github.com/arnodel/golua/lib/mathlib"
	"github.com/arnodel/golua/lib/packagelib"
	"github.com/arnodel/golua/lib/stringlib"
	"github.com/arnodel/golua/lib/tablelib"
	"github.com/arnodel/golua/lib/utf8lib"
	rt "github.com/arnodel/golua/runtime"

	"github.com/arnodel/whynot/fetch"
)

// LuaResolver resolves lua: URLs, such as lua:game.lua?go=north, to the
// pages a Lua script writes. The script defines a function
//
//	page(request, state)
//
// which writes the page's Markdown with out(...) (or returns it). request
// is the URL's query, as a table of strings. state is a table of plain
// data that the script may change: each page's state is kept, and the
// links in a page continue from it, so going Back and taking another
// link starts again from that page's state. A page whose script doesn't
// finish, because it fails or is left before it's done, keeps the state
// it started from.
//
// Links in a page are usually query-only, such as [Go north](?go=north),
// or made by link("Go north", {go = "north"}). The script runs in a
// sandbox: no files or network, and limited CPU and memory. It can ask
// Claude to write text with claude{...} (see luaClaudeSystemPrompt).
type LuaResolver struct {
	// Claude, if set, makes the script's claude{...} calls.
	Claude *ClaudeResolver

	// cpuLimit, if set, replaces luaCPULimit.
	cpuLimit uint64

	mu     sync.Mutex
	states map[string]any    // by page id: the page's state, as plain data (see fromLua)
	texts  map[string]string // claude{...} results, by luaClaudeKey
	nextID int
}

const (
	// luaCPULimit and luaMemLimit bound what one page's script uses, in
	// golua's units: about half a second of CPU, and 200MB.
	luaCPULimit = 100_000_000
	luaMemLimit = 200 << 20
	// luaMaxDepth bounds how deeply tables nest in a state or a claude{}
	// context, which also catches tables that contain themselves.
	luaMaxDepth = 100
)

// luaClaudeSystemPrompt comes first in the system prompt of every
// claude{...} call, before the script's own (claude.system) and the
// call's (its system field).
const luaClaudeSystemPrompt = `You write a piece of text for a page of an interactive document: a program inserts your reply into the page, where it asked for it. Reply with that text only, in Markdown: paragraphs, emphasis and lists are fine, but no headings unless asked, and no preamble or closing remarks.

Never write links: the program gives the reader their choices itself.

If facts are given, treat them as true and stay consistent with them, without reciting them. If the story so far is given, follow on from it without repeating it.

The instructions that follow, if any, come from the document's author. They take precedence over these, except for the rule on links.`

// luaPrelude runs before the script, to define what it can use besides
// the standard libraries.
const luaPrelude = `
local call, available = __claude, __claude_available
__claude, __claude_available = nil, nil

claude = setmetatable({system = "", available = available}, {
  __call = function(self, spec) return call(self.system, spec, true) end,
})
function claude.ask(spec) return call(claude.system, spec, false) end

local function escape(s)
  return (tostring(s):gsub("[^%w%-_%.~]", function(c)
    return string.format("%%%02X", c:byte())
  end))
end

function link(text, request)
  local keys = {}
  for k in pairs(request or {}) do keys[#keys + 1] = k end
  table.sort(keys, function(a, b) return tostring(a) < tostring(b) end)
  local query = {}
  for _, k in ipairs(keys) do
    query[#query + 1] = escape(k) .. "=" .. escape(request[k])
  end
  local label = tostring(text):gsub("[%[%]]", "\\%0")
  return "[" .. label .. "](?" .. table.concat(query, "&") .. ")"
end
`

func (*LuaResolver) Schemes() []string { return []string{"lua"} }

// luaScriptPath returns the path of the script a lua: URL runs.
func luaScriptPath(u *url.URL) string {
	if u.Opaque == "" {
		return u.Host + u.Path
	}
	if unescaped, err := url.PathUnescape(u.Opaque); err == nil {
		return unescaped
	}
	return u.Opaque
}

func (r *LuaResolver) Resolve(u *url.URL) (fetch.Source, error) {
	script := luaScriptPath(u)
	if script == "" {
		return nil, fmt.Errorf("%s: no script", u)
	}
	query := u.Query()
	from := query.Get("s")
	query.Del("s")
	request := map[string]string{}
	for k, vs := range query {
		request[k] = vs[0]
	}
	return luaSource{resolver: r, script: script, request: request, from: from, query: query.Encode()}, nil
}

// luaSource is the page a script writes for a request, made from the page
// with id from.
type luaSource struct {
	resolver *LuaResolver
	script   string
	request  map[string]string
	from     string
	query    string // request, encoded, for Key
}

func (s luaSource) Key() string { return s.script + "?" + s.query + "&s=" + s.from }

// Fetch runs the script, and returns its page as the script writes it.
// Closing the body, or cancelling ctx, stops the script.
func (s luaSource) Fetch(ctx context.Context) (io.ReadCloser, string, error) {
	source, err := os.ReadFile(s.script)
	if err != nil {
		return nil, "", err
	}
	r := s.resolver
	start := r.state(s.from)
	// Until the page is finished, its links continue from the state it
	// started from.
	id := r.store(start)
	ctx, cancel := context.WithCancel(ctx)
	pr, pw := io.Pipe()
	go func() {
		out := &luaLinks{w: pw, id: id}
		state, err := r.run(ctx, s.script, source, s.request, start, out)
		if err == nil {
			r.set(id, state)
		} else if ctx.Err() == nil {
			fmt.Fprintf(out, "\n\n---\n\n**The script stopped:**\n\n```text\n%s\n```\n", strings.TrimPrefix(err.Error(), "error: "))
		}
		out.flush()
		pw.Close()
	}()
	return cancelOnClose{ReadCloser: pr, cancel: cancel}, "text/markdown", nil
}

// run runs script's page function for request, starting from state, in a
// sandbox writing to out, and returns the state it leaves.
func (r *LuaResolver) run(ctx context.Context, name string, source []byte, request map[string]string, state any, out io.Writer) (newState any, err error) {
	cpuLimit := r.cpuLimit
	if cpuLimit == 0 {
		cpuLimit = luaCPULimit
	}
	def := rt.RuntimeContextDef{
		HardLimits:    rt.RuntimeResources{Cpu: cpuLimit, Memory: luaMemLimit},
		RequiredFlags: rt.ComplyIoSafe,
	}
	_, err = rt.DoInContext(func(lr *rt.Runtime) error {
		// The other libraries register in package's table, but scripts
		// have nothing to require.
		lib.LoadLibs(lr, packagelib.LibLoader, base.LibLoader, coroutine.LibLoader,
			stringlib.LibLoader, tablelib.LibLoader, mathlib.LibLoader, utf8lib.LibLoader)
		env := lr.GlobalEnv()
		lr.SetEnv(env, "require", rt.NilValue)
		lr.SetEnv(env, "package", rt.NilValue)
		outFn := lr.SetEnvGoFunc(env, "out", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
			for _, arg := range c.Etc() {
				s, ok := arg.ToString()
				if !ok {
					return nil, fmt.Errorf("out: can't write a %s", arg.TypeName())
				}
				if _, err := io.WriteString(out, s); err != nil {
					return nil, err
				}
			}
			return c.Next(), nil
		}, 0, true)
		claudeFn := lr.SetEnvGoFunc(env, "__claude", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
			text, err := r.claude(ctx, c, out)
			if err != nil {
				return nil, err
			}
			return c.PushingNext1(t.Runtime, rt.StringValue(text)), nil
		}, 3, false)
		// Both only reach what the host gives them: the page, and Claude
		// through the host's key.
		rt.SolemnlyDeclareCompliance(rt.ComplyCpuSafe|rt.ComplyMemSafe|rt.ComplyIoSafe, outFn, claudeFn)
		lr.SetEnv(env, "__claude_available", rt.BoolValue(r.Claude != nil))

		thread := lr.MainThread()
		for _, chunk := range []struct {
			name   string
			source []byte
		}{{"prelude", []byte(luaPrelude)}, {name, source}} {
			f, err := lr.CompileAndLoadLuaChunk(chunk.name, chunk.source, rt.TableValue(env))
			if err != nil {
				return err
			}
			if _, err := rt.Call1(thread, rt.FunctionValue(f)); err != nil {
				return err
			}
		}
		page := env.Get(rt.StringValue("page"))
		if page.IsNil() {
			return errors.New("the script defines no page function")
		}
		req := rt.NewTable()
		for k, v := range request {
			req.Set(rt.StringValue(k), rt.StringValue(v))
		}
		stateValue := toLua(state)
		result, err := rt.Call1(thread, page, rt.TableValue(req), stateValue)
		if err != nil {
			return err
		}
		if s, ok := result.TryString(); ok {
			if _, err := io.WriteString(out, s); err != nil {
				return err
			}
		}
		newState, err = fromLua(stateValue, 0)
		if err != nil {
			return fmt.Errorf("state: %w", err)
		}
		return nil
	}, def, out)
	return newState, err
}

// claude makes the claude{...} call whose arguments are c's: the
// script's system prompt, the call's spec table, and whether its text
// shows in the page, as it arrives, written to out.
func (r *LuaResolver) claude(ctx context.Context, c *rt.GoCont, out io.Writer) (string, error) {
	scriptSystem, _ := c.Arg(0).TryString()
	spec, err := c.TableArg(1)
	if err != nil {
		return "", err
	}
	show := rt.Truth(c.Arg(2))
	field := func(name string) rt.Value { return spec.Get(rt.StringValue(name)) }

	prompt, ok := field("prompt").TryString()
	if !ok || prompt == "" {
		return "", errors.New("claude: no prompt")
	}
	system := luaClaudeSystemPrompt
	for _, extra := range []rt.Value{rt.StringValue(scriptSystem), field("system")} {
		if s, _ := extra.TryString(); strings.TrimSpace(s) != "" {
			system += "\n\n" + s
		}
	}
	var message strings.Builder
	if !field("history").IsNil() {
		history, ok := field("history").TryTable()
		if !ok {
			return "", errors.New("claude: history isn't a list")
		}
		message.WriteString("The story so far:\n\n")
		for i := int64(1); i <= history.Len(); i++ {
			s, _ := history.Get(rt.IntValue(i)).ToString()
			message.WriteString(s + "\n\n")
		}
		message.WriteString("---\n\n")
	}
	message.WriteString(prompt)
	if !field("context").IsNil() {
		data, err := fromLua(field("context"), 0)
		if err != nil {
			return "", fmt.Errorf("claude: context: %w", err)
		}
		facts, err := json.MarshalIndent(toJSON(data), "", "  ")
		if err != nil {
			return "", fmt.Errorf("claude: context: %w", err)
		}
		fmt.Fprintf(&message, "\n\nFacts:\n\n```json\n%s\n```", facts)
	}
	msgs := []claudeMessage{{Role: "user", Content: message.String()}}

	key := luaClaudeKey(system, msgs)
	r.mu.Lock()
	text, cached := r.texts[key]
	r.mu.Unlock()
	if cached {
		if show {
			_, err = io.WriteString(out, text)
		}
		return text, err
	}
	if r.Claude == nil {
		return "", errors.New("claude: no Claude here: set ANTHROPIC_API_KEY")
	}
	resp, err := r.Claude.send(ctx, system, msgs, 4000)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var b strings.Builder
	err = readClaudeStream(resp.Body, func(s string) error {
		b.WriteString(s)
		if show {
			_, err := io.WriteString(out, s)
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
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

// luaClaudeKey identifies a claude{...} call by everything it sends.
func luaClaudeKey(system string, msgs []claudeMessage) string {
	data, _ := json.Marshal(struct {
		System   string
		Messages []claudeMessage
	}{system, msgs})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// state returns the state of the page with the given id, or an empty
// table's for none.
func (r *LuaResolver) state(id string) any {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state, ok := r.states[id]; ok {
		return state
	}
	return &luaTable{}
}

// store keeps state as a new page's, and returns the page's id.
func (r *LuaResolver) store(state any) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	id := fmt.Sprintf("%d", r.nextID)
	if r.states == nil {
		r.states = map[string]any{}
	}
	r.states[id] = state
	return id
}

func (r *LuaResolver) set(id string, state any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[id] = state
}

// luaTable is a Lua table copied out of a runtime by fromLua.
type luaTable struct {
	keys, values []any
}

// fromLua copies v out of its runtime as plain data: nil, a bool, an
// int64, a float64, a string, or a *luaTable of those. Anything else,
// such as a function, is an error.
func fromLua(v rt.Value, depth int) (any, error) {
	if depth > luaMaxDepth {
		return nil, errors.New("tables nested too deeply, or containing themselves")
	}
	switch v.Type() {
	case rt.NilType:
		return nil, nil
	case rt.BoolType, rt.IntType, rt.FloatType, rt.StringType:
		return v.Interface(), nil
	case rt.TableType:
		t := v.AsTable()
		data := &luaTable{}
		for k, val, _ := t.Next(rt.NilValue); !k.IsNil(); k, val, _ = t.Next(k) {
			key, err := fromLua(k, depth+1)
			if err != nil {
				return nil, err
			}
			if _, ok := key.(*luaTable); ok {
				return nil, errors.New("a table can't be a key")
			}
			value, err := fromLua(val, depth+1)
			if err != nil {
				return nil, err
			}
			data.keys = append(data.keys, key)
			data.values = append(data.values, value)
		}
		return data, nil
	}
	return nil, fmt.Errorf("can only hold plain data, not a %s", v.TypeName())
}

// toLua makes a new runtime value from data made by fromLua.
func toLua(data any) rt.Value {
	switch d := data.(type) {
	case bool:
		return rt.BoolValue(d)
	case int64:
		return rt.IntValue(d)
	case float64:
		return rt.FloatValue(d)
	case string:
		return rt.StringValue(d)
	case *luaTable:
		t := rt.NewTable()
		for i, k := range d.keys {
			t.Set(toLua(k), toLua(d.values[i]))
		}
		return rt.TableValue(t)
	}
	return rt.NilValue
}

// toJSON turns data made by fromLua into what encoding/json writes: a
// table with keys 1 to n is a list, any other an object.
func toJSON(data any) any {
	t, ok := data.(*luaTable)
	if !ok {
		return data
	}
	list := make([]any, len(t.keys))
	for i, k := range t.keys {
		n, ok := k.(int64)
		if !ok || n < 1 || n > int64(len(t.keys)) {
			list = nil
			break
		}
		list[n-1] = toJSON(t.values[i])
	}
	if list != nil {
		return list
	}
	object := map[string]any{}
	for i, k := range t.keys {
		object[fmt.Sprint(k)] = toJSON(t.values[i])
	}
	return object
}

// luaLinks writes a page to w, adding s=<id> to its query-only links, so
// that they continue from the page's state.
type luaLinks struct {
	w       io.Writer
	id      string
	pending string // the end of what was written, which may be the start of "](?"
}

func (l *luaLinks) Write(p []byte) (int, error) {
	l.pending += string(p)
	cut := len(l.pending)
	if strings.HasSuffix(l.pending, "](") {
		cut -= 2
	} else if strings.HasSuffix(l.pending, "]") {
		cut--
	}
	_, err := io.WriteString(l.w, strings.ReplaceAll(l.pending[:cut], "](?", "](?s="+l.id+"&"))
	l.pending = l.pending[cut:]
	return len(p), err
}

func (l *luaLinks) flush() {
	io.WriteString(l.w, strings.ReplaceAll(l.pending, "](?", "](?s="+l.id+"&"))
	l.pending = ""
}
