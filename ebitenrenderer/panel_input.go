package ebitenrenderer

import (
	"github.com/arnodel/whynot/input"
)

// Update reads this tick's input and passes it to the Panel's Controller
// - call once per game tick.
func (p *Panel) Update() {
	p.input.Scale = p.scale
	p.apply(p.input.Events())
}

// apply passes events to the Controller.
func (p *Panel) apply(events []input.Event) {
	p.syncController()
	p.controller.Frame(events, p.elapsed())
}

// syncController copies the Panel's link callbacks, which a host may
// reassign at any time, into its Controller.
func (p *Panel) syncController() {
	p.controller.OnLinkClick = p.OnLinkClick
	p.controller.OnLinkHover = p.OnLinkHover
	p.controller.AnchorScrolling = p.anchorScrolling
}
