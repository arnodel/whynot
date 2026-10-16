package browser

import (
	"net/url"

	"github.com/arnodel/whynot/browser/internal/whymark"
	"github.com/arnodel/whynot/browser/internal/whymark/ai"
)

// EnableAI gives whymark documents declaring the AI capability an AI to
// ask, through provider, whose models are named by tier in models. Local
// documents may use it, and remote ones too if remote is set: the reader
// pays for what they ask. Call it before the first document is opened.
func (a *App) EnableAI(provider ai.Provider, models map[string]string, remote bool) {
	capability := &ai.AI{Provider: provider, Models: models, Default: "balanced"}
	a.engineWith(
		whymark.WithCapabilities(capability),
		whymark.WithGrant(func(document *url.URL, name string) bool {
			return remote || isLocal(document)
		}),
	)
}

// isLocal reports whether location is a local document's: a file, or one
// built into the browser.
func isLocal(location *url.URL) bool {
	return location.Scheme == "file" || location.Scheme == WelcomeURL.Scheme
}
