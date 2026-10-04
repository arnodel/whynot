// Package images is how whynot gets the images a document shows. A View
// is given a [Source], which turns an image's src (as written in the
// Markdown) into an [AsyncImage]: a key to cache it by, and a function
// that fetches its bytes.
//
// Loading is whynot's job: each image is fetched and decoded at most once
// per View, in the background, so a slow fetch never holds up layout or
// drawing. The Source only says where an image comes from. By default
// that's [FileSource], which opens src as a local file path; the
// whynot.WithImageSource option gives a View another one.
//
// # Writing a Source
//
// This one resolves src against the URL a document came from, and fetches
// it over HTTP:
//
//	type webSource struct{ base *url.URL }
//
//	func (s webSource) Image(src string) (images.AsyncImage, error) {
//		u, err := s.base.Parse(src)
//		if err != nil {
//			return images.AsyncImage{}, err
//		}
//		return images.AsyncImage{Key: u.String(), Fetch: func(ctx context.Context) (io.ReadCloser, error) {
//			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
//			if err != nil {
//				return nil, err
//			}
//			resp, err := http.DefaultClient.Do(req)
//			if err != nil {
//				return nil, err
//			}
//			if resp.StatusCode != http.StatusOK {
//				resp.Body.Close()
//				return nil, fmt.Errorf("%s: %s", u, resp.Status)
//			}
//			return resp.Body, nil
//		}}, nil
//	}
//
// Image is called on every layout, so it should only interpret src; the
// slow work belongs in Fetch, which is called once per key.
package images
