// Command api-page renders whynot's public packages into one HTML page,
// with what changed since a previous release marked. See SKILL.md.
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/printer"
	"go/token"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const module = "github.com/arnodel/whynot"

// pkgs are the public packages, in reading order, by path within the repository.
var pkgs = []struct{ dir, group string }{
	{".", "Core"},
	{"canvas", "Contracts"},
	{"input", "Contracts"},
	{"fonts", "Contracts"},
	{"fetch", "Contracts"},
	{"codeblocks", "Contracts"},
	{"fonts/systemfont", "Ready-made"},
	{"codeblocks/chromahighlight", "Ready-made"},
	{"codeblocks/kroki", "Ready-made"},
	{"styles/simpletheme", "Ready-made"},
	{"backends/ebitenbackend", "Backends"},
	{"backends/giobackend", "Backends"},
}

//go:embed head.html
var head []byte

//go:embed tail.html
var tail []byte

func importPath(dir string) string {
	if dir == "." {
		return module
	}
	return module + "/" + dir
}

func shortName(dir string) string {
	if dir == "." {
		return "whynot"
	}
	return dir
}

func anchor(dir string, parts ...string) string {
	id := strings.ReplaceAll(shortName(dir), "/", "-")
	for _, p := range parts {
		if p != "" {
			id += "." + p
		}
	}
	return id
}

type symbol struct {
	key  string // "func NewView", "method View.ScrollBy", ...
	decl string
}

type loaded struct {
	fset *token.FileSet
	pkg  *doc.Package
}

func load(root, dir string) (*loaded, error) {
	full := filepath.Join(root, dir)
	bp, err := build.Default.ImportDir(full, 0)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range bp.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(full, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	p, err := doc.NewFromFiles(fset, files, importPath(dir))
	if err != nil {
		return nil, err
	}
	return &loaded{fset, p}, nil
}

func printNode(fset *token.FileSet, n any) string {
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	cfg.Fprint(&buf, fset, n)
	return buf.String()
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// symbols lists every exported symbol of l with its declaration.
func symbols(l *loaded) []symbol {
	var out []symbol
	values := func(kind string, vs []*doc.Value) {
		for _, v := range vs {
			for _, spec := range v.Decl.Specs {
				vspec := spec.(*ast.ValueSpec)
				for _, n := range vspec.Names {
					if n.IsExported() {
						out = append(out, symbol{kind + " " + n.Name, printNode(l.fset, vspec)})
					}
				}
			}
		}
	}
	funcs := func(fs []*doc.Func) {
		for _, f := range fs {
			key := "func " + f.Name
			if f.Recv != "" {
				key = "method " + strings.TrimPrefix(f.Recv, "*") + "." + f.Name
			}
			out = append(out, symbol{key, printNode(l.fset, f.Decl)})
		}
	}
	values("const", l.pkg.Consts)
	values("var", l.pkg.Vars)
	funcs(l.pkg.Funcs)
	for _, t := range l.pkg.Types {
		out = append(out, symbol{"type " + t.Name, printNode(l.fset, t.Decl)})
		values("const", t.Consts)
		values("var", t.Vars)
		funcs(t.Funcs)
		funcs(t.Methods)
	}
	return out
}

type change int

const (
	same change = iota
	added
	changed
)

type page struct {
	buf      bytes.Buffer
	newPkgs  map[string]bool
	old      map[string]map[string]string // package dir -> key -> decl
	removed  map[string][]symbol          // package dir -> removed symbols
	counts   map[string][3]int            // package dir -> new, changed, removed
	inSet    map[string]string            // import path -> dir
	commit   string
	baseline string
}

func (pg *page) status(dir, key, decl string) change {
	if pg.newPkgs[dir] {
		return added
	}
	old, ok := pg.old[dir][key]
	switch {
	case !ok:
		return added
	case norm(old) != norm(decl):
		return changed
	}
	return same
}

func badge(c change) string {
	switch c {
	case added:
		return ` <span class="badge new">new</span>`
	case changed:
		return ` <span class="badge changed">changed</span>`
	}
	return ""
}

func (pg *page) printer(dir string, l *loaded) *comment.Printer {
	pr := l.pkg.Printer()
	pr.HeadingLevel = 4
	pr.HeadingID = func(h *comment.Heading) string {
		var text strings.Builder
		for _, t := range h.Text {
			if p, ok := t.(comment.Plain); ok {
				text.WriteString(string(p))
			}
		}
		return anchor(dir, "hdr-"+strings.ReplaceAll(text.String(), " ", "-"))
	}
	pr.DocLinkURL = func(link *comment.DocLink) string {
		path := link.ImportPath
		if path == "" {
			path = importPath(dir)
		}
		if d, ok := pg.inSet[path]; ok {
			if link.Name == "" {
				return "#" + anchor(d)
			}
			return "#" + anchor(d, link.Recv, link.Name)
		}
		u := "https://pkg.go.dev/" + path
		if link.Name != "" {
			name := link.Name
			if link.Recv != "" {
				name = link.Recv + "." + name
			}
			u += "#" + name
		}
		return u
	}
	return pr
}

func (pg *page) docHTML(pr *comment.Printer, l *loaded, text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return `<div class="doc">` + string(pr.HTML(l.pkg.Parser().Parse(text))) + `</div>`
}

func (pg *page) symbolBlock(dir string, l *loaded, pr *comment.Printer, id, title, key string, node any, docText string) {
	decl := printNode(l.fset, node)
	c := pg.status(dir, key, decl)
	fmt.Fprintf(&pg.buf, `<article class="sym" id="%s"><h4><a href="#%s">%s</a>%s</h4>`, id, id, html.EscapeString(title), badge(c))
	if c == changed {
		fmt.Fprintf(&pg.buf, `<details class="was"><summary>In %s</summary><pre><code>%s</code></pre></details>`,
			pg.baseline, html.EscapeString(pg.old[dir][key]))
	}
	fmt.Fprintf(&pg.buf, `<pre><code>%s</code></pre>%s</article>`, html.EscapeString(decl), pg.docHTML(pr, l, docText))
}

func (pg *page) values(dir string, l *loaded, pr *comment.Printer, kind string, vs []*doc.Value) {
	for _, v := range vs {
		var names []string
		c := same
		for _, spec := range v.Decl.Specs {
			vspec := spec.(*ast.ValueSpec)
			for _, n := range vspec.Names {
				if !n.IsExported() {
					continue
				}
				names = append(names, n.Name)
				if s := pg.status(dir, kind+" "+n.Name, printNode(l.fset, vspec)); s > c {
					c = s
				}
			}
		}
		if len(names) == 0 {
			continue
		}
		id := anchor(dir, names[0])
		fmt.Fprintf(&pg.buf, `<article class="sym" id="%s">`, id)
		for _, n := range names[1:] {
			fmt.Fprintf(&pg.buf, `<span id="%s"></span>`, anchor(dir, n))
		}
		fmt.Fprintf(&pg.buf, `<h4><a href="#%s">%s %s</a>%s</h4>`, id, kind, html.EscapeString(strings.Join(names, ", ")), badge(c))
		fmt.Fprintf(&pg.buf, `<pre><code>%s</code></pre>%s</article>`, html.EscapeString(printNode(l.fset, v.Decl)), pg.docHTML(pr, l, v.Doc))
	}
}

func (pg *page) funcs(dir string, l *loaded, pr *comment.Printer, fs []*doc.Func) {
	for _, f := range fs {
		recv := strings.TrimPrefix(f.Recv, "*")
		key, title, id := "func "+f.Name, "func "+f.Name, anchor(dir, f.Name)
		if f.Recv != "" {
			key = "method " + recv + "." + f.Name
			title = "func (" + f.Recv + ") " + f.Name
			id = anchor(dir, recv, f.Name)
		}
		pg.symbolBlock(dir, l, pr, id, title, key, f.Decl, f.Doc)
	}
}

func (pg *page) pkgSection(dir, group string, l *loaded) {
	p := l.pkg
	pr := pg.printer(dir, l)
	id := anchor(dir)
	status := ""
	if pg.newPkgs[dir] {
		status = badge(added)
	}
	fmt.Fprintf(&pg.buf, `<section class="pkg" id="%s"><header class="pkg-head"><p class="eyebrow">%s</p>`, id, group)
	fmt.Fprintf(&pg.buf, `<h2><a href="#%s">%s</a>%s</h2><p class="import"><code>import "%s"</code></p>`, id, html.EscapeString(shortName(dir)), status, importPath(dir))
	fmt.Fprintf(&pg.buf, `<label class="read"><input type="checkbox" id="read-%s" data-pkg="%s"> Read</label></header>`, id, id)
	pg.buf.WriteString(pg.docHTML(pr, l, p.Doc))
	if len(p.Consts)+len(p.Vars) > 0 {
		pg.buf.WriteString(`<h3>Constants and variables</h3>`)
		pg.values(dir, l, pr, "const", p.Consts)
		pg.values(dir, l, pr, "var", p.Vars)
	}
	if len(p.Funcs) > 0 {
		pg.buf.WriteString(`<h3>Functions</h3>`)
		pg.funcs(dir, l, pr, p.Funcs)
	}
	if len(p.Types) > 0 {
		pg.buf.WriteString(`<h3>Types</h3>`)
	}
	for _, t := range p.Types {
		pg.symbolBlock(dir, l, pr, anchor(dir, t.Name), "type "+t.Name, "type "+t.Name, t.Decl, t.Doc)
		pg.values(dir, l, pr, "const", t.Consts)
		pg.values(dir, l, pr, "var", t.Vars)
		pg.funcs(dir, l, pr, t.Funcs)
		pg.funcs(dir, l, pr, t.Methods)
	}
	if rs := pg.removed[dir]; len(rs) > 0 {
		fmt.Fprintf(&pg.buf, `<div class="removed"><h3>Removed since %s</h3>`, pg.baseline)
		for _, r := range rs {
			fmt.Fprintf(&pg.buf, `<p class="removed-key"><span class="badge gone">removed</span> <code>%s</code></p><pre><code>%s</code></pre>`,
				html.EscapeString(r.key), html.EscapeString(r.decl))
		}
		pg.buf.WriteString(`</div>`)
	}
	pg.buf.WriteString(`</section>`)
}

func main() {
	repo := flag.String("repo", ".", "the repository to document")
	oldRepo := flag.String("old", "", "a checkout of the baseline release")
	baseline := flag.String("baseline", "", "the baseline release's tag, such as v0.6.0")
	commit := flag.String("commit", "", "the commit being documented, shown on the page")
	removedPkgs := flag.String("removed", "", "comma-separated directories of packages that only exist in the baseline, such as images")
	out := flag.String("out", "whynot-api.html", "the HTML file to write")
	flag.Parse()
	if *oldRepo == "" || *baseline == "" {
		fmt.Fprintln(os.Stderr, "api-page: -old and -baseline are required")
		os.Exit(2)
	}
	var oldOnly []string
	if *removedPkgs != "" {
		oldOnly = strings.Split(*removedPkgs, ",")
	}
	pg := &page{
		newPkgs:  map[string]bool{},
		old:      map[string]map[string]string{},
		removed:  map[string][]symbol{},
		counts:   map[string][3]int{},
		inSet:    map[string]string{},
		commit:   *commit,
		baseline: *baseline,
	}
	for _, p := range pkgs {
		pg.inSet[importPath(p.dir)] = p.dir
	}

	cur := map[string]*loaded{}
	for _, p := range pkgs {
		l, err := load(*repo, p.dir)
		if err != nil {
			panic(err)
		}
		cur[p.dir] = l
		ol, err := load(*oldRepo, p.dir)
		if err != nil {
			pg.newPkgs[p.dir] = true
			continue
		}
		pg.old[p.dir] = map[string]string{}
		for _, s := range symbols(ol) {
			pg.old[p.dir][s.key] = s.decl
		}
	}

	// Counts and removals.
	for _, p := range pkgs {
		var n [3]int
		now := map[string]bool{}
		for _, s := range symbols(cur[p.dir]) {
			now[s.key] = true
			switch pg.status(p.dir, s.key, s.decl) {
			case added:
				n[0]++
			case changed:
				n[1]++
			}
		}
		for key, decl := range pg.old[p.dir] {
			if !now[key] {
				pg.removed[p.dir] = append(pg.removed[p.dir], symbol{key, decl})
			}
		}
		sort.Slice(pg.removed[p.dir], func(i, j int) bool { return pg.removed[p.dir][i].key < pg.removed[p.dir][j].key })
		n[2] = len(pg.removed[p.dir])
		pg.counts[p.dir] = n
	}
	var gone []string
	for _, d := range oldOnly {
		if ol, err := load(*oldRepo, d); err == nil {
			gone = append(gone, fmt.Sprintf(`<li><code>%s</code>: %d exported names</li>`, importPath(d), len(symbols(ol))))
		}
	}

	pg.buf.Write(head)

	// Sidebar.
	pg.buf.WriteString(`<nav class="side" aria-label="Packages"><ol>`)
	group := ""
	for _, p := range pkgs {
		if p.group != group {
			if group != "" {
				pg.buf.WriteString(`</ol></li>`)
			}
			group = p.group
			fmt.Fprintf(&pg.buf, `<li class="group"><span>%s</span><ol>`, group)
		}
		fmt.Fprintf(&pg.buf, `<li><a href="#%s" data-nav="%s">%s</a></li>`, anchor(p.dir), anchor(p.dir), html.EscapeString(shortName(p.dir)))
	}
	pg.buf.WriteString(`</ol></li></ol></nav><main>`)

	// Summary.
	fmt.Fprintf(&pg.buf, `<section class="intro"><h1>whynot v1 API</h1><p class="lede">Every public package's documentation, as <code>go doc -all</code> would show it, on one page, for an API read-through. Built from commit <code>%s</code>. Doc links go to the right place on this page.</p>`, *commit)
	fmt.Fprintf(&pg.buf, `<h3>Changes since %s</h3><div class="table-wrap"><table class="summary"><thead><tr><th scope="col">Package</th><th scope="col" class="num">New</th><th scope="col" class="num">Changed</th><th scope="col" class="num">Removed</th></tr></thead><tbody>`, *baseline)
	for _, p := range pkgs {
		n := pg.counts[p.dir]
		cell := func(v int, cls string) string {
			if v == 0 {
				return `<td class="num zero">0</td>`
			}
			return fmt.Sprintf(`<td class="num"><span class="count %s">%d</span></td>`, cls, v)
		}
		extra := ""
		if pg.newPkgs[p.dir] {
			extra = badge(added)
		}
		fmt.Fprintf(&pg.buf, `<tr><th scope="row"><a href="#%s">%s</a>%s</th>%s%s%s</tr>`, anchor(p.dir), html.EscapeString(shortName(p.dir)), extra, cell(n[0], "new"), cell(n[1], "changed"), cell(n[2], "gone"))
	}
	pg.buf.WriteString(`</tbody></table></div>`)
	if len(gone) > 0 {
		fmt.Fprintf(&pg.buf, `<p class="gone-pkgs"><span class="badge gone">removed</span> packages:</p><ul class="gone-list">%s</ul>`, strings.Join(gone, ""))
	}
	fmt.Fprintf(&pg.buf, `<ul class="legend"><li><span class="badge new">new</span> not in %[1]s</li><li><span class="badge changed">changed</span> differs from %[1]s: expand to see the old declaration</li><li><span class="badge gone">removed</span> listed at the end of its package</li></ul></section>`, *baseline)

	for _, p := range pkgs {
		pg.pkgSection(p.dir, p.group, cur[p.dir])
	}
	pg.buf.WriteString(`</main></div>`)
	pg.buf.Write(tail)
	if err := os.WriteFile(*out, pg.buf.Bytes(), 0o644); err != nil {
		panic(err)
	}
}
