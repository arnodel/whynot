package whynot

import (
	"bytes"
	"image"
	_ "image/gif"  // registers the GIF format with image.Decode
	_ "image/jpeg" // registers the JPEG format with image.Decode
	_ "image/png"  // registers the PNG format with image.Decode
	"io"
	"os"
	"sync"
	"time"
)

// ImageSource resolves and fetches an image's raw bytes - the
// embedder-supplied policy for wherever an InlineImage's src actually
// points (relative to a document's own location, over http(s), from
// an archive, whatever it needs). Kept out of the library's own hands
// the same way document loading and link resolution already are (see
// cmd/whynot/main.go's loadDocument/resolveLink) - only the seam lives
// here. Always used through an ImageCache (see NewImageCache), never
// called directly by GetInlineLayout, so resolving and fetching an
// image only ever happens once no matter how many times it's asked
// for - and always from a background goroutine, since Open can block
// (a slow disk, a slow or hanging server) and ImageCache.Load must
// never block its caller.
type ImageSource interface {
	// Resolve turns src into a stable, absolute identifier - the cache
	// key, and what a missing- or broken-image's fallback message
	// names. Pure and cheap: no I/O, just interpreting src against
	// whatever base the ImageSource knows about.
	Resolve(src string) (string, error)
	// Open fetches the bytes at a value Resolve returned.
	Open(resolved string) (io.ReadCloser, error)
}

// FileImageSource is the default ImageSource: src is opened exactly as
// written (via the OS's own working-directory-relative resolution),
// with no further resolution - today's pre-existing behavior, used
// unless an embedder opts into WithImageSource.
type FileImageSource struct{}

func (FileImageSource) Resolve(src string) (string, error) { return src, nil }

func (FileImageSource) Open(resolved string) (io.ReadCloser, error) {
	return os.Open(resolved)
}

// ImageStatus is an image's state within an ImageCache.
type ImageStatus int

const (
	// ImagePending means a fetch is in flight (or about to start) - not
	// yet ready, not (yet, or any longer) failed. Bounds may already be
	// known even in this state - see ImageCache's own doc comment.
	ImagePending ImageStatus = iota
	// ImageReady means Image holds the fully decoded result.
	ImageReady
	// ImageFailed means the most recent attempt failed - Err says how.
	// Not necessarily permanent: see ImageCache's retry behavior.
	ImageFailed
)

// ImageResult is what ImageCache.Load returns for one src.
type ImageResult struct {
	Status ImageStatus
	// Bounds is the image's native pixel size - known once Status ==
	// ImageReady, but may already be known earlier, while Status is
	// still ImagePending (see ImageCache's header-peek). Zero while
	// genuinely unknown.
	Bounds image.Rectangle
	// Image is the decoded image - valid only when Status == ImageReady.
	Image image.Image
	// Err says why the most recent attempt failed - valid only when
	// Status == ImageFailed.
	Err error
}

// ImageChange is one entry ImageCache.ChangedSince reports.
type ImageChange struct {
	Src string
	// BoundsRevealed is true if, since the mark passed to
	// ChangedSince, this src's bounds became known for the first time
	// - the one transition that can change a slot's height
	// unpredictably (everything else - becoming ready, a retry's
	// outcome - happens at an already-known, already-laid-out size).
	// See View.invalidateChangedImages.
	BoundsRevealed bool
}

// defaultImageRetryDelay is how long ImageCache waits before treating
// a failed image as worth trying again - long enough that a
// persistently broken or slow src isn't retried on every layout
// rebuild (a resize, a zoom change, even just hovering a different
// link elsewhere in the document), short enough that a transient
// failure (a network blip, a momentarily-down server) doesn't stay
// broken for the rest of the session.
const defaultImageRetryDelay = 10 * time.Second

// ImageCache wraps an ImageSource with the "management of image data"
// the library owns: an image is resolved, fetched, and decoded at
// most once, however many times Load is called for it afterwards -
// every layout rebuild asks again. A Canvas implementation never
// fetches its own copy either - ImageBox carries the already-decoded
// image.Image forward from here, straight into Canvas.DrawImage.
//
// Load never blocks: a cache miss (or a failed entry whose retry delay
// has elapsed) returns ImagePending immediately and starts a fetch on
// a background goroutine, which is why this type is safe for
// concurrent use (Load from the layout goroutine, the goroutine's own
// writes as a fetch progresses) - unlike everything else in this
// codebase, which is only ever touched from ebiten's single game-loop
// goroutine and has no need to be.
type ImageCache struct {
	source     ImageSource
	retryDelay time.Duration

	mu      sync.Mutex
	cache   map[string]*imageCacheEntry
	version uint64
}

type imageCacheEntry struct {
	status ImageStatus
	bounds image.Rectangle
	img    image.Image
	err    error

	lastAttempt time.Time

	// changedAt/boundsRevealedAt are the cache's own version counter at
	// the moment this entry last changed / last had its bounds become
	// known for the first time - see ChangedSince.
	changedAt        uint64
	boundsRevealedAt uint64
}

func NewImageCache(source ImageSource) *ImageCache {
	return &ImageCache{
		source:     source,
		retryDelay: defaultImageRetryDelay,
		cache:      map[string]*imageCacheEntry{},
	}
}

// Load resolves src and returns its current state, starting a fetch in
// the background on a genuine miss, or on a failed entry old enough to
// retry - never blocking on the fetch itself. resolved is still
// returned when Resolve itself failed - InlineImage's fallback text
// names it in a missing- or broken-image message.
func (c *ImageCache) Load(src string) (resolved string, result ImageResult) {
	resolved, err := c.source.Resolve(src)
	if err != nil {
		return resolved, ImageResult{Status: ImageFailed, Err: err}
	}

	c.mu.Lock()
	entry, ok := c.cache[resolved]
	start := !ok || (entry.status == ImageFailed && time.Since(entry.lastAttempt) > c.retryDelay)
	if start {
		entry = &imageCacheEntry{status: ImagePending, lastAttempt: time.Now()}
		c.cache[resolved] = entry
	}
	result = ImageResult{Status: entry.status, Bounds: entry.bounds, Image: entry.img, Err: entry.err}
	c.mu.Unlock()

	if start {
		go c.fetchAndDecode(resolved)
	}
	return resolved, result
}

// ChangedSince returns every change since a previous ChangedSince (or
// an initial call with mark 0) returned mark, plus a new mark to pass
// next time. Cheap: each entry just remembers the cache's own version
// counter at the moment it last changed, so this is a scan of however
// many distinct images exist, not a growing log.
func (c *ImageCache) ChangedSince(mark uint64) (changed []ImageChange, newMark uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for resolved, entry := range c.cache {
		if entry.changedAt > mark {
			changed = append(changed, ImageChange{
				Src:            resolved,
				BoundsRevealed: entry.boundsRevealedAt > mark,
			})
		}
	}
	return changed, c.version
}

// fetchAndDecode does the actual (potentially slow) work, entirely off
// the caller's goroutine. It probes dimensions via image.DecodeConfig
// first, through a TeeReader that mirrors whatever bytes it reads into
// header - PNG/GIF/JPEG all put their size near the front of the file,
// so this is normally a small prefix of the stream, not the whole
// body - and reports them immediately (setBounds), before the rest of
// the fetch/decode completes. The full decode then continues from
// exactly where the header-peek left off (header's buffered bytes,
// then whatever's left of rc), so nothing is re-fetched or re-read
// from the start.
func (c *ImageCache) fetchAndDecode(resolved string) {
	rc, err := c.source.Open(resolved)
	if err != nil {
		c.setResult(resolved, ImageFailed, nil, err)
		return
	}
	defer rc.Close()

	var header bytes.Buffer
	if cfg, _, cfgErr := image.DecodeConfig(io.TeeReader(rc, &header)); cfgErr == nil {
		c.setBounds(resolved, image.Rectangle{Max: image.Pt(cfg.Width, cfg.Height)})
	}

	full := io.MultiReader(bytes.NewReader(header.Bytes()), rc)
	img, _, decErr := image.Decode(full)
	if decErr != nil {
		c.setResult(resolved, ImageFailed, nil, decErr)
		return
	}
	c.setResult(resolved, ImageReady, img, nil)
}

func (c *ImageCache) setBounds(resolved string, bounds image.Rectangle) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.cache[resolved]
	if entry == nil {
		return
	}
	c.version++
	entry.changedAt = c.version
	c.recordBoundsLocked(entry, bounds)
}

func (c *ImageCache) setResult(resolved string, status ImageStatus, img image.Image, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.cache[resolved]
	if entry == nil {
		return
	}
	c.version++
	entry.changedAt = c.version
	entry.status = status
	entry.img = img
	entry.err = err
	if img != nil {
		c.recordBoundsLocked(entry, img.Bounds())
	}
}

// recordBoundsLocked sets entry.bounds, and - the first time it
// transitions from unknown (the zero Rectangle) to known - records the
// version this happened at, so ChangedSince can report BoundsRevealed
// even if the entry changes further (e.g. becomes ready) before the
// caller checks again. Callers must hold c.mu and have already set
// c.version/entry.changedAt for this update.
func (c *ImageCache) recordBoundsLocked(entry *imageCacheEntry, bounds image.Rectangle) {
	wasUnknown := entry.bounds == (image.Rectangle{})
	entry.bounds = bounds
	if wasUnknown && bounds != (image.Rectangle{}) {
		entry.boundsRevealedAt = c.version
	}
}
