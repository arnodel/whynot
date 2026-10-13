package whymark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/arnodel/golua/code"
	rt "github.com/arnodel/golua/runtime"
	"github.com/arnodel/golua/safeio"

	"github.com/arnodel/whynot/browser/internal/whymark/compile"
)

// A Capability is something a document can't do by itself, such as asking
// an AI, which an engine can provide to the documents declaring it in
// their front matter (whymark: uses:), and the reader allowing it (see
// WithGrant). A document reaches it as a Lua module, with require, and
// through blocks named after it (see the compile package): the module's
// field __block, a function, if set, gives them their meaning, called as
// __block(turn, args, text), where args are the words after the name in
// the block's info string, and text its lines, code spans run.
type Capability interface {
	// Name is what documents declare, and require: "ai", say.
	Name() string
	// Module makes the capability's Lua module for one turn. It's made
	// the first time the turn requires it.
	//
	// The turn runs with CPU and memory limits, and requires IO safety,
	// so the module's Go functions run only if declared CPU, memory and
	// IO safe (see rt.SolemnlyDeclareCompliance). They must then charge
	// the turn's runtime for what they make, and reach files only through
	// golua's safeio, which allows only what the turn may read. Other
	// effects, such as asking an AI over the network, are the host's to
	// allow, which it does by giving a document the capability.
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

// granted works out what a turn of a document gets of the
// capabilities it declares: those the engine has and the reader allows,
// by name. It's an error, which the turn's page shows, if a capability the
// document requires isn't among them.
func (e *Engine) granted(document *url.URL, uses map[string]compile.Need) (map[string]Capability, error) {
	granted := map[string]Capability{}
	for name, need := range uses {
		c := e.capabilities[name]
		switch {
		case c != nil && (e.grant == nil || e.grant(document, name)):
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

// requireFuncs makes the turn's require, and __host.capability, which
// capability blocks use. require(name) returns, in order of precedence:
// what it returned for name earlier in the turn; for a capability the
// document declares, its module, or false if the turn doesn't get it (see
// Engine.granted); or the document's Lua module name, the file name.lua
// next to the document, fetched through the engine's registry and run in
// the turn's sandbox. __host.capability(name) is require(name) for a
// capability the document declares, and never a Lua module.
func (e *Engine) requireFuncs(env *rt.Table, tc *TurnContext, uses map[string]compile.Need, granted map[string]Capability) (require, capability *rt.GoFunction) {
	loaded := map[string]rt.Value{}
	load := func(t *rt.Thread, name string, modules bool) (rt.Value, error) {
		if v, ok := loaded[name]; ok {
			return v, nil
		}
		v := rt.BoolValue(false)
		var err error
		switch _, declared := uses[name]; {
		case declared:
			if capability := granted[name]; capability != nil {
				v, err = capability.Module(tc)
			}
		case modules:
			v, err = e.module(t, env, tc, name)
		default:
			err = errors.New("the document doesn't declare it")
		}
		if err != nil {
			return rt.NilValue, fmt.Errorf("require %q: %w", name, err)
		}
		loaded[name] = v
		return v, nil
	}
	goFunc := func(name string, modules bool) *rt.GoFunction {
		return rt.NewGoFunction(func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
			arg, err := c.StringArg(0)
			if err != nil {
				return nil, err
			}
			v, err := load(t, arg, modules)
			if err != nil {
				return nil, err
			}
			return c.PushingNext1(t.Runtime, v), nil
		}, name, 1, false)
	}
	require, capability = goFunc("require", true), goFunc("capability", false)
	// Modules are charged for, and read through safeio, in
	// Engine.module; capabilities answer for their own functions.
	rt.SolemnlyDeclareCompliance(rt.ComplyCpuSafe|rt.ComplyMemSafe|rt.ComplyIoSafe, require, capability)
	return require, capability
}

// module runs the document's Lua module name, and returns what it returns,
// or true if that's nil, as Lua's require does. Modules are at or below
// the document's directory: for a local document, golua's safeio ensures
// it (see documentAccess), and for a remote one, the module's URL must be
// under the document's.
func (e *Engine) module(t *rt.Thread, env *rt.Table, tc *TurnContext, name string) (rt.Value, error) {
	if name == "" || strings.ContainsAny(name, ":\\") || strings.HasPrefix(name, "/") {
		return rt.NilValue, errors.New("a module is named by a path relative to the document, such as lib/story")
	}
	location := tc.Document.ResolveReference(&url.URL{Path: name + ".lua"})
	if !under(location, tc.Document) {
		return rt.NilValue, errors.New("a module must be at or below the document's directory")
	}
	unit, size, err := e.moduleUnit(t, tc.Context, location)
	if err != nil {
		return rt.NilValue, err
	}
	// Reading and compiling it, charged as golua's load charges, whether
	// or not it was compiled earlier, so that a turn's cost doesn't
	// depend on the engine's cache.
	t.LinearRequire(4, uint64(size))
	t.ReleaseBytes(size)
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
// if its content has been seen, and the size of its source. A local module
// is read through golua's safeio, with the turn's access, and a remote one
// fetched through the engine's registry.
func (e *Engine) moduleUnit(t *rt.Thread, ctx context.Context, location *url.URL) (*code.Unit, int, error) {
	var body io.ReadCloser
	if location.Scheme == "file" {
		f, err := safeio.OpenFile(t.Runtime, urlFilePath(location), os.O_RDONLY, 0)
		if err != nil {
			return nil, 0, err
		}
		body = f
	} else {
		src, err := e.registry.Resolve(location, "")
		if err != nil {
			return nil, 0, err
		}
		if body, _, err = src.Fetch(ctx); err != nil {
			return nil, 0, err
		}
	}
	defer body.Close()
	// No turn could afford more.
	source, err := io.ReadAll(io.LimitReader(body, memLimit+1))
	if err != nil {
		return nil, 0, err
	}
	if len(source) > memLimit {
		return nil, 0, errors.New("too big")
	}
	unit, err := e.compileModule(location.String(), source)
	return unit, len(source), err
}

// documentAccess returns the files a turn of document may reach: for a
// local document, reading those at or below its directory; for any other,
// nil, which, with IO safety required, means none.
func documentAccess(document *url.URL) (*safeio.FSAccess, error) {
	if document.Scheme != "file" {
		return nil, nil
	}
	return safeio.Unrestricted.Restrict([]safeio.FSGrant{{
		Dir:     filepath.Dir(urlFilePath(document)),
		Actions: safeio.ReadAction,
	}})
}

// under reports whether u is at or below the directory of document: the
// same scheme and host, and a path in that directory. ResolveReference has
// already taken out any dot segments.
func under(u, document *url.URL) bool {
	dir := document.Path[:strings.LastIndex(document.Path, "/")+1]
	return u.Scheme == document.Scheme && u.Host == document.Host && u.User.String() == document.User.String() &&
		strings.HasPrefix(u.Path, dir)
}

// urlFilePath is the local file path of a file: URL.
func urlFilePath(u *url.URL) string {
	p := u.Path
	if p == "" {
		p = u.Opaque
	}
	// A Windows path in a URL has a slash before its drive: /C:/dir.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}
