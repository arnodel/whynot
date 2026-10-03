package whynot

// Event is something that happened in a View that an app may react to,
// returned by [Controller.Frame] and [Panel.Frame]: the counterpart of
// the input events going in. Only this package defines kinds of Event,
// so new kinds can be added without breaking anyone: code that doesn't
// know a kind ignores it.
type Event interface {
	isEvent()
}

// LinkClick is a link to elsewhere than the document itself being
// clicked or tapped. Following it, such as loading another document, is
// up to the app.
type LinkClick struct {
	// Destination is the link's destination, as written in the document.
	Destination string
}

// AnchorClick is a link within the document ("#id") being clicked or
// tapped. A Panel scrolls to the anchor by default (see
// [Panel.SetAnchorScrolling]); a Controller only reports it.
type AnchorClick struct {
	// ID is the anchor's id, percent-decoded and without the "#", ready
	// for [View.ScrollToAnchor]. It's empty for a bare "#", which
	// browsers take to mean the top of the document.
	ID string
}

func (LinkClick) isEvent()   {}
func (AnchorClick) isEvent() {}
