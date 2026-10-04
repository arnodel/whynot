// Package markdown is whynot's Markdown compiler: it parses source with
// goldmark and builds the engine's block and inline tree, alongside the
// semantic ast.Node tree they refer to (Compile). Fenced code blocks go
// through the codeblocks.Plugin given for them, and images are resolved
// to their fetch.Source.
//
// It only builds structure: appearance comes later, from a View's
// StyleSheet, and layout and drawing are the engine's job.
package markdown

import (
	"fmt"

	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/fetch"
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
	// image to that image's Source, if it was resolved.
	SoleImages map[engine.Block]fetch.Source
}

// Heading is a top-level heading of a compiled document.
type Heading struct {
	ID    string // the heading's anchor id
	Level int    // 1-6
	Text  string
}

// Options configure Compile.
type Options struct {
	// Plugins parse fenced code blocks, in priority order.
	Plugins []codeblocks.Plugin

	// ResolveImage returns the Source of an image's src. If nil, no image
	// is resolved.
	ResolveImage func(src string) (fetch.Source, error)
}

// Compile compiles Markdown source into its block tree.
func Compile(source []byte, opts Options) *Result {
	p := parser.New(
		parser.WithExtensions(
			extension.TaskListItemParser,
			extension.StrikethroughParser,
			extension.TableParser,
			// Smart quotes, dashes and ellipses. It substitutes HTML
			// entities, which Text.Value decodes to runes like any other.
			extension.TypographerParser,
		),
		parser.WithAutoHeadingID(),
	)
	node := p.Parse(source)
	c := compiler{source: source, resolveImage: opts.ResolveImage}
	for i, p := range opts.Plugins {
		// "#" can't appear in a URL scheme, so these never clash with the
		// keys a fetch.Registry makes.
		c.codeBlockPlugins = append(c.codeBlockPlugins, plugin{Plugin: p, namespace: fmt.Sprintf("#%d:", i+1)})
	}
	// A nil parent ast.Node is what marks a block as top-level.
	return &Result{
		Root:       &engine.StackBlock{Blocks: c.compileBlocks(node.FirstChild(), nil)},
		Headings:   c.headings,
		SoleImages: c.soleImages,
	}
}

// compiler is the state of one Compile call.
type compiler struct {
	source []byte

	// codeBlockPlugins are the code-block plugins, in priority order;
	// pluginCache records which of them handle each language seen so
	// far (see pluginsFor).
	codeBlockPlugins []plugin
	pluginCache      map[string][]plugin

	resolveImage func(src string) (fetch.Source, error)

	// pendingSpace is whether breakable whitespace has been seen since
	// the last Inline was appended, so the next one isn't Glued to it
	// (see appendString). It starts out true for each run of inline
	// content: the first item has nothing to glue to.
	pendingSpace bool

	// headings and soleImages accumulate the Result fields of the same
	// names as top-level blocks are compiled.
	headings   []Heading
	soleImages map[engine.Block]fetch.Source
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
