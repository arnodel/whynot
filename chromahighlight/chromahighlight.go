// Package chromahighlight implements whynot.Highlighter on top of
// github.com/alecthomas/chroma/v2. Split out from the core whynot package
// to keep chroma's dependency weight (~200 embedded language lexer
// definitions) out of the core library's dependency graph - the same
// reasoning ebitenrenderer/systemfont are their own packages.
package chromahighlight

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/arnodel/whynot"
)

// Highlighter implements whynot.Highlighter using chroma's lexers.
type Highlighter struct{}

var _ whynot.Highlighter = Highlighter{}

// Highlight looks up a lexer for language (falling back to a plain-text
// lexer if none matches, so an unrecognized language degrades to
// unhighlighted code rather than an error), coalesces adjacent
// same-type tokens (so real code doesn't produce one span per character),
// and classifies each token via classify.
func (Highlighter) Highlight(language, code string) []whynot.HighlightSpan {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	iter, err := lexer.Tokenise(nil, code)
	if err != nil {
		return []whynot.HighlightSpan{{Text: code, Class: whynot.TokenPlain}}
	}
	var spans []whynot.HighlightSpan
	for token := iter(); token != chroma.EOF; token = iter() {
		spans = append(spans, whynot.HighlightSpan{Text: token.Value, Class: classify(token.Type)})
	}
	return spans
}

// classify collapses chroma's ~50 TokenTypes down to whynot's small
// TokenClass set via TokenType.SubCategory() - NOT Category(), which
// would merge LiteralString and LiteralNumber into one indistinguishable
// "Literal" bucket (confirmed from chroma's own source: Category() is the
// 1000-wide bucket, SubCategory() the 100-wide one LiteralString/
// LiteralNumber actually differ at).
//
// chroma.KeywordType and chroma.NameClass are checked explicitly, ahead
// of the SubCategory() switch, specifically to pull them out of that
// bucketing: KeywordType's SubCategory() would otherwise land in the
// same 1000-wide bucket as a plain chroma.Keyword (KeywordType is 1006,
// Keyword 1000 - SubCategory() floors both to 1000), merging a builtin
// type name (Go's int/string/bool) into the ordinary keyword color
// instead of TokenType. chroma.NameBuiltin is deliberately NOT included
// here even though some lexers use it for builtin types too (e.g.
// Python tags an "int" annotation as NameBuiltin, not KeywordType) -
// NameBuiltin is shared with builtin *functions* (Python's len, print),
// so including it would mislabel those as types too.
//
// This still can't recognize every type usage: a lexer has no real type
// information, only syntax, so a *declared* custom type/class name
// (chroma.NameClass - e.g. Python's "class Point") is recognizable, but
// a plain *usage* of that same name generally isn't - confirmed against
// chroma's own Go lexer, which tags a custom type's every occurrence
// (declaration and usage alike) as chroma.NameOther, indistinguishable
// from any other identifier. That usage-site gap is a limitation of the
// underlying lexer, not something classify can work around.
func classify(t chroma.TokenType) whynot.TokenClass {
	switch t {
	case chroma.KeywordType, chroma.NameClass:
		return whynot.TokenType
	}
	switch t.SubCategory() {
	case chroma.Keyword:
		return whynot.TokenKeyword
	case chroma.LiteralString:
		return whynot.TokenString
	case chroma.LiteralNumber:
		return whynot.TokenNumber
	case chroma.Comment, chroma.CommentPreproc:
		return whynot.TokenComment
	default:
		return whynot.TokenPlain
	}
}
