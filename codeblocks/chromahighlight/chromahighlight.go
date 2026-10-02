// Package chromahighlight is a codeblocks.Plugin for syntax highlighting,
// on top of github.com/alecthomas/chroma/v2. Split out from the core
// whynot package to keep chroma's dependency weight (~200 embedded
// language lexer definitions) out of the core library's dependency graph
// - the same reasoning ebitenrenderer/systemfont are their own packages.
package chromahighlight

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/arnodel/whynot/codeblocks"
)

// Plugin is a codeblocks.Plugin that classifies the tokens of code
// blocks in any language chroma has a lexer for.
type Plugin struct{}

var _ codeblocks.Plugin = Plugin{}

// Handles reports whether chroma has a lexer for language.
func (Plugin) Handles(language string) bool {
	return lexers.Get(language) != nil
}

// Parse tokenises code with language's lexer, coalescing adjacent
// same-type tokens (so real code doesn't produce one span per
// character), and classifies each token via classify. It declines (nil)
// if the lexer fails.
func (Plugin) Parse(language, code string) codeblocks.Content {
	lexer := lexers.Get(language)
	if lexer == nil {
		return nil
	}
	iter, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return nil
	}
	var spans []codeblocks.Span
	for token := iter(); token != chroma.EOF; token = iter() {
		spans = append(spans, codeblocks.Span{Text: token.Value, Class: classify(token.Type)})
	}
	return codeblocks.Tokens{Spans: spans}
}

// classify maps a chroma.TokenType to one of codeblocks' conventional,
// coarser classes, or "" for none.
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
// to either ClassFunction or ClassType would sometimes mislabel the
// other.
//
// A lexer only sees syntax, not real type/binding information, so
// ClassType and ClassFunction can usually recognize a declaration but
// not every later usage of the same name - see their own doc comments.
func classify(t chroma.TokenType) string {
	switch t {
	case chroma.KeywordType, chroma.NameClass:
		return codeblocks.ClassType
	case chroma.NameFunction, chroma.NameFunctionMagic:
		return codeblocks.ClassFunction
	}
	switch t.SubCategory() {
	case chroma.Keyword:
		return codeblocks.ClassKeyword
	case chroma.LiteralString:
		return codeblocks.ClassString
	case chroma.LiteralNumber:
		return codeblocks.ClassNumber
	case chroma.Comment, chroma.CommentPreproc:
		return codeblocks.ClassComment
	default:
		return ""
	}
}
