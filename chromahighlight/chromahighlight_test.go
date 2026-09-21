package chromahighlight

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"

	"github.com/arnodel/whynot"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		tok  chroma.TokenType
		want whynot.TokenClass
	}{
		{"keyword", chroma.Keyword, whynot.TokenKeyword},
		{"keyword subtype", chroma.KeywordReserved, whynot.TokenKeyword},
		{"string", chroma.LiteralString, whynot.TokenString},
		{"string subtype", chroma.LiteralStringDouble, whynot.TokenString},
		{"number", chroma.LiteralNumber, whynot.TokenNumber},
		{"number subtype", chroma.LiteralNumberInteger, whynot.TokenNumber},
		{"comment", chroma.Comment, whynot.TokenComment},
		{"comment preproc", chroma.CommentPreproc, whynot.TokenComment},
		{"name", chroma.Name, whynot.TokenPlain},
		{"operator", chroma.Operator, whynot.TokenPlain},
		{"punctuation", chroma.Punctuation, whynot.TokenPlain},
		{"text", chroma.Text, whynot.TokenPlain},
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
	if got := classify(chroma.LiteralString); got != whynot.TokenString {
		t.Errorf("classify(LiteralString) = %v, want TokenString", got)
	}
	if got := classify(chroma.LiteralNumber); got != whynot.TokenNumber {
		t.Errorf("classify(LiteralNumber) = %v, want TokenNumber", got)
	}
}

func TestHighlightGo(t *testing.T) {
	code := "// comment\nfunc f(n int) string {\n\treturn \"hi\"\n}\n"
	spans := Highlighter{}.Highlight("go", code)

	var got strings.Builder
	var sawKeyword, sawString, sawComment bool
	for _, span := range spans {
		got.WriteString(span.Text)
		switch span.Class {
		case whynot.TokenKeyword:
			sawKeyword = true
		case whynot.TokenString:
			sawString = true
		case whynot.TokenComment:
			sawComment = true
		}
	}
	if got.String() != code {
		t.Errorf("concatenated spans = %q, want %q", got.String(), code)
	}
	if !sawKeyword {
		t.Errorf("no TokenKeyword span found in %#v", spans)
	}
	if !sawString {
		t.Errorf("no TokenString span found in %#v", spans)
	}
	if !sawComment {
		t.Errorf("no TokenComment span found in %#v", spans)
	}
}

func TestHighlightUnrecognizedLanguage(t *testing.T) {
	code := "some text in a language chroma doesn't know"
	spans := Highlighter{}.Highlight("not-a-real-language", code)
	if len(spans) != 1 {
		t.Fatalf("spans = %#v, want a single span", spans)
	}
	if spans[0].Text != code {
		t.Errorf("spans[0].Text = %q, want %q", spans[0].Text, code)
	}
	if spans[0].Class != whynot.TokenPlain {
		t.Errorf("spans[0].Class = %v, want TokenPlain", spans[0].Class)
	}
}
