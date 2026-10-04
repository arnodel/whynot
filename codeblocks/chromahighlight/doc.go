// Package chromahighlight is a code-block plugin ([codeblocks.Plugin]) that
// highlights code in every language the chroma library
// (github.com/alecthomas/chroma) knows: a fenced code block's source is
// split into tokens classed as keywords, types, strings and so on, which
// the stylesheet colors.
//
//	doc := whynot.Parse(source, whynot.WithCodeBlockPlugin(chromahighlight.Plugin{}))
//
// It's a package of its own so that programs that don't highlight code
// don't depend on chroma, which bundles lexers for some 200 languages.
package chromahighlight
