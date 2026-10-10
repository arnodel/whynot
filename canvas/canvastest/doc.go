// Package canvastest provides a [canvas.Canvas] for tests: a [Recorder]
// records what's drawn on it instead of drawing, so that a test can check
// what was drawn where without a rendering backend. For example, to check
// what a [whynot.View] shows:
//
//	r := &canvastest.Recorder{Area: view.Bounds()}
//	view.Draw(r, 0)
//	for _, text := range r.Texts {
//		fmt.Println(text.S, text.X, text.Y)
//	}
//
// [whynot.View]: https://pkg.go.dev/github.com/arnodel/whynot#View
package canvastest
