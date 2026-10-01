// Package imagecache loads the images a document shows through an
// images.Source: an asynchronous cache that fetches and decodes each
// image once, and decoding of animated GIFs.
package imagecache

import (
	"bytes"
	"image"
	_ "image/jpeg" // registers the JPEG format with image.Decode
	_ "image/png"  // registers the PNG format with image.Decode
	"io"
	"sync"
	"time"

	"github.com/arnodel/whynot/images"
)

// Status is an image's state within a Cache.
type Status int

const (
	// Pending means a fetch is in flight (or about to start) - not
	// yet ready, not (yet, or any longer) failed. Bounds may already be
	// known even in this state - see Cache's own doc comment.
	Pending Status = iota
	// Ready means Image holds the fully decoded result.
	Ready
	// Failed means the most recent attempt failed - Err says how.
	// Not necessarily permanent: see Cache's retry behavior.
	Failed
)

// Result is what Cache.Load returns for one src.
type Result struct {
	Status Status
	// Bounds is the image's native pixel size - known once Status ==
	// Ready, but may already be known earlier, while Status is
	// still Pending (see Cache's header-peek). Zero while
	// genuinely unknown.
	Bounds image.Rectangle
	// Image is the decoded image - valid only when Status == Ready
	// and this isn't an animated GIF (Animation is set instead; exactly
	// one of the two is set on a ready result).
	Image image.Image
	// Animation is set instead of Image when the decoded image is an
	// animated GIF.
	Animation *Animation
	// Err says why the most recent attempt failed - valid only when
	// Status == Failed.
	Err error
}

// Change is one entry Cache.ChangedSince reports.
type Change struct {
	Src string
	// BoundsRevealed is true if, since the mark passed to
	// ChangedSince, this src's bounds became known for the first time
	// - the one transition that can change a slot's height
	// unpredictably (everything else - becoming ready, a retry's
	// outcome - happens at an already-known, already-laid-out size).
	BoundsRevealed bool
}

// defaultRetryDelay is how long Cache waits before treating
// a failed image as worth trying again - long enough that a
// persistently broken or slow src isn't retried on every layout
// rebuild (a resize, a zoom change, even just hovering a different
// link elsewhere in the document), short enough that a transient
// failure (a network blip, a momentarily-down server) doesn't stay
// broken for the rest of the session.
const defaultRetryDelay = 10 * time.Second

// Cache is the "management of image data" the library owns: an image
// is fetched and decoded at most once, however many times Load is
// called for it afterwards - every layout rebuild asks again. A Canvas implementation never
// fetches its own copy either - ImageBox carries the already-decoded
// image.Image forward from here, straight into Canvas.DrawImage.
//
// Load never blocks: a cache miss (or a failed entry whose retry delay
// has elapsed) returns Pending immediately and starts a fetch on
// a background goroutine, which is why this type is safe for
// concurrent use (Load from the layout goroutine, the goroutine's own
// writes as a fetch progresses) - unlike everything else in this
// codebase, which is only ever touched from ebiten's single game-loop
// goroutine and has no need to be.
type Cache struct {
	source     images.Source
	retryDelay time.Duration

	mu      sync.Mutex
	cache   map[string]*cacheEntry
	version uint64
}

type cacheEntry struct {
	status Status
	bounds image.Rectangle
	img    image.Image
	anim   *Animation
	err    error

	lastAttempt time.Time

	// changedAt/boundsRevealedAt are the cache's own version counter at
	// the moment this entry last changed / last had its bounds become
	// known for the first time - see ChangedSince.
	changedAt        uint64
	boundsRevealedAt uint64
}

// NewCache returns a Cache loading the srcs passed to Load through
// source.
func NewCache(source images.Source) *Cache {
	return &Cache{
		source:     source,
		retryDelay: defaultRetryDelay,
		cache:      map[string]*cacheEntry{},
	}
}

// Load resolves src through the Source and returns its current state,
// starting a fetch in the background on a genuine miss, or on a failed
// entry old enough to retry - never blocking on the fetch itself.
// resolved is the AsyncImage's Key, or src itself when the Source
// failed - InlineImage's fallback text names it in a missing- or
// broken-image message.
func (c *Cache) Load(src string) (resolved string, result Result) {
	img, err := c.source.Image(src)
	if err != nil {
		return src, Result{Status: Failed, Err: err}
	}
	return img.Key, c.LoadImage(img)
}

// LoadImage is Load for an image that needs no resolving - e.g. a code
// block plugin's diagram, whose fetch might be a POST rather than a GET.
func (c *Cache) LoadImage(img images.AsyncImage) Result {
	key := img.Key
	c.mu.Lock()
	entry, ok := c.cache[key]
	start := !ok || (entry.status == Failed && time.Since(entry.lastAttempt) > c.retryDelay)
	if start {
		entry = &cacheEntry{status: Pending, lastAttempt: time.Now()}
		c.cache[key] = entry
	}
	result := Result{Status: entry.status, Bounds: entry.bounds, Image: entry.img, Animation: entry.anim, Err: entry.err}
	c.mu.Unlock()

	if start {
		go c.fetchAndDecode(key, img.Fetch)
	}
	return result
}

// ChangedSince returns every change since a previous ChangedSince (or
// an initial call with mark 0) returned mark, plus a new mark to pass
// next time. Cheap: each entry just remembers the cache's own version
// counter at the moment it last changed, so this is a scan of however
// many distinct images exist, not a growing log.
func (c *Cache) ChangedSince(mark uint64) (changed []Change, newMark uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for resolved, entry := range c.cache {
		if entry.changedAt > mark {
			changed = append(changed, Change{
				Src:            resolved,
				BoundsRevealed: entry.boundsRevealedAt > mark,
			})
		}
	}
	return changed, c.version
}

// fetchAndDecode does the actual (potentially slow) work, entirely off
// the caller's goroutine, via fetch. It
// probes dimensions via image.DecodeConfig first, through a TeeReader
// that mirrors whatever bytes it reads into header - PNG/GIF/JPEG all
// put their size near the front of the file, so this is normally a
// small prefix of the stream, not the whole body - and reports them
// immediately (setBounds), before the rest of the fetch/decode
// completes. The full decode then continues from exactly where the
// header-peek left off (header's buffered bytes, then whatever's left
// of rc), so nothing is re-fetched or re-read from the start. A GIF
// (DecodeConfig's own format name, already read to get here) decodes
// every frame via decodeAnimatedGIF instead of image.Decode's
// single-frame result.
func (c *Cache) fetchAndDecode(key string, fetch func() (io.ReadCloser, error)) {
	rc, err := fetch()
	if err != nil {
		c.setFailed(key, err)
		return
	}
	defer rc.Close()

	var header bytes.Buffer
	format := ""
	if cfg, fmt, cfgErr := image.DecodeConfig(io.TeeReader(rc, &header)); cfgErr == nil {
		format = fmt
		c.setBounds(key, image.Rectangle{Max: image.Pt(cfg.Width, cfg.Height)})
	}

	full := io.MultiReader(bytes.NewReader(header.Bytes()), rc)
	if format == "gif" {
		anim, decErr := decodeAnimatedGIF(full)
		if decErr != nil {
			c.setFailed(key, decErr)
			return
		}
		c.setReady(key, nil, anim)
		return
	}
	img, _, decErr := image.Decode(full)
	if decErr != nil {
		c.setFailed(key, decErr)
		return
	}
	c.setReady(key, img, nil)
}

func (c *Cache) setBounds(resolved string, bounds image.Rectangle) {
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

// setReady stores a decoded result - exactly one of img/anim is
// non-nil, matching Result's own Image/Animation split.
func (c *Cache) setReady(resolved string, img image.Image, anim *Animation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.cache[resolved]
	if entry == nil {
		return
	}
	c.version++
	entry.changedAt = c.version
	entry.status = Ready
	entry.img = img
	entry.anim = anim
	entry.err = nil
	switch {
	case img != nil:
		c.recordBoundsLocked(entry, img.Bounds())
	case anim != nil:
		c.recordBoundsLocked(entry, anim.Bounds())
	}
}

func (c *Cache) setFailed(resolved string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.cache[resolved]
	if entry == nil {
		return
	}
	c.version++
	entry.changedAt = c.version
	entry.status = Failed
	entry.img = nil
	entry.anim = nil
	entry.err = err
}

// recordBoundsLocked sets entry.bounds, and - the first time it
// transitions from unknown (the zero Rectangle) to known - records the
// version this happened at, so ChangedSince can report BoundsRevealed
// even if the entry changes further (e.g. becomes ready) before the
// caller checks again. Callers must hold c.mu and have already set
// c.version/entry.changedAt for this update.
func (c *Cache) recordBoundsLocked(entry *cacheEntry, bounds image.Rectangle) {
	wasUnknown := entry.bounds == (image.Rectangle{})
	entry.bounds = bounds
	if wasUnknown && bounds != (image.Rectangle{}) {
		entry.boundsRevealedAt = c.version
	}
}
