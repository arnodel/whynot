package markdown

import (
	"fmt"
	"testing"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
)

// fakePlugin is a codeblocks.Plugin test double: it handles every
// language, and parse decides what each code block becomes.
type fakePlugin struct {
	parse func(language, code string) codeblocks.Content
}

func (fakePlugin) Handles(string) bool { return true }

func (p fakePlugin) Parse(language, code string) codeblocks.Content {
	return p.parse(language, code)
}

// tokensOf is a fakePlugin returning spans for any code.
func tokensOf(spans ...codeblocks.Span) fakePlugin {
	return fakePlugin{parse: func(string, string) codeblocks.Content {
		return codeblocks.Tokens{Spans: spans}
	}}
}

// wholeCode is a fakePlugin returning the code as one span of class.
func wholeCode(class string) fakePlugin {
	return fakePlugin{parse: func(_, code string) codeblocks.Content {
		return codeblocks.Tokens{Spans: []codeblocks.Span{{Text: code, Class: class}}}
	}}
}

func lineTexts(t *testing.T, line []engine.Inline) []string {
	t.Helper()
	texts := make([]string, len(line))
	for i, part := range line {
		text, ok := part.(*engine.InlineText)
		if !ok {
			t.Fatalf("part %d = %T, want *InlineText", i, part)
		}
		texts[i] = text.Text
	}
	return texts
}

func TestTokenLinesEmpty(t *testing.T) {
	if lines := tokenLines(nil, nil, 0); len(lines) != 0 {
		t.Fatalf("lines = %#v, want empty", lines)
	}
}

func TestTokenLinesMultipleSpansPerLine(t *testing.T) {
	blockNode := (*ast.Node)(nil).AddChild(ast.TagCodeBlock)
	lines := tokenLines([]codeblocks.Span{
		{Text: "func", Class: codeblocks.ClassKeyword},
		{Text: " f() {"},
	}, blockNode, 1)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1", len(lines))
	}
	if got := lineTexts(t, lines[0]); len(got) != 2 || got[0] != "func" || got[1] != " f() {" {
		t.Fatalf("lines[0] texts = %#v", got)
	}
	keywordNode := lines[0][0].(*engine.InlineText).ASTNode
	if keywordNode == blockNode {
		t.Fatalf("keyword span reused the block's own node, want a TagCodeToken child")
	}
	if keywordNode.Tag != ast.TagCodeToken || keywordNode.Class != codeblocks.ClassKeyword {
		t.Fatalf("keyword node = %v %q, want TagCodeToken %q", keywordNode.Tag, keywordNode.Class, codeblocks.ClassKeyword)
	}
	if keywordNode.Parent != blockNode {
		t.Errorf("keyword node's parent = %v, want the block's node", keywordNode.Parent)
	}
	plainNode := lines[0][1].(*engine.InlineText).ASTNode
	if plainNode != blockNode {
		t.Fatalf("unclassified span node = %v, want the block's own node (no child allocated)", plainNode)
	}
}

// TestTokenLinesKeepsUnknownClass checks a class whynot's stylesheets
// don't know is still recorded on the node: classes are open-ended.
func TestTokenLinesKeepsUnknownClass(t *testing.T) {
	blockNode := (*ast.Node)(nil).AddChild(ast.TagCodeBlock)
	lines := tokenLines([]codeblocks.Span{{Text: "@decorator", Class: "decorator"}}, blockNode, 1)
	if node := lines[0][0].(*engine.InlineText).ASTNode; node.Class != "decorator" {
		t.Errorf("class = %q, want %q", node.Class, "decorator")
	}
}

func TestTokenLinesSpanCrossingMultipleLines(t *testing.T) {
	blockNode := (*ast.Node)(nil).AddChild(ast.TagCodeBlock)
	lines := tokenLines([]codeblocks.Span{
		{Text: "/* a\nb */", Class: codeblocks.ClassComment},
		{Text: "\nrest"},
	}, blockNode, 3)
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	for i, want := range []string{"/* a", "b */", "rest"} {
		if got := lineTexts(t, lines[i]); len(got) != 1 || got[0] != want {
			t.Fatalf("lines[%d] texts = %#v, want [%q]", i, got, want)
		}
	}
	for i := 0; i < 2; i++ {
		if node := lines[i][0].(*engine.InlineText).ASTNode; node.Class != codeblocks.ClassComment {
			t.Fatalf("lines[%d] class = %q, want %q", i, node.Class, codeblocks.ClassComment)
		}
	}
	if lines[2][0].(*engine.InlineText).ASTNode != blockNode {
		t.Fatalf("lines[2] node = %v, want the block's own node", lines[2][0].(*engine.InlineText).ASTNode)
	}
}

// TestTokenLinesBlankLineGetsAPart is a regression test: a blank source
// line contributes no span text, which previously left its parts slice
// empty - LineBox indexes parts[0] unconditionally, so this crashed
// rendering. Every line must end up with at least one part.
func TestTokenLinesBlankLineGetsAPart(t *testing.T) {
	blockNode := (*ast.Node)(nil).AddChild(ast.TagCodeBlock)
	lines := tokenLines([]codeblocks.Span{{Text: "func f() {\n\n}\n"}}, blockNode, 3)
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3: %#v", len(lines), lines)
	}
	if len(lines[1]) != 1 {
		t.Fatalf("blank line parts = %#v, want exactly 1 part", lines[1])
	}
	if got := lines[1][0].(*engine.InlineText).Text; got != "" {
		t.Errorf("blank line text = %q, want empty", got)
	}
}

func TestTokenLinesMismatchedLineCountFails(t *testing.T) {
	blockNode := (*ast.Node)(nil).AddChild(ast.TagCodeBlock)
	// One line's worth of text for a two-line block: a misbehaving plugin.
	if lines := tokenLines([]codeblocks.Span{{Text: "just one line"}}, blockNode, 2); lines != nil {
		t.Fatalf("lines = %#v, want nil (fallback signal)", lines)
	}
}

// codeBlockOf runs the compiler's code block logic over rawLines with
// plugins.
func codeBlockOf(rawLines []string, plugins ...codeblocks.Plugin) (engine.Block, *ast.Node) {
	astNode := (*ast.Node)(nil).AddChild(ast.TagCodeBlock)
	ps := make([]plugin, len(plugins))
	for i, p := range plugins {
		ps[i] = plugin{Plugin: p, namespace: fmt.Sprintf("#%d:", i+1)}
	}
	return (&compiler{}).codeBlock(astNode, "go", rawLines, ps), astNode
}

// TestCodeBlockPassesSourceVerbatim is a regression test: rawLines each
// already end in their own "\n" (see blocks.go's KindCodeBlock case),
// so they must reach the plugin concatenated verbatim, with no extra
// "\n" between them.
func TestCodeBlockPassesSourceVerbatim(t *testing.T) {
	var got string
	p := fakePlugin{parse: func(_, code string) codeblocks.Content {
		got = code
		return nil
	}}
	codeBlockOf([]string{"line one\n", "line two\n"}, p)
	if want := "line one\nline two\n"; got != want {
		t.Errorf("plugin got %q, want %q", got, want)
	}
}

func TestCodeBlockNoPluginIsPlain(t *testing.T) {
	block, node := codeBlockOf([]string{"a\n", "b\n"})
	cb, ok := block.(*engine.CodeBlock)
	if !ok || len(cb.Lines) != 2 {
		t.Fatalf("block = %#v, want a 2-line *CodeBlock", block)
	}
	if cb.Lines[0][0].(*engine.InlineText).ASTNode != node {
		t.Error("plain text should use the block's own node")
	}
}

func TestCodeBlockTokens(t *testing.T) {
	block, _ := codeBlockOf([]string{"x := 1\n"}, wholeCode(codeblocks.ClassNumber))
	cb := block.(*engine.CodeBlock)
	if node := cb.Lines[0][0].(*engine.InlineText).ASTNode; node.Class != codeblocks.ClassNumber {
		t.Errorf("class = %q, want %q", node.Class, codeblocks.ClassNumber)
	}
}

// TestCodeBlockSkipsDecliningAndBadPlugins checks a plugin returning nil,
// or tokens that don't reproduce the source, hands the block to the next.
func TestCodeBlockSkipsDecliningAndBadPlugins(t *testing.T) {
	declining := fakePlugin{parse: func(string, string) codeblocks.Content { return nil }}
	bad := tokensOf(codeblocks.Span{Text: "not the source"})
	block, _ := codeBlockOf([]string{"a\n", "b\n"}, declining, bad, wholeCode(codeblocks.ClassString))
	cb := block.(*engine.CodeBlock)
	if node := cb.Lines[0][0].(*engine.InlineText).ASTNode; node.Class != codeblocks.ClassString {
		t.Errorf("class = %q, want %q (from the third plugin)", node.Class, codeblocks.ClassString)
	}
}

// TestCodeBlockImageFallsBackToNextPlugin checks an Image becomes a
// diagram whose fallback is what the next plugin makes of the block.
func TestCodeBlockImageFallsBackToNextPlugin(t *testing.T) {
	diagram := fakePlugin{parse: func(string, string) codeblocks.Content {
		return codeblocks.Image{Source: keySource("diagram")}
	}}
	block, _ := codeBlockOf([]string{"graph\n"}, diagram, wholeCode(codeblocks.ClassKeyword))
	d, ok := block.(*engine.DiagramBlock)
	if !ok {
		t.Fatalf("block = %T, want *DiagramBlock", block)
	}
	if want := "#1:diagram"; d.Image.Key() != want {
		t.Errorf("image key = %q, want %q", d.Image.Key(), want)
	}
	fallback, ok := d.Fallback.(*engine.CodeBlock)
	if !ok {
		t.Fatalf("fallback = %T, want *CodeBlock", d.Fallback)
	}
	if node := fallback.Lines[0][0].(*engine.InlineText).ASTNode; node.Class != codeblocks.ClassKeyword {
		t.Errorf("fallback class = %q, want the next plugin's %q", node.Class, codeblocks.ClassKeyword)
	}
}
