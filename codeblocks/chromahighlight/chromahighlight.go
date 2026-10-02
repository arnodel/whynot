// Package chromahighlight implements codeblocks.Highlighter on top of
// github.com/alecthomas/chroma/v2. Split out from the core whynot package
// to keep chroma's dependency weight (~200 embedded language lexer
// definitions) out of the core library's dependency graph - the same
// reasoning ebitenrenderer/systemfont are their own packages.
package chromahighlight

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/arnodel/whynot/codeblocks"
)

// Highlighter implements codeblocks.Highlighter using chroma's lexers.
type Highlighter struct{}

var _ codeblocks.Highlighter = Highlighter{}

// Highlight looks up a lexer for language (falling back to a plain-text
// lexer if none matches, so an unrecognized language degrades to
// unhighlighted code rather than an error), coalesces adjacent
// same-type tokens (so real code doesn't produce one span per character),
// and classifies each token via classify.
func (Highlighter) Highlight(language, code string) []codeblocks.HighlightSpan {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	iter, err := lexer.Tokenise(nil, code)
	if err != nil {
		return []codeblocks.HighlightSpan{{Text: code, Class: codeblocks.TokenPlain}}
	}
	var spans []codeblocks.HighlightSpan
	for token := iter(); token != chroma.EOF; token = iter() {
		spans = append(spans, codeblocks.HighlightSpan{Text: token.Value, Class: classify(token.Type)})
	}
	return spans
}

// classify maps a chroma.TokenType to whynot's coarser TokenClass.
//
// chroma.KeywordType/NameClass and chroma.NameFunction/
// NameFunctionMagic are checked explicitly, ahead of the SubCategory()
// switch below: without that, KeywordType would land in the same bucket
// as a plain Keyword, and NameClass/NameFunction aren't covered by any
// bucket at all. SubCategory(), not Category(), because Category()
// would merge LiteralString and LiteralNumber into one indistinguishable
// bucket.
//
// chroma.NameBuiltin is deliberately left unclassified: some lexers use
// it for both builtin functions and builtin type names, so mapping it
// to either TokenFunction or TokenType would sometimes mislabel the
// other.
//
// A lexer only sees syntax, not real type/binding information, so
// TokenType and TokenFunction can usually recognize a declaration but
// not every later usage of the same name - see their own doc comments.
func classify(t chroma.TokenType) codeblocks.TokenClass {
	switch t {
	case chroma.KeywordType, chroma.NameClass:
		return codeblocks.TokenType
	case chroma.NameFunction, chroma.NameFunctionMagic:
		return codeblocks.TokenFunction
	}
	switch t.SubCategory() {
	case chroma.Keyword:
		return codeblocks.TokenKeyword
	case chroma.LiteralString:
		return codeblocks.TokenString
	case chroma.LiteralNumber:
		return codeblocks.TokenNumber
	case chroma.Comment, chroma.CommentPreproc:
		return codeblocks.TokenComment
	default:
		return codeblocks.TokenPlain
	}
}
