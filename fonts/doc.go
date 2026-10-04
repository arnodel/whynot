// Package fonts is how whynot gets the fonts it draws text with. A View is
// given a [FaceSelector], which turns a [TextStyle] (a size, a weight, a
// style and a [Family]) into a font face, at the resolution the View
// draws at. The stylesheet decides which style each piece of text gets;
// the FaceSelector decides which font draws it.
//
// The package provides two selectors:
//
//   - [GoSelector] serves the Go fonts, bundled with golang.org/x/image.
//   - [CustomSelector] serves font files you register, for whichever
//     families, weights and styles you choose, and falls back to another
//     selector, the Go fonts by default, for the rest.
//
// The systemfont subpackage finds fonts installed on the system, by name.
// Any other source of fonts can implement FaceSelector itself.
//
// # Using your own font
//
// Register a font file for the slots it covers, here regular
// proportional text; bold, italic and monospace text still use the Go
// fonts:
//
//	selector := fonts.NewCustomSelector()
//	err := selector.AddFontFile(fonts.Proportional, font.WeightNormal, font.StyleNormal, "myfont.ttf", 0)
//	if err != nil {
//		log.Fatal(err)
//	}
//	view := whynot.NewView(doc, selector, simpletheme.DarkStyleSheet)
//
// The last argument picks a font within a collection file (.ttc or .otc);
// it's 0 for an ordinary font file.
package fonts
