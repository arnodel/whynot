package simpletheme

import (
	"image/color"
	"reflect"
	"testing"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/internal/styling"
)

// TestPresetsMatchEngineDefaults checks that converting a preset to a
// StyleSheet reproduces the engine's own default styles exactly - no field
// lost or mapped wrong in either direction.
func TestPresetsMatchEngineDefaults(t *testing.T) {
	for name, c := range map[string]struct {
		theme *Theme
		want  *styling.Basic
	}{
		"dark":  {Dark(), styling.Dark()},
		"light": {Light(), styling.Light()},
	} {
		got := c.theme.StyleSheet().Styles()
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: StyleSheet() = %+v, want %+v", name, got, c.want)
		}
	}
}

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
	if p.Style != font.StyleItalic || p.Weight != font.WeightThin || p.Family != styling.Monospace {
		t.Errorf("got style %v, weight %v, family %v; want italic, thin, monospace", p.Style, p.Weight, p.Family)
	}
	p = TextStyle{Style: StyleNormal, Weight: WeightBlack, Family: Proportional}.partial()
	if p.Style != font.StyleNormal || p.Weight != font.WeightBlack || p.Family != styling.Proportional {
		t.Errorf("got style %v, weight %v, family %v; want normal, black, proportional", p.Style, p.Weight, p.Family)
	}
}

func TestBaseTextStyleZeroTakesDefault(t *testing.T) {
	theme := Dark()
	theme.BaseTextStyle = TextStyle{Family: Monospace}
	got := theme.StyleSheet().Styles().TextStyle(nil)
	want := styling.Dark().BaseTextStyle
	want.Family = styling.Monospace
	if got.TextStyle != want || got.Set != styling.AllTextStyleFields {
		t.Errorf("root text style = %+v, want %+v with every field set", got, want)
	}
}

func TestStyleSheetIsASnapshot(t *testing.T) {
	theme := Dark()
	sheet := theme.StyleSheet()
	theme.LinkColor = color.RGBA{1, 2, 3, 255}
	if got := sheet.Styles().Color(&styling.Node{Tag: styling.TagLink}); got == theme.LinkColor {
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
