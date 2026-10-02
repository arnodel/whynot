package chromahighlight

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"

	"github.com/arnodel/whynot/codeblocks"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		tok  chroma.TokenType
		want string
	}{
		{"keyword", chroma.Keyword, codeblocks.ClassKeyword},
		{"keyword subtype", chroma.KeywordReserved, codeblocks.ClassKeyword},
		{"keyword type (builtin type)", chroma.KeywordType, codeblocks.ClassType},
		{"name class (declared type/class)", chroma.NameClass, codeblocks.ClassType},
		{"name function (declared/called function)", chroma.NameFunction, codeblocks.ClassFunction},
		{"name function magic (dunder method)", chroma.NameFunctionMagic, codeblocks.ClassFunction},
		{"string", chroma.LiteralString, codeblocks.ClassString},
		{"string subtype", chroma.LiteralStringDouble, codeblocks.ClassString},
		{"number", chroma.LiteralNumber, codeblocks.ClassNumber},
		{"number subtype", chroma.LiteralNumberInteger, codeblocks.ClassNumber},
		{"comment", chroma.Comment, codeblocks.ClassComment},
		{"comment preproc", chroma.CommentPreproc, codeblocks.ClassComment},
		{"name", chroma.Name, ""},
		{"name builtin (shared between builtin functions and, in some lexers, types)", chroma.NameBuiltin, ""},
		{"operator", chroma.Operator, ""},
		{"punctuation", chroma.Punctuation, ""},
		{"text", chroma.Text, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.tok); got != tc.want {
				t.Errorf("classify(%v) = %v, want %v", tc.tok, got, tc.want)
			}
		})
	}
}

// TestClassifyDistinguishesStringFromNumber is the specific regression
// this package's design depends on getting right: SubCategory(), not
// Category(), because Category() collapses LiteralString and
// LiteralNumber into one indistinguishable Literal bucket.
func TestClassifyDistinguishesStringFromNumber(t *testing.T) {
	if got := classify(chroma.LiteralString); got != codeblocks.ClassString {
		t.Errorf("classify(LiteralString) = %v, want ClassString", got)
	}
	if got := classify(chroma.LiteralNumber); got != codeblocks.ClassNumber {
		t.Errorf("classify(LiteralNumber) = %v, want ClassNumber", got)
	}
}

// TestClassifyDistinguishesTypeFromKeyword checks that a builtin type
// name (chroma.KeywordType) gets its own ClassType color rather than
// falling into the same bucket as a plain chroma.Keyword.
func TestClassifyDistinguishesTypeFromKeyword(t *testing.T) {
	if got := classify(chroma.KeywordType); got != codeblocks.ClassType {
		t.Errorf("classify(KeywordType) = %v, want ClassType", got)
	}
	if got := classify(chroma.Keyword); got != codeblocks.ClassKeyword {
		t.Errorf("classify(Keyword) = %v, want ClassKeyword", got)
	}
}

// TestClassifyDistinguishesFunctionFromPlain checks that
// chroma.NameFunction gets its own ClassFunction color rather than
// silently falling through to no class (it has no SubCategory()
// bucket of its own).
func TestClassifyDistinguishesFunctionFromPlain(t *testing.T) {
	if got := classify(chroma.NameFunction); got != codeblocks.ClassFunction {
		t.Errorf("classify(NameFunction) = %v, want ClassFunction", got)
	}
	if got := classify(chroma.Name); got != "" {
		t.Errorf("classify(Name) = %v, want no class", got)
	}
}

func TestParseGo(t *testing.T) {
	code := "// comment\nfunc f(n int) string {\n\treturn \"hi\"\n}\n"
	tokens, ok := Plugin{}.Parse("go", code).(codeblocks.Tokens)
	if !ok {
		t.Fatalf("Parse = %T, want codeblocks.Tokens", Plugin{}.Parse("go", code))
	}
	spans := tokens.Spans

	var got strings.Builder
	var sawKeyword, sawType, sawFunction, sawString, sawComment bool
	for _, span := range spans {
		got.WriteString(span.Text)
		switch span.Class {
		case codeblocks.ClassKeyword:
			sawKeyword = true
		case codeblocks.ClassType:
			sawType = true
		case codeblocks.ClassFunction:
			sawFunction = true
		case codeblocks.ClassString:
			sawString = true
		case codeblocks.ClassComment:
			sawComment = true
		}
	}
	if got.String() != code {
		t.Errorf("concatenated spans = %q, want %q", got.String(), code)
	}
	if !sawKeyword {
		t.Errorf("no ClassKeyword span found in %#v", spans)
	}
	// n's declared type (int) and f's return type (string) are both
	// builtin types - chroma's Go lexer tags them chroma.KeywordType.
	if !sawType {
		t.Errorf("no ClassType span found in %#v", spans)
	}
	// f's own declared name - chroma's Go lexer tags it
	// chroma.NameFunction.
	if !sawFunction {
		t.Errorf("no ClassFunction span found in %#v", spans)
	}
	if !sawString {
		t.Errorf("no ClassString span found in %#v", spans)
	}
	if !sawComment {
		t.Errorf("no ClassComment span found in %#v", spans)
	}
}

func TestHandles(t *testing.T) {
	if !(Plugin{}).Handles("go") {
		t.Error(`Handles("go") = false, want true`)
	}
	if (Plugin{}).Handles("not-a-real-language") {
		t.Error(`Handles("not-a-real-language") = true, want false`)
	}
}
