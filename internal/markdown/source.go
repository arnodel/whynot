package markdown

import (
	"unicode/utf8"

	gmast "github.com/yuin/goldmark/v2/ast"
	extast "github.com/yuin/goldmark/v2/extension/ast"

	"github.com/arnodel/whynot/internal/ast"
)

// Where elements are in the source, recorded as each one's ast.Range: a
// block from its start to where the next block starts (or its parent
// ends), less the blank lines between them; an inline element from its
// opening markup to the end of its closing markup. goldmark records where
// nodes start, but not where they end, so ends are worked out here.

// rangeOf returns the Range of [start, end) in c.source, as offsets in the
// whole document, or the zero Range if it's empty or unknown.
func (c *compiler) rangeOf(start, end int) ast.Range {
	if start < 0 || end <= start {
		return ast.Range{}
	}
	return ast.Range{Start: c.offset + start, End: c.offset + end}
}

// blockStart returns where a block node starts, or -1 if unknown.
func blockStart(node gmast.Node) int {
	if pos := node.Pos(); pos >= 0 {
		return pos
	}
	if b, ok := node.(gmast.BlockNode); ok {
		if segs := b.Source(); len(segs) > 0 {
			return segs[0].Start
		}
	}
	if cb, ok := node.(*gmast.CodeBlock); ok {
		if segs := cb.Value.Segments(); len(segs) > 0 {
			return segs[0].Start
		}
	}
	return -1
}

// blockEnd returns where a block starting at start ends, given next, where
// what follows it starts: before the whitespace that precedes next.
func (c *compiler) blockEnd(start, next int) int {
	next = min(next, len(c.source))
	for next > start && isSpace(c.source[next-1]) {
		next--
	}
	return next
}

// blockRanges returns the Ranges of first and its following block
// siblings, the last of which ends at end.
func (c *compiler) blockRanges(first gmast.Node, end int) map[gmast.Node]ast.Range {
	ranges := map[gmast.Node]ast.Range{}
	var prev gmast.Node
	prevStart := -1
	for node := first; node != nil; node = node.NextSibling() {
		if _, ok := node.(gmast.BlockNode); !ok {
			continue
		}
		start := blockStart(node)
		if prev != nil && start >= 0 {
			ranges[prev] = c.rangeOf(prevStart, c.blockEnd(prevStart, start))
		}
		prev, prevStart = node, start
	}
	if prev != nil {
		ranges[prev] = c.rangeOf(prevStart, c.blockEnd(prevStart, end))
	}
	return ranges
}

// localEnd returns r's end as an offset in c.source, for the blocks
// inside it.
func (c *compiler) localEnd(r ast.Range) int {
	if r == (ast.Range{}) {
		return len(c.source)
	}
	return r.End - c.offset
}

// textRange returns the source of a Text node, and whether its value is
// the decoded source, so that each word's place in the source can be found
// (see align). A value goldmark made, such as a smart quote, has no source
// of its own: it's taken to run from where the previous sibling ends to
// where the next one starts.
func (c *compiler) textRange(t *gmast.Text) (start, end int, decoded bool) {
	if !t.Value.IsOwned() {
		i := t.Value.Index()
		return i.Start, i.Stop, true
	}
	start, end = t.Pos(), -1
	if prev := t.PreviousSibling(); start < 0 && prev != nil {
		start = c.inlineEnd(prev)
	}
	if next := t.NextSibling(); next != nil {
		end = c.inlineStart(next)
	}
	if start < 0 || end < start {
		return -1, -1, false
	}
	return start, end, false
}

// inlineStart returns where an inline node starts, or -1 if unknown.
func (c *compiler) inlineStart(node gmast.Node) int {
	if t, ok := node.(*gmast.Text); ok && !t.Value.IsOwned() {
		return t.Value.Index().Start
	}
	if cs, ok := node.(*gmast.CodeSpan); ok {
		// goldmark's position is the code's: back from it to the run of
		// backquotes opening it.
		if is := cs.Value.Indices(); !cs.Value.IsOwned() && len(is) > 0 {
			i := is[0].Start
			for i > 0 && c.source[i-1] == ' ' {
				i--
			}
			for i > 0 && c.source[i-1] == '`' {
				i--
			}
			return i
		}
	}
	if pos := node.Pos(); pos >= 0 {
		return pos
	}
	if first := node.FirstChild(); first != nil {
		return c.inlineStart(first)
	}
	return -1
}

// inlineEnd returns where an inline node ends, closing markup included,
// or -1 if unknown.
func (c *compiler) inlineEnd(node gmast.Node) int {
	start := c.inlineStart(node)
	switch n := node.(type) {
	case *gmast.Text:
		_, end, _ := c.textRange(n)
		return end
	case *gmast.CodeSpan:
		return c.closingRun(start, '`')
	case *gmast.AutoLink:
		return c.after(start, '>')
	case *gmast.RawHTML:
		if is := n.Value.Indices(); !n.Value.IsOwned() && len(is) > 0 {
			return is[len(is)-1].Stop
		}
		return -1
	case *gmast.Link, *gmast.Image:
		label := start + 1
		if _, ok := node.(*gmast.Image); ok {
			label++
		}
		if last := node.LastChild(); last != nil {
			if end := c.inlineEnd(last); end >= 0 {
				label = end
			}
		}
		close := c.after(label, ']')
		if close < 0 {
			return -1
		}
		return c.linkTail(close)
	}
	if start < 0 {
		return -1
	}
	// Emphasis, strong emphasis and strikethrough: the content, then a
	// run of the opening delimiter.
	n := 1
	switch node.Kind() {
	case gmast.KindStrong:
		n = 2
	case extast.KindStrikethrough:
		n = runLength(c.source[start:], c.source[start])
	}
	end := start + n
	if last := node.LastChild(); last != nil {
		if e := c.inlineEnd(last); e >= 0 {
			end = e
		}
	}
	delim := c.source[start]
	for i := 0; i < n && end < len(c.source) && c.source[end] == delim; i++ {
		end++
	}
	return end
}

// closingRun returns where a code span starting at start ends: after the
// next run of backquotes as long as the one it opens with.
func (c *compiler) closingRun(start int, delim byte) int {
	if start < 0 {
		return -1
	}
	n := runLength(c.source[start:], delim)
	for i := start + n; i < len(c.source); {
		if c.source[i] != delim {
			i++
			continue
		}
		run := runLength(c.source[i:], delim)
		if run == n {
			return i + run
		}
		i += run
	}
	return -1
}

// after returns the position after the first b at or after i, or -1.
func (c *compiler) after(i int, b byte) int {
	if i < 0 {
		return -1
	}
	for ; i < len(c.source); i++ {
		if c.source[i] == b {
			return i + 1
		}
	}
	return -1
}

// linkTail returns where a link ends, given i, the position after its
// label's "]": after an inline destination "(...)", a reference "[...]",
// or at i for a shortcut reference.
func (c *compiler) linkTail(i int) int {
	if i >= len(c.source) {
		return i
	}
	switch c.source[i] {
	case '(':
		depth := 0
		var quote byte
		for j := i; j < len(c.source); j++ {
			ch := c.source[j]
			switch {
			case ch == '\\':
				j++
			case quote != 0:
				if ch == quote {
					quote = 0
				}
			case ch == '"' || ch == '\'':
				quote = ch
			case ch == '<':
				if k := c.after(j, '>'); k > 0 {
					j = k - 1
				}
			case ch == '(':
				depth++
			case ch == ')':
				depth--
				if depth == 0 {
					return j + 1
				}
			}
		}
	case '[':
		for j := i + 1; j < len(c.source); j++ {
			switch c.source[j] {
			case '\\':
				j++
			case ']':
				return j + 1
			}
		}
	}
	return i
}

// runLength returns how many b's s starts with.
func runLength(s []byte, b byte) int {
	n := 0
	for n < len(s) && s[n] == b {
		n++
	}
	return n
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// align maps each byte of decoded, the decoding of raw (escapes and
// entities resolved), to its offset in raw, with one more entry for the
// end; a character from an escape or an entity maps to where it starts.
// It returns nil if decoded isn't raw decoded that way.
func align(raw, decoded string) []int {
	offsets := make([]int, 0, len(decoded)+1)
	i := 0
	for j := 0; j < len(decoded); {
		if n := entityLength(raw[i:]); n > 0 {
			_, size := utf8.DecodeRuneInString(decoded[j:])
			for k := 0; k < size; k++ {
				offsets = append(offsets, i)
			}
			i, j = i+n, j+size
			continue
		}
		switch {
		case i < len(raw) && raw[i] == decoded[j]:
			offsets = append(offsets, i)
			i, j = i+1, j+1
		case i+1 < len(raw) && raw[i] == '\\' && raw[i+1] == decoded[j]:
			offsets = append(offsets, i)
			i, j = i+2, j+1
		default:
			return nil
		}
	}
	return append(offsets, i)
}

// entityLength returns the length of the entity s starts with, such as
// "&amp;" or "&#x2014;", or 0 if it doesn't start with one.
func entityLength(s string) int {
	if len(s) < 3 || s[0] != '&' {
		return 0
	}
	for i := 1; i < len(s) && i < 33; i++ {
		switch c := s[i]; {
		case c == ';':
			if i > 1 {
				return i + 1
			}
			return 0
		case c == '#' && i == 1, c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		default:
			return 0
		}
	}
	return 0
}
