// Package markdown is whynot's Markdown compiler: it parses source with
// goldmark and builds the engine's block and inline tree, alongside the
// semantic ast.Node tree they refer to (Compile). Fenced code blocks go
// through the codeblocks.Plugin given for them.
//
// It only builds structure: appearance comes later, from a View's
// StyleSheet, and layout and drawing are the engine's job.
package markdown

import (
	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
)

// Result is a compiled document: its block tree, and what the compiler
// recorded about its top-level structure along the way.
type Result struct {
	// Root holds the top-level blocks.
	Root *engine.StackBlock

	// Headings is every top-level heading, in document order.
	Headings []Heading

	// SoleImages maps each top-level text block consisting of a single
	// image to that image's src.
	SoleImages map[engine.Block]string
}

// Heading is a top-level heading of a compiled document.
type Heading struct {
	ID    string // the heading's anchor id
	Level int    // 1-6
	Text  string
}

// Compile compiles Markdown source into its block tree, with plugins
// (in priority order) parsing fenced code blocks. It only builds
// structure: appearance comes later, from a View's StyleSheet.
func Compile(source []byte, plugins []codeblocks.Plugin) *Result {
	p := parser.New(
		parser.WithExtensions(
			extension.TaskListItemParser,
			extension.StrikethroughParser,
			extension.TableParser,
			// Substitutes straight quotes/dashes/ellipsis for their
			// typographic equivalents ("x" -> "x", -- -> en dash, etc.) as
			// a plain Text node, same as any other inline text - its
			// default substitutions are HTML entities (e.g. "&rsquo;"),
			// but Text.Value's decoder resolves those the same way it
			// already resolves &nbsp;/&amp; in ordinary prose (see
			// TestParseResolvesEntitiesAndEscapes), so no extra config is
			// needed to get literal runes out of it.
			extension.TypographerParser,
		),
		parser.WithAutoHeadingID(),
	)
	node := p.Parse(source)
	c := compiler{source: source, codeBlockPlugins: plugins}
	// A nil parent ast.Node is what marks a block as top-level.
	return &Result{
		Root:       &engine.StackBlock{Blocks: c.compileBlocks(node.FirstChild(), nil)},
		Headings:   c.headings,
		SoleImages: c.soleImages,
	}
}

// compiler compiles Markdown source into a Block/ast.Node tree - structure
// only, no appearance. See Parse.
type compiler struct {
	source []byte

	// codeBlockPlugins is every codeblocks.Plugin registered via
	// WithCodeBlockPlugin, in registration order. pluginCache remembers
	// which of them handle a given language, populated lazily by
	// pluginsFor - so a document with many fences in the same language
	// only calls Handles once per plugin per language, not once per
	// fence.
	codeBlockPlugins []codeblocks.Plugin
	pluginCache      map[string][]codeblocks.Plugin

	// pendingSpace is true when a breakable space has been seen in the
	// source but not yet attached to the next appended Inline item - see
	// appendString. Reset to true at the start of each fresh run of
	// inline content (a paragraph, heading, list item head, or table
	// cell), since there's nothing for the first item there to glue to.
	pendingSpace bool

	// headings and soleImages accumulate the Result fields of the same
	// names as top-level blocks are compiled.
	headings   []Heading
	soleImages map[engine.Block]string
}

// compileBlocks compiles first and its following siblings under parent,
// dropping any that compile to nothing.
func (c *compiler) compileBlocks(first gmast.Node, parent *ast.Node) []engine.Block {
	var blocks []engine.Block
	for node := first; node != nil; node = node.NextSibling() {
		if _, ok := node.(gmast.BlockNode); !ok {
			continue
		}
		if block := c.compileBlock(node, parent); block != nil {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// compileInlines compiles node's children as a fresh run of inline
// content - a paragraph, heading, list item head or table cell.
func (c *compiler) compileInlines(node gmast.Node, astNode *ast.Node) []engine.Inline {
	c.pendingSpace = true
	return c.appendChildren(nil, node, astNode)
}
