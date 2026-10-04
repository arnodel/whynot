// Package kroki is a code-block plugin that shows diagrams as images:
// a fenced code block in a diagram language is sent to Kroki
// (https://kroki.io), a service that renders diagrams, and shown as the
// picture it returns. Mermaid is supported.
//
// The image loads in the background, so a View needs a fallback to show
// meanwhile, or if Kroki can't be reached: the next plugin given to
// whynot.Parse that handles the language, such as a syntax highlighter,
// or else the plain code:
//
//	doc := whynot.Parse(source,
//		whynot.WithCodeBlockPlugin(kroki.Plugin{}),
//		whynot.WithCodeBlockPlugin(chromahighlight.Plugin{}),
//	)
//
// Diagrams are sent to kroki.io's public service by default; set a
// [Plugin]'s BaseURL to use your own Kroki instance instead.
package kroki
