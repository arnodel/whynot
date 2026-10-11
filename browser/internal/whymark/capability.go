package whymark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/arnodel/golua/code"
	rt "github.com/arnodel/golua/runtime"

	"github.com/arnodel/whynot/browser/internal/whymark/compile"
)

// A Capability is something a document can't do by itself, such as asking
// an AI, which an engine can provide to the documents declaring it in
// their front matter (whymark: uses:), and the reader allowing it (see
// Engine.Grant). A document reaches it as a Lua module, with require.
type Capability interface {
	// Module makes the capability's Lua module for one turn. It's made
	// the first time the turn requires it.
	Module(t *TurnContext) (rt.Value, error)
}

// TurnContext is what a capability's module knows of the turn it's made
// for.
type TurnContext struct {
	// Context is cancelled when the turn's page is closed.
	Context context.Context
	// Runtime is the turn's, which the module's values belong to.
	Runtime *rt.Runtime
	// Document is the document's URL, with no query or fragment.
	Document *url.URL
	// PageID is the id of the page the turn makes.
	PageID string
	// Request is the turn's request.
	Request Request
	// Write writes text to the page, after its header (see
	// turn.write_headers): what the page shows of the module's work.
	Write func(string) error
}

// capabilities works out what a turn of a document gets of the
// capabilities it declares: those the engine has and the reader allows,
// by name. It's an error, which the turn's page shows, if a capability the
// document requires isn't among them.
func (e *Engine) capabilities(document *url.URL, uses map[string]compile.Need) (map[string]Capability, error) {
	granted := map[string]Capability{}
	for name, need := range uses {
		c := e.Capabilities[name]
		switch {
		case c != nil && (e.Grant == nil || e.Grant(document, name)):
			granted[name] = c
		case need != compile.Required:
		case c == nil:
			return nil, fmt.Errorf("this document needs %q, which isn't available here", name)
		default:
			return nil, fmt.Errorf("this document needs %q, which it isn't allowed to use", name)
		}
	}
	return granted, nil
}

// requireFunc makes the turn's require. require(name) returns, in order of
// precedence: what it returned for name earlier in the turn; for a
// capability the document declares, its module, or false if the turn
// doesn't get it (see Engine.capabilities); or the document's Lua module
// name, the file name.lua next to the document, fetched through the
// engine's registry and run in the turn's sandbox.
func (e *Engine) requireFunc(r *rt.Runtime, env *rt.Table, tc *TurnContext, uses map[string]compile.Need, granted map[string]Capability) *rt.GoFunction {
	loaded := map[string]rt.Value{}
	fn := rt.NewGoFunction(func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		name, err := c.StringArg(0)
		if err != nil {
			return nil, err
		}
		if v, ok := loaded[name]; ok {
			return c.PushingNext1(t.Runtime, v), nil
		}
		var v rt.Value
		if _, declared := uses[name]; declared {
			v = rt.BoolValue(false)
			if capability := granted[name]; capability != nil {
				if v, err = capability.Module(tc); err != nil {
					return nil, fmt.Errorf("require %q: %w", name, err)
				}
			}
		} else {
			if v, err = e.module(t, env, tc, name); err != nil {
				return nil, fmt.Errorf("require %q: %w", name, err)
			}
		}
		loaded[name] = v
		return c.PushingNext1(t.Runtime, v), nil
	}, "require", 1, false)
	// It reaches only declared capabilities, and documents through the
	// engine's registry.
	rt.SolemnlyDeclareCompliance(rt.ComplyCpuSafe|rt.ComplyMemSafe|rt.ComplyIoSafe, fn)
	return fn
}

// module runs the document's Lua module name, and returns what it returns,
// or true if that's nil, as Lua's require does.
func (e *Engine) module(t *rt.Thread, env *rt.Table, tc *TurnContext, name string) (rt.Value, error) {
	if name == "" || strings.ContainsAny(name, ":\\") || strings.HasPrefix(name, "/") {
		return rt.NilValue, errors.New("a module is named by a path relative to the document, such as lib/story")
	}
	location := tc.Document.ResolveReference(&url.URL{Path: name + ".lua"})
	unit, err := e.moduleUnit(tc.Context, location)
	if err != nil {
		return rt.NilValue, err
	}
	chunk := t.Runtime.LoadLuaUnit(unit, rt.TableValue(env))
	v, err := rt.Call1(t, rt.FunctionValue(chunk), rt.StringValue(name))
	if err != nil {
		return rt.NilValue, err
	}
	if v.IsNil() {
		v = rt.BoolValue(true)
	}
	return v, nil
}

// moduleUnit returns the Lua module at location compiled, from the cache
// if its content has been seen.
func (e *Engine) moduleUnit(ctx context.Context, location *url.URL) (*code.Unit, error) {
	src, err := e.Registry.Resolve(location, "")
	if err != nil {
		return nil, err
	}
	body, _, err := src.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	source, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	return e.compileModule(location.String(), source)
}
