package giorenderer

import (
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/arnodel/whynot/input"
)

// Update reads this frame's pointer input and passes it to the Panel's
// Controller - call once per frame, before Draw. Gio routes events by
// area, so a press on the scrollbar's own region (added after this
// Panel's own area - see Draw) never reaches here.
func (p *Panel) Update(gtx layout.Context) {
	p.controller.OnLinkClick = p.OnLinkClick
	p.controller.OnLinkHover = p.OnLinkHover
	p.controller.AnchorScrolling = p.anchorScrolling

	events := p.input.Source(gtx, p, p.bounds).Events()
	p.controller.Frame(events, p.elapsed())
	// Gio only produces frames when something happens, so keep asking
	// for them while there's a fling or a fading scrollbar to animate.
	if p.controller.Animating() {
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.OnPress != nil && pressed(events) {
		p.OnPress()
	}
}

// pressed reports whether events include a press: a mouse button or a
// touch.
func pressed(events []input.Event) bool {
	for _, e := range events {
		switch e := e.(type) {
		case input.PointerButton:
			if e.Down {
				return true
			}
		case input.TouchStart:
			return true
		}
	}
	return false
}
