// Package codeblocks is the contract between whynot's Markdown compiler
// and code-block plugins, which change how fenced code blocks are shown.
// A [Plugin] says which languages it handles, and parses each such
// block's source into [Content]: [Tokens], spans of the source classified
// for syntax coloring, or an [Image], such as a rendered diagram.
//
// Plugins are given to whynot.Parse with the whynot.WithCodeBlockPlugin
// option, in priority order. The first plugin that handles a block's
// language parses it. If it makes an Image, what the next plugin makes of
// the block is shown while the image loads, or if it fails.
//
// Ready-made plugins are in subpackages: chromahighlight highlights code
// in every language the chroma library knows, and kroki turns Mermaid
// diagrams into images.
//
// # Writing a plugin
//
// A plugin produces data; whynot turns it into the document and colors
// it. This one shows the comment lines of shell code blocks in the
// stylesheet's comment color:
//
//	type shellComments struct{}
//
//	func (shellComments) Handles(language string) bool {
//		return language == "sh"
//	}
//
//	func (shellComments) Parse(language, code string) codeblocks.Content {
//		var spans []codeblocks.Span
//		for _, line := range strings.SplitAfter(code, "\n") {
//			class := "" // the code block's own color
//			if strings.HasPrefix(strings.TrimSpace(line), "#") {
//				class = codeblocks.ClassComment
//			}
//			spans = append(spans, codeblocks.Span{Text: line, Class: class})
//		}
//		return codeblocks.Tokens{Spans: spans}
//	}
//
// The spans must add up to the block's source exactly. A class is any
// string: a stylesheet colors the ones it knows, such as [ClassComment],
// and shows the others in the code block's own color.
package codeblocks
