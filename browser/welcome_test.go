package browser

import (
	"bytes"
	"testing"
)

func TestRenderWelcomeAddressBarTip(t *testing.T) {
	old := AddressBarEditable
	defer func() { AddressBarEditable = old }()

	AddressBarEditable = false
	md := renderWelcome()
	if bytes.Contains(md, []byte("{{ADDRESS_BAR_TIP}}")) {
		t.Error("renderWelcome() with AddressBarEditable=false left the placeholder unsubstituted")
	}
	if bytes.Contains(md, []byte("address bar")) {
		t.Error("renderWelcome() with AddressBarEditable=false mentions the address bar, want no mention")
	}

	AddressBarEditable = true
	md = renderWelcome()
	if bytes.Contains(md, []byte("{{ADDRESS_BAR_TIP}}")) {
		t.Error("renderWelcome() with AddressBarEditable=true left the placeholder unsubstituted")
	}
	if !bytes.Contains(md, []byte("Click the address bar")) {
		t.Error("renderWelcome() with AddressBarEditable=true doesn't mention the address bar")
	}
	// The substitution must not corrupt the rest of the page - the
	// closing "Paste \"welcome\"..." line should still follow, on its
	// own paragraph, in either case.
	if !bytes.Contains(md, []byte("Paste \"welcome\" at any time")) {
		t.Error("renderWelcome() with AddressBarEditable=true lost the closing paragraph after the substitution")
	}
}
