package compile

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/arnodel/golua/code"
	rt "github.com/arnodel/golua/runtime"
	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
)

// A Program is a whymark document compiled: Lua, and golua's code for it.
// It's plain data, which any number of turns, in any runtime, can run.
type Program struct {
	Settings
	// Lua is the chunk, as Lua source (see the package documentation for
	// how it's called). Its lines are the document's, so that errors give
	// the document's line numbers.
	Lua string
	// Unit is Lua compiled by golua, ready to load into a runtime.
	Unit *code.Unit
	// Passages are the document's passages, in order.
	Passages []Passage
}

// A Passage is a passage of a document.
type Passage struct {
	// ID is the passage's id: its heading's anchor.
	ID string
	// Title is its heading's text, without brackets.
	Title string
	// Quiet is whether its heading is left off its page: "# (Title)".
	Quiet bool
}

// Passage returns the passage id, and whether there's one.
func (p *Program) Passage(id string) (Passage, bool) {
	for _, ps := range p.Passages {
		if ps.ID == id {
			return ps, true
		}
	}
	return Passage{}, false
}

// StartPassage returns the id of the passage the document starts at: its
// settings' Start, else its first passage's, or "" without passages.
func (p *Program) StartPassage() string {
	if p.Start != "" || len(p.Passages) == 0 {
		return p.Start
	}
	return p.Passages[0].ID
}

// Compile compiles source, a whymark document, to a Program. name names
// the document in the code's errors, such as its URL. It's an error if
// source isn't a whymark document, is written for another version of the
// format, or doesn't compile.
func Compile(name string, source []byte) (*Program, error) {
	settings, err := ReadSettings(source)
	if err != nil {
		return nil, err
	}
	if settings.Version != Version {
		return nil, fmt.Errorf("written for version %d of whymark; this compiler knows version %d", settings.Version, Version)
	}
	p, err := toLua(source, settings.Uses)
	if err != nil {
		return nil, err
	}
	p.Settings = settings
	// Compiling needs a runtime, but not the sandbox: nothing runs.
	p.Unit, _, err = rt.New(io.Discard).CompileLuaChunk(name, []byte(p.Lua))
	if err != nil {
		return nil, err
	}
	return p, nil
}

// toLua compiles a whymark document to Lua. Each level-1 heading starts
// a passage, whose id is the heading's anchor; a heading in brackets,
// "# (Title)", is a quiet passage's, not shown on its page. Before the
// first heading, lua blocks are the prelude, run every turn, and anything
// else is ignored. In a passage:
//
//   - a lua fenced block runs where it is, and isn't shown;
//   - a code span with one backquote runs: `= expr` writes expr's value,
//     anything else is Lua statements, such as `if turn.state.lamp then`;
//   - a line holding only statement spans disappears, newline and all;
//   - a code span with two or more backquotes is shown, as code;
//   - a capability block, a fenced block named after a capability the
//     document declares in uses, captures its lines, spans run, and
//     hands them to the capability, if the turn has it (see
//     whymark.Capability); a without block, "without" and the
//     capability's name, runs as ordinary lines if the turn hasn't it.
//
// The chunk's lines are the document's, so that errors give its line
// numbers.
func toLua(source []byte, uses map[string]Need) (*Program, error) {
	// Front matter is blanked, keeping its lines.
	if _, n, ok := splitFrontMatter(source); ok {
		lines := bytes.SplitN(source, []byte("\n"), n+1)
		source = append(bytes.Repeat([]byte("\n"), n), lines[n]...)
	}
	lines := strings.Split(string(source), "\n")
	lineOf := func(offset int) int { return bytes.Count(source[:offset], []byte("\n")) }

	// What each line is, besides an ordinary line of a passage.
	type lineKind int
	const (
		ordinary     lineKind = iota
		literal               // a line of code to show: spans don't run
		heading               // a passage's heading
		hidden                // a setext heading's underline: not shown
		luaFence              // the fence of a lua block
		luaLine               // a line of Lua
		blockFence            // the opening fence of a capability block
		withoutFence          // the opening fence of a without block
		closingFence          // the closing fence of either
	)
	kinds := make([]lineKind, len(lines))
	// A capability block's capability and arguments, by its opening line.
	blocks := map[int][]string{}
	titles, quiet, ids := map[int]string{}, map[int]bool{}, map[int]string{}
	p := &Program{}

	doc := parser.New(parser.WithAutoHeadingID()).Parse(source)
	err := gmast.Walk(doc, func(n gmast.Node, entering bool) (gmast.WalkStatus, error) {
		if !entering {
			return gmast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *gmast.Heading:
			if n.Level != 1 || n.Parent() != doc {
				return gmast.WalkContinue, nil
			}
			line := lineOf(n.Pos())
			var parts []string
			for _, seg := range n.Source() {
				parts = append(parts, string(source[seg.Start:seg.Stop]))
			}
			title := strings.TrimSpace(strings.Join(parts, " "))
			if inner, ok := strings.CutPrefix(title, "("); ok && strings.HasSuffix(inner, ")") {
				title, quiet[line] = strings.TrimSpace(strings.TrimSuffix(inner, ")")), true
			}
			if title == "" {
				return gmast.WalkStop, fmt.Errorf("line %d: a passage needs a title", line+1)
			}
			id, _ := n.Attribute("id")
			kinds[line], titles[line], ids[line] = heading, title, id.Value(source)
			p.Passages = append(p.Passages, Passage{ID: ids[line], Title: title, Quiet: quiet[line]})
			if n.HeadingKind == gmast.HeadingKindSetext {
				for i := line + 1; i <= line+len(parts); i++ {
					kinds[i] = hidden
				}
			}
			return gmast.WalkSkipChildren, nil
		case *gmast.CodeBlock:
			open, last := lineOf(n.Pos()), lineOf(n.Pos())
			if segs := n.Value.Segments(); len(segs) > 0 {
				last = lineOf(segs[len(segs)-1].Start)
			}
			close := -1
			if n.CodeBlockKind == gmast.CodeBlockKindFenced && last+1 < len(lines) && isFence(lines[last+1]) {
				close = last + 1
			}
			info := strings.Fields(n.Info.Value(source))
			kind := literal
			switch {
			case n.CodeBlockKind != gmast.CodeBlockKindFenced || n.Parent() != doc || len(info) == 0:
			case info[0] == "lua":
				kind = luaFence
			case info[0] == "without" && len(info) == 2 && declared(uses, info[1]):
				kind = withoutFence
			case declared(uses, info[0]):
				kind = blockFence
			}
			if kind == literal {
				for i := open; i <= max(last, close); i++ {
					kinds[i] = literal
				}
				return gmast.WalkContinue, nil
			}
			if close < 0 {
				return gmast.WalkStop, fmt.Errorf("line %d: a %s block needs its closing fence", open+1, info[0])
			}
			switch kind {
			case luaFence:
				kinds[open], kinds[close] = luaFence, luaFence
				for i := open + 1; i < close; i++ {
					kinds[i] = luaLine
				}
			default:
				// Their lines are ordinary ones.
				kinds[open], kinds[close], blocks[open] = kind, closingFence, info
			}
		}
		return gmast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, len(lines))
	// The chunk's first line also declares what passages need, so that
	// the document's lines keep their numbers.
	out[0] = `local turn, __host = ... local __passages, __go = {}, {} local function __value(x) if x ~= nil then turn.write(x) end end ` + blocksLua
	inPassages := false
	blockEnd := "" // what closes the block being compiled
	for i, line := range lines {
		switch {
		case kinds[i] == heading:
			if inPassages {
				out[i] += "end} "
			}
			inPassages = true
			out[i] += fmt.Sprintf("__passages[%s] = {title = %s, quiet = %t, body = function(turn) ", luaQuote(ids[i]), luaQuote(titles[i]), quiet[i])
		case kinds[i] == luaLine:
			out[i] += line
		case !inPassages:
			// Before the first passage, only the prelude's Lua counts.
		case kinds[i] == blockFence:
			var args []string
			for _, a := range blocks[i][1:] {
				args = append(args, luaQuote(a))
			}
			out[i] += fmt.Sprintf("__block(%s, {%s}, function() ", luaQuote(blocks[i][0]), strings.Join(args, ", "))
			blockEnd = "end) "
		case kinds[i] == withoutFence:
			out[i] += fmt.Sprintf("if not __host.capability(%s) then ", luaQuote(blocks[i][1]))
			blockEnd = "end "
		case kinds[i] == closingFence:
			out[i] += blockEnd
		case kinds[i] == ordinary || kinds[i] == literal:
			if i == len(lines)-1 && line == "" {
				continue // the end of the file's last line
			}
			code, err := compileLine(line, kinds[i] == literal)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			out[i] += code
		}
	}
	end := "\n" + noPassagesLua
	if inPassages {
		end = " end}\n" + passagesLua
	}
	p.Lua = strings.Join(out, "\n") + end
	return p, nil
}

// declared reports whether uses declares the capability name.
func declared(uses map[string]Need, name string) bool {
	_, ok := uses[name]
	return ok
}

// blocksLua defines __block(name, args, f), which runs a capability
// block: it hands what f writes, instead of writing it, to the
// capability's __block, if the turn has the capability.
const blocksLua = `local function __block(name, args, f) ` +
	`local c = __host.capability(name) if not c then return end ` +
	`local b = c.__block if b == nil then error("blocks named " .. name .. " mean nothing to it", 2) end ` +
	`local w, parts = turn.write, {} turn.write = function(s) parts[#parts+1] = tostring(s) end ` +
	`local ok, err = pcall(f) turn.write = w if not ok then error(err, 0) end ` +
	`b(turn, args, table.concat(parts)) end `

// passagesLua ends the chunk of a document with passages: its page(turn)
// shows the turn's passage, its heading first unless it's quiet, and
// turn.go and turn.show reach the other passages.
const passagesLua = `
if page ~= nil then error("a document with passages can't define page(turn)", 0) end

local function passage(id, level)
  local p = __passages[id]
  if not p then error("no passage " .. string.format("%q", tostring(id)), level) end
  return p
end

function turn.go(id)
  passage(id, 3)
  if __host.started() then error("turn.go after the page has started", 2) end
  turn.passage = id
  error(__go, 0)
end

local depth = 0
function turn.show(id)
  local p = passage(id, 3)
  depth = depth + 1
  if depth > 100 then error("turn.show nested too deeply", 2) end
  p.body(turn)
  depth = depth - 1
end

function page(turn)
  for _ = 1, 100 do
    local p = passage(turn.passage, 0)
    __host.heading(p.quiet and "" or p.title)
    local ok, err = pcall(p.body, turn)
    if ok then return end
    if err ~= __go then error(err, 0) end
  end
  error("turn.go jumped too many times", 0)
end
`

// noPassagesLua ends the chunk of a document without passages.
const noPassagesLua = `
function turn.go() error("this document has no passages", 2) end
function turn.show() error("this document has no passages", 2) end
`

// isFence reports whether line closes a fenced code block.
func isFence(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

// compileLine compiles a line of a passage into Lua that writes it. In a
// literal line, code spans don't run.
func compileLine(line string, literal bool) (string, error) {
	if literal {
		return "turn.write(" + luaQuote(line+"\n") + ") ", nil
	}
	type piece struct {
		text string
		code bool
	}
	var pieces []piece
	text := ""
	for i := 0; i < len(line); {
		switch {
		case line[i] == '\\' && i+1 < len(line) && line[i+1] == '`':
			text += line[i : i+2]
			i += 2
			continue
		case line[i] != '`':
			text += line[i : i+1]
			i++
			continue
		}
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		end := closingRun(line, i+n, n)
		if end < 0 || n > 1 {
			// An unclosed run, or a span to show: Markdown's.
			if end < 0 {
				end = i + n
			} else {
				end += n
			}
			text += line[i:end]
			i = end
			continue
		}
		if text != "" {
			pieces = append(pieces, piece{text: text})
			text = ""
		}
		pieces = append(pieces, piece{text: strings.TrimSpace(line[i+1 : end]), code: true})
		i = end + 1
	}
	if text != "" {
		pieces = append(pieces, piece{text: text})
	}

	// A line holding only statements disappears.
	onlyStatements := true
	var statements strings.Builder
	for _, p := range pieces {
		if p.code && !strings.HasPrefix(p.text, "=") {
			statements.WriteString(p.text + " ")
		} else if p.code || strings.TrimSpace(p.text) != "" {
			onlyStatements = false
		}
	}
	if onlyStatements && statements.Len() > 0 {
		return statements.String(), nil
	}

	var b strings.Builder
	for _, p := range pieces {
		switch {
		case !p.code:
			b.WriteString("turn.write(" + luaQuote(p.text) + ") ")
		case strings.HasPrefix(p.text, "="):
			b.WriteString("__value(" + strings.TrimSpace(p.text[1:]) + ") ")
		default:
			b.WriteString(p.text + " ")
		}
	}
	b.WriteString(`turn.write("\n") `)
	return b.String(), nil
}

// closingRun returns where in line, from start, the next run of exactly n
// backquotes starts, or -1 if there's none.
func closingRun(line string, start, n int) int {
	for i := start; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(line) && line[j] == '`' {
			j++
		}
		if j-i == n {
			return i
		}
		i = j
	}
	return -1
}

// luaQuote returns s as a Lua string literal, on one line.
func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c < ' ' || c == 0x7f:
			fmt.Fprintf(&b, `\%03d`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
