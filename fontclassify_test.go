package whynot

import (
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

func TestClassifySubfamily(t *testing.T) {
	cases := []struct {
		name       string
		wantWeight font.Weight
		wantStyle  font.Style
		wantOK     bool
	}{
		{"", font.WeightNormal, font.StyleNormal, true},
		{"Regular", font.WeightNormal, font.StyleNormal, true},
		{"Roman", font.WeightNormal, font.StyleNormal, true},
		{"Book", font.WeightNormal, font.StyleNormal, true},
		{"Normal", font.WeightNormal, font.StyleNormal, true},
		{"Text", font.WeightNormal, font.StyleNormal, true},
		{"Plain", font.WeightNormal, font.StyleNormal, true},
		{"  Bold  ", font.WeightBold, font.StyleNormal, true},
		{"Medium", font.WeightMedium, font.StyleNormal, true},
		{"Italic", font.WeightNormal, font.StyleItalic, true},
		{"Oblique", font.WeightNormal, font.StyleItalic, true},
		{"Bold Italic", font.WeightBold, font.StyleItalic, true},
		{"BoldItalic", font.WeightBold, font.StyleItalic, true},
		{"Bold-Italic", font.WeightBold, font.StyleItalic, true},
		{"BoldOblique", font.WeightBold, font.StyleItalic, true},
		{"Medium Italic", font.WeightMedium, font.StyleItalic, true},
		{"Regular Italic", font.WeightNormal, font.StyleItalic, true},
		{"Light", 0, 0, false},
		{"Thin", 0, 0, false},
		{"Black", 0, 0, false},
		{"Heavy", 0, 0, false},
		{"SemiBold", 0, 0, false},
		{"Semi Bold", 0, 0, false},
		{"ExtraBold", 0, 0, false},
		{"Condensed", 0, 0, false},
		{"Condensed Bold", 0, 0, false},
		{"Light Italic", 0, 0, false},
	}
	for _, c := range cases {
		weight, style, ok := classifySubfamily(c.name)
		if ok != c.wantOK {
			t.Errorf("classifySubfamily(%q) ok = %v, want %v", c.name, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if weight != c.wantWeight || style != c.wantStyle {
			t.Errorf("classifySubfamily(%q) = (%v, %v), want (%v, %v)", c.name, weight, style, c.wantWeight, c.wantStyle)
		}
	}
}

func TestSplitStyleTokens(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"BoldItalic", []string{"bold", "italic"}},
		{"Bold Italic", []string{"bold", "italic"}},
		{"Go-Bold_Italic", []string{"go", "bold", "italic"}},
		{"", nil},
	}
	for _, c := range cases {
		got := splitStyleTokens(c.name)
		if len(got) != len(c.want) {
			t.Errorf("splitStyleTokens(%q) = %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitStyleTokens(%q) = %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

func TestNormalizeFontName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Arial MT", "arialmt"},
		{"ArialMT", "arialmt"},
		{"Arial-Bold_Italic", "arialbolditalic"},
	}
	for _, c := range cases {
		if got := normalizeFontName(c.name); got != c.want {
			t.Errorf("normalizeFontName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSubfontBelongsToFamily(t *testing.T) {
	collection, err := opentype.ParseCollection(goregular.TTF)
	if err != nil {
		t.Fatalf("ParseCollection: %v", err)
	}
	sub, err := collection.Font(0)
	if err != nil {
		t.Fatalf("Font(0): %v", err)
	}
	var buf sfnt.Buffer
	cases := []struct {
		want string
		ok   bool
	}{
		{"Go", true},
		{"go", true},
		{"Go Regular", true},
		{"Arial", false},
		{"", true},
	}
	for _, c := range cases {
		if got := subfontBelongsToFamily(sub, &buf, c.want); got != c.ok {
			t.Errorf("subfontBelongsToFamily(..., %q) = %v, want %v", c.want, got, c.ok)
		}
	}
}
