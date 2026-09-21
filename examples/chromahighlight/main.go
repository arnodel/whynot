// Command chromahighlight is a runnable example of whynot.WithHighlighter
// backed by chromahighlight.Highlighter (github.com/alecthomas/chroma/v2).
// Fenced code blocks in a recognized language render token-by-token in the
// active StyleSheet's SyntaxColors palette; an unrecognized language (or a
// plain indented block, which has no fence to name one) falls back to the
// same flat CodeBlockColor rendering as without a highlighter at all.
package main

import (
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/chromahighlight"
	"github.com/arnodel/whynot/ebitenrenderer"
)

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# Syntax highlighting example\n\n")
	b.WriteString("Fenced code blocks below are colored token-by-token via\n")
	b.WriteString("`chromahighlight.Highlighter`, keyword/type/function/string/number/comment\n")
	b.WriteString("each in its own color from the active `StyleSheet`'s `SyntaxColors`.\n\n")

	b.WriteString("## Go\n\n```go\n")
	b.WriteString("// Sum adds up a slice of ints, logging as it goes.\n")
	b.WriteString("func Sum(nums []int) int {\n")
	b.WriteString("\ttotal := 0\n")
	b.WriteString("\tfor _, n := range nums {\n")
	b.WriteString("\t\ttotal += n // running total\n")
	b.WriteString("\t}\n")
	b.WriteString("\tlog.Printf(\"total: %d\", total)\n")
	b.WriteString("\treturn total\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("## Python\n\n```python\n")
	b.WriteString("# Sum adds up a list of ints, logging as it goes.\n")
	b.WriteString("def total(nums):\n")
	b.WriteString("    print(\"summing\", len(nums), \"numbers\")\n")
	b.WriteString("    result = 0\n")
	b.WriteString("    for n in nums:\n")
	b.WriteString("        result += n  # running total\n")
	b.WriteString("    return result\n```\n\n")

	b.WriteString("## Type names\n\n")
	b.WriteString("A builtin type (Go's `int`) and a declared custom type/class (Python's\n")
	b.WriteString("`Point`) share one color - see chromahighlight.classify's doc comment for\n")
	b.WriteString("why a custom type's later *usages*, not just its declaration, aren't\n")
	b.WriteString("always recognizable this way (a lexer limitation, not whynot's own):\n\n")
	b.WriteString("```python\n")
	b.WriteString("class Point:\n")
	b.WriteString("    def __init__(self, x: int, y: int) -> None:\n")
	b.WriteString("        self.x = x\n")
	b.WriteString("        self.y = y\n```\n\n")

	b.WriteString("## Unrecognized language\n\n")
	b.WriteString("This fence names a made-up language chroma won't recognize, so it\n")
	b.WriteString("falls back to plain, unhighlighted rendering:\n\n")
	b.WriteString("```not-a-real-language\n")
	b.WriteString("some text here, all one flat color\n```\n\n")

	b.WriteString("## Indented code block\n\n")
	b.WriteString("Indented blocks have no fence, so no language at all - also plain:\n\n")
	b.WriteString("    plain indented code, one flat color\n")
	return b.String()
}

type game struct {
	view     *whynot.View
	renderer *ebitenrenderer.Renderer
	start    time.Time
}

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.view.Scroll(dy * 2)
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.view.Draw(g.renderer.NewCanvas(screen), 0, 0)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	width := int(float64(outsideWidth) * scale)
	height := int(float64(outsideHeight) * scale)
	g.view.Layout(width, height, scale, time.Since(g.start))
	return width, height
}

func main() {
	g := &game{
		view: whynot.NewView(
			[]byte(exampleDoc()),
			whynot.NewGoFontFaceSelector(72),
			whynot.WithHighlighter(chromahighlight.Highlighter{}),
		),
		renderer: ebitenrenderer.New(),
		start:    time.Now(),
	}
	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("whynot syntax highlighting example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
