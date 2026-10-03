package simpletheme

import (
	"image/color"
	"testing"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/styling"
)

func TestTextStyleZeroInherits(t *testing.T) {
	p := TextStyle{Weight: WeightBold}.partial()
	if p.Set != styling.FieldWeight {
		t.Errorf("Set = %v, want only FieldWeight", p.Set)
	}
	if p.Weight != font.WeightBold {
		t.Errorf("Weight = %v, want font.WeightBold", p.Weight)
	}
	if got := (TextStyle{}).partial(); got.Set != 0 {
		t.Errorf("zero TextStyle sets %v, want nothing", got.Set)
	}
}

func TestTextStyleEnumsMap(t *testing.T) {
	p := TextStyle{Style: StyleItalic, Weight: WeightThin, Family: Monospace}.partial()
	if p.Style != font.StyleItalic || p.Weight != font.WeightThin || p.Family != fonts.Monospace {
		t.Errorf("got style %v, weight %v, family %v; want italic, thin, monospace", p.Style, p.Weight, p.Family)
	}
	p = TextStyle{Style: StyleNormal, Weight: WeightBlack, Family: Proportional}.partial()
	if p.Style != font.StyleNormal || p.Weight != font.WeightBlack || p.Family != fonts.Proportional {
		t.Errorf("got style %v, weight %v, family %v; want normal, black, proportional", p.Style, p.Weight, p.Family)
	}
}

func TestBaseTextStyleZeroTakesDefault(t *testing.T) {
	theme := Dark()
	theme.BaseTextStyle = TextStyle{Family: Monospace}
	got := theme.StyleSheet().Styles().TextStyle(nil)
	want := defaultBaseTextStyle
	want.Family = fonts.Monospace
	if got.TextStyle != want || got.Set != styling.AllTextStyleFields {
		t.Errorf("root text style = %+v, want %+v with every field set", got, want)
	}
}

func TestStyleSheetIsASnapshot(t *testing.T) {
	theme := Dark()
	sheet := theme.StyleSheet()
	theme.LinkColor = color.RGBA{1, 2, 3, 255}
	if got := sheet.Styles().Color(&ast.Node{Tag: ast.TagLink}); got == theme.LinkColor {
		t.Error("changing the theme changed a StyleSheet taken before")
	}
}

func TestLineHeight(t *testing.T) {
	theme := Dark()
	theme.LineHeight = 1.5
	if got := theme.StyleSheet().Styles().LineHeight(nil); got != 1.5 {
		t.Errorf("LineHeight = %v, want 1.5", got)
	}
}

// TestLightColors checks that Light changes the colors that must be
// readable on a light background, rather than being Dark under another
// name.
func TestLightColors(t *testing.T) {
	dark, light := Dark(), Light()
	for _, c := range []struct {
		name        string
		dark, light any
	}{
		{"BackgroundColor", dark.BackgroundColor, light.BackgroundColor},
		{"TextColor", dark.TextColor, light.TextColor},
		{"LinkColor", dark.LinkColor, light.LinkColor},
		{"CodeBlockColor", dark.CodeBlockColor, light.CodeBlockColor},
		{"CodeSpanColor", dark.CodeSpanColor, light.CodeSpanColor},
		{"Scrollbar", dark.Scrollbar, light.Scrollbar},
		{"SyntaxColors", dark.SyntaxColors, light.SyntaxColors},
	} {
		if c.dark == c.light {
			t.Errorf("light %s = dark's (%v), want one readable on a light background", c.name, c.light)
		}
	}
}

// TestLightSharesNonColorValues checks that only colors differ between the
// presets.
func TestLightSharesNonColorValues(t *testing.T) {
	dark, light := Dark(), Light()
	if light.ParagraphMargins != dark.ParagraphMargins {
		t.Errorf("ParagraphMargins = %+v, want dark's %+v", light.ParagraphMargins, dark.ParagraphMargins)
	}
	if light.HeadingTextStyles != dark.HeadingTextStyles {
		t.Errorf("HeadingTextStyles = %+v, want dark's %+v", light.HeadingTextStyles, dark.HeadingTextStyles)
	}
	if light.LineHeight != dark.LineHeight {
		t.Errorf("LineHeight = %v, want dark's %v", light.LineHeight, dark.LineHeight)
	}
}
