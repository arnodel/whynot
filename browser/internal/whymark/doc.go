// Package whymark is the engine of whymark documents: Markdown documents
// that run, written as passages with Lua in them (see whymark.md, at the
// root of the browser module, for the format).
//
// The engine and whoever shows its pages talk in turns. A turn is a
// Request (a location, the page it comes from, what was typed), and the
// page it gets: a Header, then Markdown. An Engine takes turns; a user
// agent reads a page with ReadHeader, and makes the request for a link on
// it with Follow.
//
// The work is in three layers. Package compile turns a document into a
// Program, a value that knows nothing of turns. Running a program, for one
// turn in a fresh sandbox, is this package's run. The Engine handles
// requests: it fetches documents, compiles them (once per content), finds
// the snapshot a request comes from, runs the program, and writes the
// page. None of it knows anything of rendering.
package whymark
