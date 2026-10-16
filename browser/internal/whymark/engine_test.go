package whymark

import (
	"bufio"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	rt "github.com/arnodel/golua/runtime"

	"github.com/arnodel/whynot/browser/internal/whymark/compile"
	"github.com/arnodel/whynot/fetch"
)

// cellar is a tiny adventure, the format's running example.
const cellar = "---\n" +
	"title: The Cellar\n" +
	"whymark:\n" +
	"  version: 1\n" +
	"  journal: true\n" +
	"---\n" +
	"\n" +
	"A tiny adventure. This prose is for whoever reads the source.\n" +
	"\n" +
	"```lua\n" +
	"function init(turn) return { lamp = false } end\n" +
	"```\n" +
	"\n" +
	"# Hall\n" +
	"\n" +
	"You're in a hall. `= turn.state.lamp and \"Your lamp glows.\" or \"It's dark below.\"`\n" +
	"`if turn.query.shout then`\n" +
	"Your voice echoes.\n" +
	"`end`\n" +
	"\n" +
	"`if not turn.state.lamp then`\n" +
	"- [Take the lamp](#take-the-lamp)\n" +
	"`end`\n" +
	"- [Go down](#cellar) · [Shout](?shout=yes)\n" +
	"\n" +
	"# (Take the lamp)\n" +
	"\n" +
	"```lua\n" +
	"turn.state.lamp = true\n" +
	"```\n" +
	"\n" +
	"You take the lamp from its hook. [Back to the hall](#hall)\n" +
	"\n" +
	"# Cellar\n" +
	"\n" +
	"`if turn.state.lamp then`\n" +
	"A dusty bottle of port. **You win!**\n" +
	"`else`\n" +
	"It's pitch black. [Back up](#hall)\n" +
	"`end`\n"

// testEngine returns an Engine serving files, by name, from a temporary
// directory, and a function giving a file's URL.
func testEngine(t *testing.T, files map[string]string, options ...Option) (*Engine, func(string) *url.URL) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return NewEngine(fetch.NewRegistry(fetch.FileResolver{Root: root}), options...), func(name string) *url.URL {
		return &url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, name))}
	}
}

// turned is a page, as a user agent reads it.
type turned struct {
	Header
	body string
}

// take takes a turn, and reads its page.
func take(t *testing.T, e *Engine, req Request) turned {
	t.Helper()
	r, err := e.Turn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	br := bufio.NewReader(r)
	h, err := ReadHeader(br)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(br)
	if err != nil {
		t.Fatal(err)
	}
	return turned{Header: h, body: string(body)}
}

// follow takes the turn for the link, as written, on page p.
func follow(t *testing.T, e *Engine, p turned, link string) turned {
	t.Helper()
	req, ok := Follow(p.Header, link)
	if !ok {
		t.Fatalf("%s doesn't continue the trail", link)
	}
	return take(t, e, req)
}

// wants checks that body has each of want, and none of wantNot.
func wants(t *testing.T, what string, p turned, want []string, wantNot []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(p.body, w) {
			t.Errorf("%s: the page doesn't have %q:\n%s", what, w, p.body)
		}
	}
	for _, w := range wantNot {
		if strings.Contains(p.body, w) {
			t.Errorf("%s: the page has %q:\n%s", what, w, p.body)
		}
	}
}

// TestWalkthrough plays the cellar to its end, and checks a branch from
// an earlier page.
func TestWalkthrough(t *testing.T) {
	e, file := testEngine(t, map[string]string{"cellar.md": cellar})
	hall := take(t, e, Request{Location: file("cellar.md")})
	if hall.Location.Fragment != "hall" || hall.Title != "Hall" || !hall.Journal || hall.PageID == "" {
		t.Errorf("the first page's header is %+v, want the hall, titled Hall, with the journal hint and a page-id", hall.Header)
	}
	wants(t, "the hall", hall, []string{"# Hall\n", "It's dark below.", "[Take the lamp](#take-the-lamp)"}, []string{"tiny adventure", "Your voice"})

	lamp := follow(t, e, hall, "#take-the-lamp")
	if lamp.Title != "Take the lamp" {
		t.Errorf("a quiet passage's title is %q, want its heading", lamp.Title)
	}
	wants(t, "taking the lamp", lamp, []string{"You take the lamp"}, []string{"# Take the lamp", "# (Take"})

	lit := follow(t, e, lamp, "#hall")
	wants(t, "the hall, with the lamp", lit, []string{"Your lamp glows."}, []string{"Take the lamp"})
	won := follow(t, e, lit, "#cellar")
	wants(t, "the cellar, with the lamp", won, []string{"You win!"}, []string{"pitch black"})

	// From the first page, the lamp was never taken.
	dark := follow(t, e, hall, "#cellar")
	wants(t, "the cellar, from the first page", dark, []string{"pitch black"}, []string{"You win!"})

	if n := len(e.compiled); n != 1 {
		t.Errorf("the document was compiled %d times, want once", n)
	}
	if c := e.compiled[sha256.Sum256([]byte(cellar))]; c == nil || c.program.Unit == nil {
		t.Error("the compiled program isn't cached")
	}
}

// TestQueryStaysOnPassage checks that a query link stays on its page's
// passage, and that a passage link drops the query.
func TestQueryStaysOnPassage(t *testing.T) {
	e, file := testEngine(t, map[string]string{"cellar.md": cellar})
	hall := take(t, e, Request{Location: file("cellar.md")})
	shout := follow(t, e, hall, "?shout=yes")
	if shout.Location.Fragment != "hall" {
		t.Errorf("a query link went to %q, want the hall", shout.Location.Fragment)
	}
	wants(t, "shouting", shout, []string{"Your voice echoes."}, nil)
	down := follow(t, e, shout, "#cellar")
	if down.Location.RawQuery != "" {
		t.Errorf("a passage link kept the query: %s", down.Location)
	}
}

// TestFollow checks which links continue the trail.
func TestFollow(t *testing.T) {
	location, _ := url.Parse("file:///docs/cellar.md?shout=yes#hall")
	h := Header{Location: location, PageID: "7"}
	for _, c := range []struct {
		link, want string
		ok         bool
	}{
		{"#cellar", "file:///docs/cellar.md#cellar", true},
		{"?go=north", "file:///docs/cellar.md?go=north", true},
		{"cellar.md", "", false},
		{"https://example.com", "", false},
	} {
		req, ok := Follow(h, c.link)
		if ok != c.ok || ok && (req.Location.String() != c.want || req.FromPageID != "7") {
			t.Errorf("Follow(%q) = %v, %v, want %s, %v", c.link, req, ok, c.want, c.ok)
		}
	}
}

// TestGoAndShow checks a jump to another passage, and a passage shown in
// another as a subroutine.
func TestGoAndShow(t *testing.T) {
	doc := "---\nwhymark:\n  version: 1\n---\n" +
		"# Room\n\nA room. `turn.show(\"inventory\")`\n\n[Take the key](#take)\n\n" +
		"# (Take)\n\n```lua\nturn.state.key = true\nturn.go(\"room\")\n```\n\nNever shown.\n\n" +
		"# (Inventory)\n\nYou carry `= turn.state.key and \"a key\" or \"nothing\"`.\n"
	e, file := testEngine(t, map[string]string{"room.md": doc})
	room := take(t, e, Request{Location: file("room.md")})
	wants(t, "the room", room, []string{"# Room", "You carry nothing."}, []string{"Inventory"})
	took := follow(t, e, room, "#take")
	if took.Location.Fragment != "room" || took.Title != "Room" {
		t.Errorf("after turn.go, the page is %s, titled %q, want the room", took.Location, took.Title)
	}
	wants(t, "after taking the key", took, []string{"# Room", "You carry a key."}, []string{"Never shown"})
}

// TestCodeDocument checks a document making its page in code: its
// headers must be written first, and default to the document's title.
func TestCodeDocument(t *testing.T) {
	good := "---\ntitle: Counter\nwhymark:\n  version: 1\n---\n\n```lua\n" +
		"function init(turn) return {n = 0} end\n" +
		"function page(turn)\n" +
		"  if turn.query.add then turn.state.n = turn.state.n + 1 end\n" +
		"  turn.write_headers{title = \"Count \" .. turn.state.n}\n" +
		"  turn.write(\"The count is \", turn.state.n, \". \", turn.link(\"Add one\", {add = 1}), \"\\n\")\n" +
		"end\n```\n"
	bad := "---\ntitle: Hasty\nwhymark:\n  version: 1\n---\n\n```lua\nfunction page(turn) turn.write(\"too soon\") end\n```\n"
	e, file := testEngine(t, map[string]string{"good.md": good, "bad.md": bad})
	first := take(t, e, Request{Location: file("good.md")})
	if first.Title != "Count 0" {
		t.Errorf("the title is %q, want what write_headers set", first.Title)
	}
	wants(t, "the counter", first, []string{"The count is 0.", "[Add one](?add=1)"}, nil)
	second := follow(t, e, first, "?add=1")
	wants(t, "after adding one", second, []string{"The count is 1."}, nil)

	hasty := take(t, e, Request{Location: file("bad.md")})
	if hasty.Title != "Hasty" {
		t.Errorf("a failed page's title is %q, want the document's", hasty.Title)
	}
	wants(t, "writing before the headers", hasty, []string{"The page stopped", "write_headers before turn.write"}, []string{"too soon"})
}

// TestFailedTurnKeepsState checks that a turn that fails keeps the state
// it started from.
func TestFailedTurnKeepsState(t *testing.T) {
	doc := "---\nwhymark:\n  version: 1\n---\n" +
		"# Start\n\nCoins: `= turn.state.coins or 0`. [Spend](#spend)\n\n" +
		"# Spend\n\n```lua\nturn.state.coins = 99\nerror(\"the till is broken\")\n```\n"
	e, file := testEngine(t, map[string]string{"shop.md": doc})
	start := take(t, e, Request{Location: file("shop.md")})
	spent := follow(t, e, start, "#spend")
	wants(t, "the failed turn", spent, []string{"The page stopped", "the till is broken"}, nil)
	back := follow(t, e, spent, "#start")
	wants(t, "after the failed turn", back, []string{"Coins: 0."}, []string{"99"})
}

// TestInput checks that typed input reaches the turn.
func TestInput(t *testing.T) {
	doc := "---\nwhymark:\n  version: 1\n---\n# Chat\n\nYou said: `= turn.input.say or \"nothing\"`.\n"
	e, file := testEngine(t, map[string]string{"chat.md": doc})
	first := take(t, e, Request{Location: file("chat.md")})
	req, _ := Follow(first.Header, "#chat")
	req.Input = map[string]string{"say": "hello"}
	wants(t, "after typing", take(t, e, req), []string{"You said: hello."}, nil)
}

// TestDocumentErrors checks what happens with documents and requests that
// can't make a page.
func TestDocumentErrors(t *testing.T) {
	e, file := testEngine(t, map[string]string{
		"cellar.md":  cellar,
		"plain.md":   "# Just Markdown\n",
		"future.md":  "---\nwhymark:\n  version: 2\n---\n# Hall\n",
		"broken.md":  "---\nwhymark:\n  version: 1\n---\n# Hall\n\n`if then`\n",
		"nowhere.md": "---\nwhymark:\n  version: 1\n---\nJust prose, and no page(turn).\n",
	})
	if _, err := e.Turn(context.Background(), Request{Location: file("plain.md")}); err == nil {
		t.Error("a plain Markdown file made a page")
	}
	hall := take(t, e, Request{Location: file("cellar.md")})
	other := file("future.md")
	if _, err := e.Turn(context.Background(), Request{Location: other, FromPageID: hall.PageID}); err == nil {
		t.Error("a page of one document continued another")
	}
	if _, err := e.Turn(context.Background(), Request{Location: file("cellar.md"), FromPageID: "nope"}); err == nil {
		t.Error("an unknown page-id made a page")
	}
	missing := file("cellar.md")
	missing.Fragment = "attic"
	wants(t, "a missing passage", take(t, e, Request{Location: missing}), []string{"The page stopped", `no passage "attic"`}, nil)
	wants(t, "a newer version", take(t, e, Request{Location: file("future.md")}), []string{"version 2"}, nil)
	// Line 7 of the file, front matter included.
	wants(t, "a syntax error", take(t, e, Request{Location: file("broken.md")}), []string{"The page stopped", "broken.md:7"}, nil)
	wants(t, "no passages or page", take(t, e, Request{Location: file("nowhere.md")}), []string{"no passages, and defines no page(turn)"}, nil)
}

// TestSpecExample plays the document in the specification, whymark.md, so
// that it stays true.
func TestSpecExample(t *testing.T) {
	spec, err := os.ReadFile("../../whymark.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(spec), "````markdown\n")
	doc, _, ok2 := strings.Cut(rest, "````\n")
	if !ok || !ok2 {
		t.Fatal("no example document in the specification")
	}
	e, file := testEngine(t, map[string]string{"cellar.md": doc})
	hall := take(t, e, Request{Location: file("cellar.md")})
	wants(t, "the spec's hall", hall, []string{"# Hall", "It's dark below."}, []string{"Prose before"})
	lamp := follow(t, e, hall, "#take-the-lamp")
	wants(t, "the spec's lamp", lamp, []string{"You take the lamp"}, []string{"# Take"})
	wants(t, "the spec's cellar", follow(t, e, follow(t, e, lamp, "#hall"), "#cellar"), []string{"You win!"}, nil)
}

// TestRunProgram runs a compiled program on its own, with no engine and
// no request: a program is a value, which any number of turns can run.
func TestRunProgram(t *testing.T) {
	prog, err := compile.Compile("cellar.md", []byte(cellar))
	if err != nil {
		t.Fatal(err)
	}
	location, _ := url.Parse("cellar.md")
	for _, c := range []struct {
		lamp bool
		want string
	}{{false, "pitch black"}, {true, "You win!"}} {
		var out strings.Builder
		p := &page{w: &out, header: Header{Location: location, PageID: "test"}, auto: true, program: prog}
		state := &table{keys: []any{"lamp"}, values: []any{c.lamp}}
		if _, err := run(prog, p, turnInput{state: state, passage: "cellar"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), c.want) {
			t.Errorf("with the lamp %v, the cellar says:\n%s\nwant %q", c.lamp, out.String(), c.want)
		}
	}
}

// TestRandomPerPage checks that rolls belong to the page: the same
// request makes a new page, which rolls anew; and that dice seeded from
// the state follow the story instead.
func TestRandomPerPage(t *testing.T) {
	doc := "---\nwhymark:\n  version: 1\n---\n" +
		"```lua\nfunction init(turn) return {seed = math.random(1 << 30)} end\n```\n\n" +
		"# Roll\n\nYou roll `= math.random(1000000)`.\n" +
		"`math.randomseed(turn.state.seed)`\nThe story rolls `= math.random(1000000)`.\n" +
		"`turn.state.seed = math.random(1 << 30)`\n[Again](?again=1)\n"
	e, file := testEngine(t, map[string]string{"roll.md": doc})
	first := take(t, e, Request{Location: file("roll.md")})
	req, _ := Follow(first.Header, "?again=1")
	a, b := take(t, e, req), take(t, e, req)
	free := func(p turned) string { return strings.SplitN(p.body, "The story", 2)[0] }
	story := func(p turned) string { return strings.SplitN(p.body, "The story", 2)[1] }
	if free(a) == free(b) {
		t.Errorf("two pages from the same request rolled the same:\n%s\n%s", a.body, b.body)
	}
	if story(a) != story(b) {
		t.Errorf("dice seeded from the state rolled differently for the same request:\n%s\n%s", a.body, b.body)
	}
}

// greeter is a capability for tests, "greet": its module has hello(name),
// which writes a greeting into the page, and the page's id, and its blocks
// write their arguments and text.
type greeter struct{}

func (greeter) Name() string { return "greet" }

func (greeter) Module(tc *TurnContext) (rt.Value, error) {
	m := rt.NewTable()
	hello := rt.NewGoFunction(func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		name, err := c.StringArg(0)
		if err != nil {
			return nil, err
		}
		if err := tc.Write("Hello, " + name + "!"); err != nil {
			return nil, err
		}
		return c.Next(), nil
	}, "hello", 1, false)
	block := rt.NewGoFunction(func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		args, err := c.TableArg(1)
		if err != nil {
			return nil, err
		}
		text, err := c.StringArg(2)
		if err != nil {
			return nil, err
		}
		var words []string
		for i := int64(1); i <= args.Len(); i++ {
			s, _ := args.Get(rt.IntValue(i)).TryString()
			words = append(words, s)
		}
		if err := tc.Write("[" + strings.Join(words, " ") + "] " + text); err != nil {
			return nil, err
		}
		return c.Next(), nil
	}, "__block", 3, false)
	rt.SolemnlyDeclareCompliance(rt.ComplyCpuSafe|rt.ComplyMemSafe|rt.ComplyIoSafe, hello, block)
	m.Set(rt.StringValue("hello"), rt.FunctionValue(hello))
	m.Set(rt.StringValue("__block"), rt.FunctionValue(block))
	m.Set(rt.StringValue("page"), rt.StringValue(tc.PageID))
	return rt.TableValue(m), nil
}

// TestCapabilities checks what require gives for capabilities: the module,
// when declared, available and granted; false when optional but not
// granted or not available; an error page when required but missing; and
// nothing at all when not declared.
func TestCapabilities(t *testing.T) {
	doc := func(uses string) string {
		return "---\nwhymark:\n  version: 1\n" + uses + "---\n# Hi\n\n" +
			"```lua\nlocal greet = require \"greet\"\nif greet then greet.hello(\"reader\") else turn.write(\"No greeting.\") end\n```\n"
	}
	files := map[string]string{
		"optional.md": doc("  uses:\n    greet: optional\n"),
		"required.md": doc("  uses:\n    greet: required\n"),
		"none.md":     doc(""),
	}
	allow := true
	e, file := testEngine(t, files, WithCapabilities(greeter{}), WithGrant(func(document *url.URL, name string) bool { return allow }))

	wants(t, "granted", take(t, e, Request{Location: file("optional.md")}), []string{"Hello, reader!"}, nil)
	wants(t, "required and granted", take(t, e, Request{Location: file("required.md")}), []string{"Hello, reader!"}, nil)
	wants(t, "not declared", take(t, e, Request{Location: file("none.md")}), []string{"The page stopped", `require "greet"`}, []string{"Hello"})

	allow = false
	wants(t, "optional, not granted", take(t, e, Request{Location: file("optional.md")}), []string{"No greeting."}, []string{"Hello"})
	wants(t, "required, not granted", take(t, e, Request{Location: file("required.md")}), []string{"The page stopped", "isn't allowed"}, []string{"Hello", "No greeting"})

	e, file = testEngine(t, files)
	wants(t, "optional, not available", take(t, e, Request{Location: file("optional.md")}), []string{"No greeting."}, nil)
	wants(t, "required, not available", take(t, e, Request{Location: file("required.md")}), []string{"The page stopped", "isn't available"}, nil)
}

// TestCapabilityBlocks checks blocks named after a capability: their text,
// spans run, and arguments handed to the capability's __block; without
// blocks, shown only without the capability; and both as code when the
// document doesn't declare the capability.
func TestCapabilityBlocks(t *testing.T) {
	body := "# Hi\n\nBefore.\n\n```greet loud warm\nOne `= 1 + 1`\n```\n```without greet\nNo greeter.\n```\n\nAfter.\n"
	files := map[string]string{
		"blocks.md": "---\nwhymark:\n  version: 1\n  uses:\n    greet: optional\n---\n" + body,
		"plain.md":  "---\nwhymark:\n  version: 1\n---\n" + body,
		"nothing.md": "---\nwhymark:\n  version: 1\n  uses:\n    greet: optional\n---\n```lua\nlocal m = require \"greet\"\nif m then m.__block = nil end\n```\n" +
			"# Hi\n\n```greet\nHello\n```\n",
	}
	allow := true
	e, file := testEngine(t, files, WithCapabilities(greeter{}), WithGrant(func(*url.URL, string) bool { return allow }))
	wants(t, "with the capability", take(t, e, Request{Location: file("blocks.md")}), []string{"Before.\n\n[loud warm] One 2\n", "After."}, []string{"No greeter", "```"})
	wants(t, "undeclared", take(t, e, Request{Location: file("plain.md")}), []string{"```greet loud warm\nOne `= 1 + 1`\n```", "```without greet\nNo greeter.\n```"}, nil)
	wants(t, "no __block", take(t, e, Request{Location: file("nothing.md")}), []string{"The page stopped", "mean nothing"}, nil)
	allow = false
	wants(t, "without the capability", take(t, e, Request{Location: file("blocks.md")}), []string{"Before.\n\nNo greeter.\n\nAfter."}, []string{"loud", "One"})
}

// TestModules checks the document's own Lua modules: one next to the
// document, required from its prelude and a passage, run once a turn, and
// errors for a missing one and a bad name.
func TestModules(t *testing.T) {
	e, file := testEngine(t, map[string]string{
		"story.md": "---\nwhymark:\n  version: 1\n---\n```lua\nlocal helpers = require \"helpers\"\n```\n\n" +
			"# Start\n\n`= require(\"helpers\").shout(\"hello\")` (loaded `= require(\"helpers\").loads` time)\n",
		"helpers.lua": "loads = (loads or 0) + 1\nreturn {shout = function(s) return s:upper() .. \"!\" end, loads = loads}\n",
		"missing.md":  "---\nwhymark:\n  version: 1\n---\n# Start\n\n`require \"nowhere\"`\n",
		"badname.md":  "---\nwhymark:\n  version: 1\n---\n# Start\n\n`require \"/etc/passwd\"`\n",
	})
	wants(t, "a module", take(t, e, Request{Location: file("story.md")}), []string{"HELLO! (loaded 1 time)"}, nil)
	wants(t, "a missing module", take(t, e, Request{Location: file("missing.md")}), []string{"The page stopped", `require "nowhere"`}, nil)
	wants(t, "a bad module name", take(t, e, Request{Location: file("badname.md")}), []string{"The page stopped", "relative to the document"}, nil)
}

// TestNoFileAccess checks that the sandbox has none of golua's functions
// reading files: turns don't require IO safety, which would rule them out.
func TestNoFileAccess(t *testing.T) {
	e, file := testEngine(t, map[string]string{
		"files.md": "---\nwhymark:\n  version: 1\n---\n# Files\n\n" +
			"`= tostring(dofile) .. tostring(loadfile) .. tostring(io) .. tostring(os) .. tostring(package)`\n",
	})
	wants(t, "the sandbox", take(t, e, Request{Location: file("files.md")}), []string{"nilnilnilnilnil"}, nil)
}

// TestModulesStayBelow checks that a document's modules are at or below
// its directory: one above is refused, and so is one reached through a
// symbolic link pointing out of it, which golua's safeio refuses.
func TestModulesStayBelow(t *testing.T) {
	doc := func(module string) string {
		return "---\nwhymark:\n  version: 1\n---\n# Start\n\n`= require(\"" + module + "\")`\n"
	}
	e, file := testEngine(t, map[string]string{
		"secret.lua":       "return \"the secret\"\n",
		"story/lib/ok.lua": "return \"fine\"\n",
		"story/below.md":   doc("lib/ok"),
		"story/above.md":   doc("../secret"),
		"story/linked.md":  doc("linked"),
	})
	wants(t, "a module below", take(t, e, Request{Location: file("story/below.md")}), []string{"fine"}, []string{"The page stopped"})
	wants(t, "a module above", take(t, e, Request{Location: file("story/above.md")}), []string{"The page stopped", "at or below"}, []string{"the secret"})
	if err := os.Symlink(filepath.FromSlash(file("secret.lua").Path), filepath.FromSlash(file("story/linked.lua").Path)); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	wants(t, "a link out", take(t, e, Request{Location: file("story/linked.md")}), []string{"The page stopped", `require "linked"`}, []string{"the secret"})
}

// TestRemoteModules checks a remote document's modules: fetched from
// below its URL, and refused above it.
func TestRemoteModules(t *testing.T) {
	files := map[string]string{
		"/stories/story.md":   "---\nwhymark:\n  version: 1\n---\n# Start\n\n`= require(\"lib/ok\")` `= turn.query.up and require(\"../secret\") or \"\"`\n",
		"/stories/lib/ok.lua": "return \"fine\"\n",
		"/secret.lua":         "return \"the secret\"\n",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, content)
	}))
	defer server.Close()
	e := NewEngine(fetch.NewRegistry(fetch.HTTPResolver{AllowHTTP: true}))
	location, _ := url.Parse(server.URL + "/stories/story.md")
	wants(t, "a module below", take(t, e, Request{Location: location}), []string{"fine"}, []string{"The page stopped"})
	location, _ = url.Parse(server.URL + "/stories/story.md?up=yes")
	wants(t, "a module above", take(t, e, Request{Location: location}), []string{"The page stopped", "at or below"}, []string{"the secret"})
}

// TestExamples plays the browser's example documents, in examples/.
func TestExamples(t *testing.T) {
	read := func(name string) string {
		data, err := os.ReadFile("../../examples/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	e, file := testEngine(t, map[string]string{"cellar.md": read("cellar.md"), "clockmaker.md": read("clockmaker.md"), "notebook.md": read("notebook.md"), "chat.md": read("chat.md"),
		"lib/dice.lua": read("lib/dice.lua"), "lib/story.lua": read("lib/story.lua")})

	hall := take(t, e, Request{Location: file("cellar.md")})
	won := follow(t, e, follow(t, e, follow(t, e, hall, "#take-the-lamp"), "#hall"), "#cellar")
	wants(t, "the cellar example", won, []string{"You win!"}, []string{"The page stopped"})

	shop := take(t, e, Request{Location: file("clockmaker.md")})
	wants(t, "the shop", shop, []string{"Every clock in the shop has stopped", "10 minutes to midnight"}, []string{"The page stopped", "Describe the shop"})
	page := shop
	for range 5 {
		page = follow(t, e, page, "#the-workbench")
		if strings.Contains(page.body, "brass key") {
			break
		}
		page = follow(t, e, page, "#the-shop")
	}
	if !strings.Contains(page.body, "brass key") {
		t.Fatalf("five searches of the workbench found no key:\n%s", page.body)
	}
	page = follow(t, e, follow(t, e, page, "#the-shop"), "#the-cabinet")
	wants(t, "the cabinet", page, []string{"clock oil"}, nil)
	page = follow(t, e, follow(t, e, page, "#the-shop"), "#the-stairs")
	page = follow(t, e, page, "#midnight")
	wants(t, "midnight", page, []string{"You've won"}, []string{"The page stopped"})

	// The notebook: what's written on each page, walking back.
	write := func(p turned, text string) turned {
		req, _ := Follow(p.Header, "?")
		req.Input = map[string]string{"entry": text}
		return take(t, e, req)
	}
	empty := take(t, e, Request{Location: file("notebook.md")})
	wants(t, "the empty notebook", empty, []string{"Nothing written yet.", "```input entry"}, nil)
	two := write(write(empty, "eggs"), "milk")
	wants(t, "two entries", two, []string{"1. eggs", "2. milk"}, []string{"Nothing written"})
	other := write(empty, "tea")
	wants(t, "another notebook, from the start", other, []string{"1. tea"}, []string{"eggs"})

	// The chat, here without an AI (see package ai's tests for one).
	chat := take(t, e, Request{Location: file("chat.md")})
	wants(t, "the chat", chat, []string{"Say something to start.", "needs an AI", "give whynot an API key"}, []string{"The page stopped"})
	req, _ := Follow(chat.Header, "?")
	req.Input = map[string]string{"say": "Hello"}
	wants(t, "the chat, saying something", take(t, e, req), []string{"needs an AI"}, []string{"The page stopped", "You:"})
}

// TestLibDice checks the example dice module: fixed rolls are the same on
// sibling pages and move on with the trail, free rolls differ, a fixed
// roll leaves later free rolls free, and rolls are within their range.
func TestLibDice(t *testing.T) {
	dice, err := os.ReadFile("../../examples/lib/dice.lua")
	if err != nil {
		t.Fatal(err)
	}
	e, file := testEngine(t, map[string]string{
		"lib/dice.lua": string(dice),
		"rolls.md": "---\nwhymark:\n  version: 1\n---\n```lua\nlocal dice = require \"lib/dice\"\n" +
			"function init(turn) return dice.init({}) end\n" +
			"function free() local r = {} for i = 1, 20 do r[i] = dice.roll(1000) end return table.concat(r, \",\") end\n" +
			"function fixed() local r = {} for i = 1, 20 do r[i] = dice.fixed_roll(turn, \"d1000\") end return table.concat(r, \",\") end\n" +
			"function inrange(spec, lo, hi) for _ = 1, 200 do local v = dice.roll(spec) if v < lo or v > hi then return false end end return true end\n" +
			"```\n# (Rolls)\n\nfixed `= fixed()` free `= free()` end\n\n" +
			"ranges `= tostring(inrange(\"2d6+1\", 3, 13) and inrange(\"d20\", 1, 20) and inrange(4, 1, 4) and inrange(\"3d6-3\", 0, 15))`\n",
		"bad.md": "---\nwhymark:\n  version: 1\n---\n```lua\nlocal dice = require \"lib/dice\"\n```\n# Bad\n\n`= dice.roll(\"2x6\")`\n",
	})
	rolls := regexp.MustCompile(`fixed (\S+) free (\S+) end`)
	parse := func(p turned) (fixed, free string) {
		t.Helper()
		m := rolls.FindStringSubmatch(p.body)
		if m == nil {
			t.Fatalf("no rolls on the page:\n%s", p.body)
		}
		return m[1], m[2]
	}
	start := take(t, e, Request{Location: file("rolls.md")})
	wants(t, "the ranges", start, []string{"ranges true"}, []string{"The page stopped"})
	a, b := follow(t, e, start, "?"), follow(t, e, start, "?")
	fixedA, freeA := parse(a)
	fixedB, freeB := parse(b)
	if fixedA != fixedB {
		t.Errorf("sibling pages rolled differently with fixed rolls: %s and %s", fixedA, fixedB)
	}
	if freeA == freeB {
		t.Errorf("sibling pages rolled the same with free rolls, after fixed ones: %s", freeA)
	}
	if fixedNext, _ := parse(follow(t, e, a, "?")); fixedNext == fixedA {
		t.Errorf("the next page's fixed rolls are the same as its parent's: %s", fixedNext)
	}
	wants(t, "a bad roll", take(t, e, Request{Location: file("bad.md")}), []string{"The page stopped", "not a roll: 2x6"}, nil)
}

// TestTurnTime checks turn.time: when the page was made, by the engine's
// clock, with the fields of os.date("*t") and iso; and an earlier page's,
// kept with it.
func TestTurnTime(t *testing.T) {
	zone := time.FixedZone("test", 3600)
	now := time.Date(2026, 10, 16, 9, 12, 5, 0, zone) // a Friday
	e, file := testEngine(t, map[string]string{
		"time.md": "---\nwhymark:\n  version: 1\n---\n# (Time)\n\n" +
			"`local t = turn.time`\n" +
			"now `= t.year`-`= t.month`-`= t.day` `= t.hour`:`= t.min`:`= t.sec` wday `= t.wday` yday `= t.yday` iso `= t.iso`\n\n" +
			"`if turn.previous then`\nbefore `= turn.previous.time.iso`\n`end`\n",
	}, WithClock(func() time.Time { return now }))
	first := take(t, e, Request{Location: file("time.md")})
	wants(t, "the first page", first, []string{"now 2026-10-16 9:12:5 wday 6 yday 289 iso 2026-10-16T09:12:05+01:00"}, []string{"before", "The page stopped"})
	now = now.Add(26 * time.Hour)
	second := follow(t, e, first, "?")
	wants(t, "a page a day later", second, []string{"now 2026-10-17 11:12:5 wday 7", "before 2026-10-16T09:12:05+01:00"}, nil)
}

// TestTodoExample plays the to-do example without an AI, with commands:
// items and sublists, due dates relative to the page's time, sections by
// when things are due, urgent ones first, and what couldn't be done.
func TestTodoExample(t *testing.T) {
	read := func(name string) string {
		data, err := os.ReadFile("../../examples/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	now := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC) // a Friday
	e, file := testEngine(t, map[string]string{"todo.md": read("todo.md"), "lib/dates.lua": read("lib/dates.lua")},
		WithClock(func() time.Time { return now }))
	page := take(t, e, Request{Location: file("todo.md")})
	wants(t, "the empty list", page, []string{"Nothing to do yet.", "use commands"}, []string{"The page stopped"})
	say := func(text string) {
		t.Helper()
		req, _ := Follow(page.Header, "?")
		req.Input = map[string]string{"say": text}
		page = take(t, e, req)
	}
	say("add Groceries; add Call the bank by tomorrow !; add Milk under 1; add Eggs under 1 by +3")
	wants(t, "four items", page, []string{
		"- Added Call the bank",
		"## This week\n\n- [ ] Call the bank · due tomorrow · **urgent** · *#2* · [done](?act=done&id=2) · [remove](?act=remove&id=2)\n" +
			"- [ ] Groceries · *#1* · [done](?act=done&id=1) · [remove](?act=remove&id=1)\n" +
			"  - [ ] Eggs · due on Monday · *#4* ·",
	}, []string{"Couldn't", "The page stopped"})
	say("done 3; due 1 2026-10-10; add Taxes by +20; move 9 under 1; blah 2")
	wants(t, "changes and problems", page, []string{
		"- Done: Milk", "- Couldn't: there's no item 9", "- Couldn't: no operation \"blah\"",
		"## Overdue\n\n- [ ] Groceries · **overdue**, due 6 days ago · *#1* ·",
		"  - [x] Milk · *#3* · [undo](?act=undone&id=3) · [remove](?act=remove&id=3)\n",
		"## Later\n\n- [ ] Taxes · due in 20 days · *#5*",
	}, nil)
	// An item's links act on it at once.
	page = follow(t, e, page, "?act=done&id=2")
	wants(t, "a done link", page, []string{"- Done: Call the bank\n", "## Done\n\n- [x] Call the bank"}, []string{"Couldn't"})
	page = follow(t, e, page, "?act=remove&id=1")
	wants(t, "a remove link", page, []string{"- Removed Groceries"}, []string{"] Eggs", "] Milk"})
	page = follow(t, e, page, "?act=explode&id=5")
	wants(t, "a made-up link", page, []string{"- Couldn't: no action \"explode\""}, []string{"The page stopped"})
}

// TestSnapshotParents checks that each page remembers the page it came
// from, and the request it answered.
func TestSnapshotParents(t *testing.T) {
	e, file := testEngine(t, map[string]string{"cellar.md": cellar})
	hall := take(t, e, Request{Location: file("cellar.md")})
	lamp := follow(t, e, hall, "#take-the-lamp")
	dark := follow(t, e, hall, "#cellar")
	for _, c := range []struct {
		page, parent, passage string
	}{{hall.PageID, "", ""}, {lamp.PageID, hall.PageID, "take-the-lamp"}, {dark.PageID, hall.PageID, "cellar"}} {
		snap := e.pages[c.page]
		if snap.parent != c.parent || snap.request.Location.Fragment != c.passage {
			t.Errorf("page %s: parent %q, request %s; want parent %q, passage %q", c.page, snap.parent, snap.request.Location, c.parent, c.passage)
		}
	}
}

// TestPrevious checks turn.previous: the page the turn comes from, and
// through it the pages before, each with the state it left, its notes,
// its passage, and its request's query and input; and that a branch sees
// only its own.
func TestPrevious(t *testing.T) {
	doc := "---\nwhymark:\n  version: 1\n---\n" +
		"```lua\nfunction init(turn) return {score = 0} end\n" +
		"function pages()\n  local list, p = {}, turn.previous\n" +
		"  while p do table.insert(list, 1, p) p = p.previous end\n  return list\nend\n```\n\n" +
		"# (Play)\n\n" +
		"```lua\nif turn.query.add then turn.state.score = turn.state.score + tonumber(turn.query.add) end\n" +
		"turn.notes.said = turn.input.say\n```\n\n" +
		"`for i, page in ipairs(pages()) do`\n" +
		"- page `= i`: `= page.passage`, score `= page.state.score`, said `= page.notes.said or \"-\"`, typed `= page.input.say or \"-\"`, added `= page.query.add or \"-\"`\n" +
		"`end`\n\n" +
		"Score `= turn.state.score`. [Add two](?add=2) [Add three](?add=3)\n"
	e, file := testEngine(t, map[string]string{"play.md": doc})
	first := take(t, e, Request{Location: file("play.md")})
	wants(t, "the first page", first, []string{"Score 0."}, []string{"- page"})

	req, _ := Follow(first.Header, "?add=2")
	req.Input = map[string]string{"say": "hi"}
	second := take(t, e, req)
	third := follow(t, e, second, "?add=3")
	wants(t, "the third page", third, []string{
		"page 1: play, score 0, said -, typed -, added -",
		"page 2: play, score 2, said hi, typed hi, added 2",
		"Score 5.",
	}, []string{"page 3"})

	// From the first page again: the second and third pages aren't in
	// this branch.
	branch := follow(t, e, first, "?add=3")
	wants(t, "a branch from the first page", branch, []string{"page 1: play, score 0", "Score 3."}, []string{"page 2"})
}
