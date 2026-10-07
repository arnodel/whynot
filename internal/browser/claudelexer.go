package browser

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Syntax highlighting for a passage file's claude and noclaude blocks,
// wherever whynot shows one, such as in a passage file's source: their
// code spans as Lua, and a claude block's prose, the prompt, which the
// reader never sees, as a comment.
func init() {
	lexers.Register(passageBlockLexer("Claude prompt", "claude", chroma.Comment))
	lexers.Register(passageBlockLexer("Claude fallback", "noclaude", chroma.Text))
}

// passageBlockLexer returns a lexer for passage text: prose, of the given
// type, with code spans that run highlighted as Lua.
func passageBlockLexer(name, alias string, prose chroma.TokenType) chroma.Lexer {
	lua := lexers.Get("lua")
	return chroma.MustNewLexer(&chroma.Config{Name: name, Aliases: []string{alias}}, func() chroma.Rules {
		return chroma.Rules{
			"root": {
				{Pattern: "``+[^`]*``+", Type: chroma.LiteralStringBacktick},
				{Pattern: "(`)([^`\n]+)(`)", Type: chroma.ByGroups(chroma.Punctuation, chroma.UsingLexer(lua), chroma.Punctuation)},
				{Pattern: "[^`]+", Type: prose},
				{Pattern: "`", Type: prose},
			},
		}
	})
}
