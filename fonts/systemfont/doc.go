// Package systemfont is a [fonts.FaceSelector] for the fonts installed on
// the system. A [Selector] finds a font by name, such as "Arial" or
// "Helvetica Neue" ([Selector.RegisterSystemFont]), or picks the
// platform's usual interface font for you ([Selector.RegisterPreferredFont]).
// Whatever it doesn't find is drawn with the bundled Go fonts.
//
//	selector := systemfont.New()
//	if err := selector.RegisterPreferredFont(fonts.Proportional); err != nil {
//		log.Print(err) // not fatal: the Go fonts are used instead
//	}
//	view := whynot.NewView(doc, selector, simpletheme.DarkStyleSheet)
//
// [New] scans the system's font directories, so make one Selector when
// the program starts, rather than one per document.
//
// It's a package of its own because the library it finds fonts with
// doesn't build for the web: a program that also runs in a browser uses
// it only in its native build.
package systemfont
