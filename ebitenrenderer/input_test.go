package ebitenrenderer

import (
	"reflect"
	"testing"

	"github.com/arnodel/whynot/input"
)

// TestInputMouseEvents checks Input reports what changed between ticks.
func TestInputMouseEvents(t *testing.T) {
	var in Input
	steps := []struct {
		name string
		st   mouseInput
		want []input.Event
	}{
		{"first tick reports the pointer", mouseInput{x: 10, y: 20},
			[]input.Event{input.PointerMove{X: 10, Y: 20}}},
		{"no change reports nothing", mouseInput{x: 10, y: 20}, nil},
		{"press", mouseInput{x: 10, y: 20, down: true, mods: input.ModShift},
			[]input.Event{input.PointerButton{X: 10, Y: 20, Button: input.ButtonPrimary, Down: true, Mods: input.ModShift}}},
		{"move while pressed", mouseInput{x: 15, y: 20, down: true},
			[]input.Event{input.PointerMove{X: 15, Y: 20}}},
		{"release", mouseInput{x: 15, y: 20},
			[]input.Event{input.PointerButton{X: 15, Y: 20, Button: input.ButtonPrimary}}},
		// ebiten.Wheel is positive up; Wheel events are positive down,
		// in pixels: -1 * wheelPixels * scale.
		{"wheel down", mouseInput{x: 15, y: 20, wheelY: -1, scale: 2},
			[]input.Event{input.Wheel{X: 15, Y: 20, DY: wheelPixels * 2}}},
	}
	for _, s := range steps {
		if got := in.mouseEvents(s.st); !reflect.DeepEqual(got, s.want) {
			t.Errorf("%s: events = %#v, want %#v", s.name, got, s.want)
		}
	}
}
