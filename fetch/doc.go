// Package fetch is how whynot gets content: the images a document
// shows, and, for a program that wants to, the documents themselves.
//
// A [Source] is where some content comes from: a key to cache it by, and
// a way to fetch its bytes. A [Registry] turns a reference, such as an
// image's src as written in the Markdown, into a Source: it resolves the
// reference against a base URL, as a web browser does, and hands the
// result to the [Resolver] for its URL scheme.
//
// # Opting in
//
// Nothing is fetched unless you allow it: a Registry only resolves the
// schemes of the Resolvers it's made with. This one allows local files
// beneath a directory, and https:
//
//	root, err := os.OpenRoot("docs")
//	if err != nil {
//		log.Fatal(err)
//	}
//	registry := fetch.NewRegistry(
//		fetch.FileResolver{Root: root},
//		fetch.HTTPResolver{},
//	)
//
// Give it to whynot.Parse, with the URL relative srcs are relative to:
//
//	doc := whynot.Parse(markdown,
//		whynot.WithBaseURL(location),
//		whynot.WithImageRegistry(registry),
//	)
//
// A document parsed without a registry shows no images, only their
// alternative text.
//
// # Loading documents
//
// A program can use a Registry to load documents too, from links or an
// address bar. A Source reports the media type it knows, from a file's
// extension or an HTTP response's Content-Type, so the program can check
// it got Markdown, and not, say, a web page:
//
//	src, err := registry.Resolve(base, link)
//	if err != nil {
//		return err
//	}
//	body, mediaType, err := src.Fetch(ctx)
//
// When the media type is "", [net/http.DetectContentType] can guess it
// from the first bytes.
//
// # Writing a Resolver
//
// A Resolver handles one or more URL schemes. This one serves images
// embedded in the program, under an app: scheme, as in ![logo](app:logo.png):
//
//	//go:embed images
//	var embedded embed.FS
//
//	type appResolver struct{}
//
//	func (appResolver) Schemes() []string { return []string{"app"} }
//
//	func (appResolver) Resolve(u *url.URL) (fetch.Source, error) {
//		name := u.Opaque // app:logo.png
//		if name == "" {
//			name = u.Path // app:/logo.png, or a src relative to an app: document
//		}
//		return appSource(path.Join("images", name)), nil
//	}
//
//	type appSource string
//
//	func (s appSource) Key() string { return string(s) }
//
//	func (s appSource) Fetch(context.Context) (io.ReadCloser, string, error) {
//		f, err := embedded.Open(string(s))
//		return f, "", err
//	}
//
// Resolve should only interpret the URL; the slow work belongs in Fetch,
// which whynot calls in the background, once per key.
package fetch
