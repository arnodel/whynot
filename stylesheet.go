package whynot

import "github.com/arnodel/whynot/internal/styling"

// StyleSheet is how a document looks: an immutable, opaque value, made by
// the packages under styles/ (e.g. styles/simpletheme's DarkStyleSheet).
// A View asked for a color reads it from its StyleSheet (see
// [View.HighlightColor], [View.ScrollbarColor]).
type StyleSheet interface {
	// Styles is for whynot's own use. Its result type is internal, so a
	// StyleSheet can only be made by packages in this module.
	Styles() styling.Styles
}
