// Package simpletheme makes stylesheets ([StyleSheet]) from a fixed
// set of fields: margins, text styles and colors for each kind of
// Markdown element, plus the scrollbars and the colors of highlighted
// code. It's one way to style whynot: the core has no look of its own.
//
// Use a ready-made stylesheet, [DarkStyleSheet] or [LightStyleSheet]. A
// whynot View uses DarkStyleSheet unless it's given another:
//
//	view := whynot.NewView(doc, whynot.WithStyleSheet(simpletheme.LightStyleSheet))
//
// or start from one of the themes, [Dark] or [Light], and change what you
// need:
//
//	theme := simpletheme.Dark()
//	theme.LinkColor = color.RGBA{0xFF, 0x40, 0x40, 0xFF}
//	view.SetStyleSheet(theme.StyleSheet())
//
// [Theme.StyleSheet] takes a snapshot: changing the [Theme] afterwards
// doesn't change the stylesheets already made from it.
//
// A [TextStyle] only sets its non-zero fields; the others are inherited
// from the enclosing element, so a theme's strong text, for instance,
// only needs to set its weight. Sizes, margins and other dimensions are in
// logical pixels: a View multiplies them by its scale and zoom.
package simpletheme
