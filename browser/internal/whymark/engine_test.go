package whymark

import (
	"bufio"
	"context"
	"crypto/sha256"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
func testEngine(t *testing.T, files map[string]string) (*Engine, func(string) *url.URL) {
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
	return &Engine{Registry: fetch.NewRegistry(fetch.FileResolver{Root: root})}, func(name string) *url.URL {
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
		if _, _, err := run(prog, p, turnInput{state: state, passage: "cellar"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), c.want) {
			t.Errorf("with the lamp %v, the cellar says:\n%s\nwant %q", c.lamp, out.String(), c.want)
		}
	}
}

// TestRandomFromRequest checks that math.random gives the same numbers
// for the same request, Reload's, and others for another.
func TestRandomFromRequest(t *testing.T) {
	doc := "---\nwhymark:\n  version: 1\n---\n# Roll\n\nYou roll `= math.random(1000000)`. [Again](?again=1)\n"
	e, file := testEngine(t, map[string]string{"roll.md": doc})
	first := take(t, e, Request{Location: file("roll.md")})
	req, _ := Follow(first.Header, "?again=1")
	a, b := take(t, e, req), take(t, e, req)
	if a.body != b.body {
		t.Errorf("the same request rolled differently:\n%s\n%s", a.body, b.body)
	}
	req.Location.RawQuery = "again=2"
	if c := take(t, e, req); c.body == a.body {
		t.Error("another request rolled the same")
	}
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
	e, file := testEngine(t, map[string]string{"cellar.md": read("cellar.md"), "clockmaker.md": read("clockmaker.md")})

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
}
