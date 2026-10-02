// Package engine lays out and draws a document: it turns the block tree
// the compiler builds into layout boxes, draws them onto a canvas.Canvas,
// and holds the viewport state a View scrolls through.
//
// It covers the block and inline types with their layouts, line layout,
// the rendering context (Context), the document stack and its scroll
// cursor, sideways-scrolling blocks (ScrollBox and HScrollState) and
// diagram blocks. It depends only on contracts (ast, styling, fonts,
// images, canvas) and the image cache, never on themes, the compiler or
// the public whynot package.
//
// Parsing Markdown is not its job, and neither is input handling or the
// public API: those stay with the compiler and whynot.View.
package engine
