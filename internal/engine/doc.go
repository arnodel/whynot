// Package engine is the pipeline from a document's blocks to pixels: it
// turns the block and inline tree the compiler builds into layout boxes,
// and draws and hit-tests them on a canvas.Canvas.
//
// It covers the block and inline types with their layouts, line layout,
// the rendering context (Context), the lazily laid-out top level
// (StackBox, which answers position queries but keeps no position of
// its own), sideways-scrolling blocks (ScrollBox) and diagram blocks. It
// depends only on contracts (ast, styling, fonts, images, canvas) and
// the image cache, never on themes, the compiler or the public whynot
// package.
//
// State is not its job: the scroll position, sideways offsets, hover and
// scrollbar timing belong to whynot.View, which hands drawing what it
// needs through Context (the ScrollOffset and Scrollbar hooks). Neither is parsing Markdown,
// input handling, or the public API.
package engine
