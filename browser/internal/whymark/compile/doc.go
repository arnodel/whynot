// Package compile compiles whymark documents to Lua: a passage file in, a
// Program out. It only compiles: running a program is for whoever takes
// turns with it (see package whymark).
//
// # How a program's chunk is called
//
// The chunk is called with two arguments: the turn, and the host's
// helpers. It runs the document's prelude, and for a document with
// passages, defines the global page(turn), and turn.go and turn.show. A
// document without passages defines page(turn) itself, in its prelude.
// Either may define init(turn).
//
// The turn is a table with the fields state, query, input, passage, note
// and trail (see whymark.md, the format's specification), and the
// functions write, write_headers and link, which the host provides before
// calling the chunk.
//
// The host's helpers are a table of two functions, which the chunk uses
// and documents don't see:
//
//   - started() reports whether the page's header has been written: once
//     it has, turn.go is an error;
//   - heading(text) sets the heading to write before the passage's first
//     output: its title, or "" for a quiet passage.
package compile
