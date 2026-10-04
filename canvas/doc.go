// Package canvas is the drawing contract between whynot and a graphics
// framework. A View draws a document onto a [Canvas], which a backend
// implements with its framework: whynot's backends for Ebitengine and Gio
// each provide one. A program can also draw its own things on a Canvas,
// such as a toolbar around a document.
//
// A Canvas only has to draw rectangles, text and images, and to clip
// them. The [Canvas] documentation is the specification for implementing
// one: the requirements that need the most care are placing text exactly
// as golang.org/x/image/font places it, and keeping the coordinates when
// clipping. backends/README.md in whynot's repository describes the rest
// of what a backend provides.
//
// Coordinates are integer pixels, in the canvas's own coordinate space,
// which whynot uses throughout: see the Coordinates section of package
// whynot's documentation.
package canvas
