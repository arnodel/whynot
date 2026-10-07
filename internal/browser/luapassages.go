package browser

import (
	"bytes"
	"fmt"
	"strings"

	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
)

// A passage file is a lua: page written as Markdown. Each level-1
// heading starts a passage: a page, whose heading is its title. Before
// the first heading, only lua blocks are allowed: they run before every
// page. In a passage:
//
//   - a lua fenced block runs where it is, and isn't shown;
//   - a code span with one backquote runs: `= expr` writes expr's value,
//     anything else is Lua statements, such as `if state.lamp then`;
//   - a line holding only a statement span disappears, newline and all;
//   - a code span with two or more backquotes is shown, as code;
//   - a claude fenced block asks Claude to write its text, which is the
//     prompt, with spans run, and shows Claude's text where it is;
//   - a link to a passage's heading, such as [Go](#the-hall), goes to the
//     passage.
//
// compilePassages compiles a passage file into a Lua chunk defining page,
// whose lines are the file's, so that errors give the file's line
// numbers, and returns the passages' ids.
func compilePassages(source []byte) (code string, passages map[string]bool, err error) {
	lines := strings.Split(string(source), "\n")
	lineOf := func(offset int) int { return bytes.Count(source[:offset], []byte("\n")) }

	// What each line is, besides an ordinary line of a passage.
	type lineKind int
	const (
		ordinary    lineKind = iota
		literal              // a line of code to show: spans don't run
		heading              // a passage's heading
		hidden               // a setext heading's underline: not shown
		luaFence             // the fence of a lua block
		luaLine              // a line of Lua
		claudeOpen           // the opening fence of a claude block
		claudeClose          // its closing fence
		claudeLine           // a line of its prompt
	)
	kinds := make([]lineKind, len(lines))
	titles := map[int]string{}
	ids := map[int]string{}
	passages = map[string]bool{}

	doc := parser.New(parser.WithAutoHeadingID()).Parse(source)
	err = gmast.Walk(doc, func(n gmast.Node, entering bool) (gmast.WalkStatus, error) {
		if !entering {
			return gmast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *gmast.Heading:
			if n.Level != 1 || n.Parent() != doc {
				return gmast.WalkContinue, nil
			}
			line := lineOf(n.Pos())
			var title []string
			for _, seg := range n.Source() {
				title = append(title, string(source[seg.Start:seg.Stop]))
			}
			id, _ := n.Attribute("id")
			if len(title) == 0 {
				return gmast.WalkStop, fmt.Errorf("line %d: a passage needs a title", line+1)
			}
			kinds[line], titles[line], ids[line] = heading, strings.Join(title, " "), id.Value(source)
			passages[ids[line]] = true
			if n.HeadingKind == gmast.HeadingKindSetext {
				for i := line + 1; i <= line+len(title); i++ {
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
			language, _ := n.Language(source)
			if n.CodeBlockKind != gmast.CodeBlockKindFenced || n.Parent() != doc || (language != "lua" && language != "claude") {
				for i := open; i <= max(last, close); i++ {
					kinds[i] = literal
				}
				return gmast.WalkContinue, nil
			}
			if close < 0 {
				return gmast.WalkStop, fmt.Errorf("line %d: a %s block needs its closing fence", open+1, language)
			}
			if language == "lua" {
				kinds[open], kinds[close] = luaFence, luaFence
				for i := open + 1; i < close; i++ {
					kinds[i] = luaLine
				}
			} else {
				kinds[open], kinds[close] = claudeOpen, claudeClose
				for i := open + 1; i < close; i++ {
					kinds[i] = claudeLine
				}
			}
		}
		return gmast.WalkContinue, nil
	})
	if err != nil {
		return "", nil, err
	}

	out := make([]string, len(lines))
	first := ""
	for i, line := range lines {
		switch kinds[i] {
		case heading:
			if first != "" {
				out[i] = "end} "
			} else {
				first = ids[i]
			}
			out[i] += fmt.Sprintf("__passages[%s] = {title = %s, body = function() ", luaQuote(ids[i]), luaQuote(titles[i]))
		case hidden, luaFence:
		case luaLine:
			out[i] = line
		case claudeOpen:
			out[i] = "claude{prompt = __capture(function() "
		case claudeClose:
			out[i] = `end)} out("\n") `
		default:
			if first == "" {
				if strings.TrimSpace(line) != "" {
					return "", nil, fmt.Errorf("line %d: before the first passage's heading, only lua blocks are allowed", i+1)
				}
				continue
			}
			if i == len(lines)-1 && line == "" {
				continue // the end of the file's last line
			}
			out[i], err = compileLine(line, kinds[i] == literal)
			if err != nil {
				return "", nil, fmt.Errorf("line %d: %w", i+1, err)
			}
		}
	}
	if first == "" {
		return "", nil, fmt.Errorf("no passages: a passage starts with a level-1 heading")
	}
	code = "local __passages = {} " + strings.Join(out, "\n") + " end}\n" + fmt.Sprintf(`
function show(id)
  local p = __passages[id]
  if not p then error("no passage " .. string.format("%%q", tostring(id)), 2) end
  p.body()
end

function page(request, state)
  passage = request.p or %s
  local p = __passages[passage]
  if not p then error("no passage " .. string.format("%%q", tostring(passage)), 0) end
  out("# ", p.title, "\n")
  p.body()
end
`, luaQuote(first))
	return code, passages, nil
}

// isFence reports whether line closes a fenced code block.
func isFence(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

// compileLine compiles a line of a passage into Lua that writes it. In a
// literal line, code spans don't run.
func compileLine(line string, literal bool) (string, error) {
	if literal {
		return "out(" + luaQuote(line+"\n") + ") ", nil
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
			b.WriteString("out(" + luaQuote(p.text) + ") ")
		case strings.HasPrefix(p.text, "="):
			b.WriteString("__value(" + strings.TrimSpace(p.text[1:]) + ") ")
		default:
			b.WriteString(p.text + " ")
		}
	}
	b.WriteString(`out("\n") `)
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
