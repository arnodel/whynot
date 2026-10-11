package whymark

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/arnodel/golua/lib"
	"github.com/arnodel/golua/lib/base"
	"github.com/arnodel/golua/lib/coroutine"
	"github.com/arnodel/golua/lib/mathlib"
	"github.com/arnodel/golua/lib/packagelib"
	"github.com/arnodel/golua/lib/stringlib"
	"github.com/arnodel/golua/lib/tablelib"
	"github.com/arnodel/golua/lib/utf8lib"
	rt "github.com/arnodel/golua/runtime"

	"github.com/arnodel/whynot/browser/internal/whymark/compile"
)

const (
	// cpuLimit and memLimit bound what one turn uses, in golua's units:
	// about half a second of CPU, and 200MB.
	cpuLimit = 100_000_000
	memLimit = 200 << 20
	// maxDepth bounds how deeply tables nest in a state, which also
	// catches tables that contain themselves.
	maxDepth = 100
)

// turnLua runs before the document's chunk, to give the turn what's
// written in Lua.
const turnLua = `
local turn = ...

local function escape(s)
  return (tostring(s):gsub("[^%w%-_%.~]", function(c)
    return string.format("%%%02X", c:byte())
  end))
end

function turn.link(text, query)
  local keys = {}
  for k in pairs(query or {}) do keys[#keys + 1] = k end
  table.sort(keys, function(a, b) return tostring(a) < tostring(b) end)
  local parts = {}
  for _, k in ipairs(keys) do
    parts[#parts + 1] = escape(k) .. "=" .. escape(query[k])
  end
  local label = tostring(text):gsub("[%[%]]", "\\%0")
  return "[" .. label .. "](?" .. table.concat(parts, "&") .. ")"
end
`

// page is a turn's page as it's written: its header, sent once, at the
// script's request or before its first output, then its Markdown.
type page struct {
	w      io.Writer
	header Header
	// auto is whether the first output sends the header, as for a
	// document with passages; otherwise, writing first is an error.
	auto bool
	sent bool
	// heading is the passage's heading, written before its first output.
	heading string
	// program is the document's, for the header's default title.
	program *compile.Program
}

// send writes the header, for the turn's passage, with title if it's not
// "", else the default.
func (p *page) send(passage, title string) error {
	if p.sent {
		return errors.New("turn.write_headers called twice, or after the page started")
	}
	p.sent = true
	if passage != "" {
		location := *p.header.Location
		location.Fragment, location.RawFragment = passage, ""
		p.header.Location = &location
	}
	if title == "" {
		title = p.program.Title
		if ps, ok := p.program.Passage(passage); ok {
			title = ps.Title
		}
	}
	p.header.Title = title
	return writeHeader(p.w, p.header)
}

// write writes s, after the header and the passage's heading. Before the
// page has started, whitespace is dropped: it would be anyway, after the
// header, and a passage's blank lines mustn't start it, so that a lua
// block after them can still write the headers or go elsewhere.
func (p *page) write(passage, s string) error {
	if !p.sent && strings.TrimSpace(s) == "" {
		return nil
	}
	if !p.sent {
		if !p.auto {
			return errors.New("call turn.write_headers before turn.write")
		}
		if err := p.send(passage, ""); err != nil {
			return err
		}
	}
	if p.heading != "" {
		heading := p.heading
		p.heading = ""
		if _, err := io.WriteString(p.w, "# "+heading+"\n\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(p.w, s)
	return err
}

// turnInput is what a turn starts from.
type turnInput struct {
	// state is the trail's data; start says the turn starts the document,
	// from init(turn).
	state any
	start bool
	// passage is the passage to show, query and input the request's.
	passage      string
	query, input map[string]string
}

// run runs a turn of prog, in a fresh sandbox, writing its page to p. It
// returns the state and the passage the turn leaves.
func run(prog *compile.Program, p *page, in turnInput) (newState any, newPassage string, err error) {
	def := rt.RuntimeContextDef{
		HardLimits:    rt.RuntimeResources{Cpu: cpuLimit, Memory: memLimit},
		RequiredFlags: rt.ComplyIoSafe,
	}
	_, err = rt.DoInContext(func(r *rt.Runtime) error {
		// The other libraries register in package's table, but documents
		// have nothing to require yet.
		lib.LoadLibs(r, packagelib.LibLoader, base.LibLoader, coroutine.LibLoader,
			stringlib.LibLoader, tablelib.LibLoader, mathlib.LibLoader, utf8lib.LibLoader)
		env := r.GlobalEnv()
		r.SetEnv(env, "require", rt.NilValue)
		r.SetEnv(env, "package", rt.NilValue)

		turn := rt.NewTable()
		turnPassage := func() string {
			s, _ := turn.Get(rt.StringValue("passage")).TryString()
			return s
		}
		write := goFunction(r, turn, "write", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
			for _, arg := range c.Etc() {
				s, ok := arg.ToString()
				if !ok {
					return nil, fmt.Errorf("turn.write: can't write a %s", arg.TypeName())
				}
				if err := p.write(turnPassage(), s); err != nil {
					return nil, err
				}
			}
			return c.Next(), nil
		}, 0, true)
		writeHeaders := goFunction(r, turn, "write_headers", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
			title := ""
			if c.NArgs() > 0 {
				fields, err := c.TableArg(0)
				if err != nil {
					return nil, err
				}
				for k, v, _ := fields.Next(rt.NilValue); !k.IsNil(); k, v, _ = fields.Next(k) {
					switch name, _ := k.TryString(); name {
					case "title":
						s, ok := v.ToString()
						if !ok {
							return nil, fmt.Errorf("turn.write_headers: title can't be a %s", v.TypeName())
						}
						title = s
					case "location", "page-id":
						return nil, fmt.Errorf("turn.write_headers: %s is the engine's", name)
					default:
						return nil, fmt.Errorf("turn.write_headers: no header %v", k.Interface())
					}
				}
			}
			if err := p.send(turnPassage(), title); err != nil {
				return nil, err
			}
			return c.Next(), nil
		}, 1, false)

		host := rt.NewTable()
		started := goFunction(r, host, "started", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
			return c.PushingNext1(t.Runtime, rt.BoolValue(p.sent)), nil
		}, 0, false)
		heading := goFunction(r, host, "heading", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
			s, err := c.StringArg(0)
			if err != nil {
				return nil, err
			}
			p.heading = s
			return c.Next(), nil
		}, 1, false)
		// They only reach the turn's own page.
		rt.SolemnlyDeclareCompliance(rt.ComplyCpuSafe|rt.ComplyMemSafe|rt.ComplyIoSafe, write, writeHeaders, started, heading)

		turn.Set(rt.StringValue("state"), toLua(in.state))
		turn.Set(rt.StringValue("query"), stringTable(in.query))
		turn.Set(rt.StringValue("input"), stringTable(in.input))
		turn.Set(rt.StringValue("passage"), rt.StringValue(in.passage))

		thread := r.MainThread()
		turnChunk, err := r.CompileAndLoadLuaChunk("turn", []byte(turnLua), rt.TableValue(env))
		if err != nil {
			return err
		}
		if _, err := rt.Call1(thread, rt.FunctionValue(turnChunk), rt.TableValue(turn)); err != nil {
			return err
		}
		chunk := r.LoadLuaUnit(prog.Unit, rt.TableValue(env))
		if _, err := rt.Call1(thread, rt.FunctionValue(chunk), rt.TableValue(turn), rt.TableValue(host)); err != nil {
			return err
		}

		if in.start {
			fresh := rt.TableValue(rt.NewTable())
			if init := env.Get(rt.StringValue("init")); !init.IsNil() {
				v, err := rt.Call1(thread, init, rt.TableValue(turn))
				if err != nil {
					return err
				}
				if !v.IsNil() {
					fresh = v
				}
			}
			turn.Set(rt.StringValue("state"), fresh)
		}
		pageFn := env.Get(rt.StringValue("page"))
		if pageFn.IsNil() {
			return errors.New("the document has no passages, and defines no page(turn)")
		}
		if _, err := rt.Call1(thread, pageFn, rt.TableValue(turn)); err != nil {
			return err
		}
		newState, err = fromLua(turn.Get(rt.StringValue("state")), 0)
		if err != nil {
			return fmt.Errorf("turn.state: %w", err)
		}
		newPassage = turnPassage()
		return nil
	}, def, io.Discard)
	return newState, newPassage, err
}

// goFunction sets t's field name to a Go function.
func goFunction(r *rt.Runtime, t *rt.Table, name string, f func(*rt.Thread, *rt.GoCont) (rt.Cont, error), nArgs int, variadic bool) *rt.GoFunction {
	fn := rt.NewGoFunction(f, name, nArgs, variadic)
	t.Set(rt.StringValue(name), rt.FunctionValue(fn))
	return fn
}

// stringTable makes a table of m.
func stringTable(m map[string]string) rt.Value {
	t := rt.NewTable()
	for k, v := range m {
		t.Set(rt.StringValue(k), rt.StringValue(v))
	}
	return rt.TableValue(t)
}

// table is a Lua table copied out of a runtime by fromLua.
type table struct {
	keys, values []any
}

// fromLua copies v out of its runtime as plain data: nil, a bool, an
// int64, a float64, a string, or a *table of those. Anything else, such
// as a function, is an error.
func fromLua(v rt.Value, depth int) (any, error) {
	if depth > maxDepth {
		return nil, errors.New("tables nested too deeply, or containing themselves")
	}
	switch v.Type() {
	case rt.NilType:
		return nil, nil
	case rt.BoolType, rt.IntType, rt.FloatType, rt.StringType:
		return v.Interface(), nil
	case rt.TableType:
		t := v.AsTable()
		data := &table{}
		for k, val, _ := t.Next(rt.NilValue); !k.IsNil(); k, val, _ = t.Next(k) {
			key, err := fromLua(k, depth+1)
			if err != nil {
				return nil, err
			}
			if _, ok := key.(*table); ok {
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
	case *table:
		t := rt.NewTable()
		for i, k := range d.keys {
			t.Set(toLua(k), toLua(d.values[i]))
		}
		return rt.TableValue(t)
	}
	return rt.NilValue
}
