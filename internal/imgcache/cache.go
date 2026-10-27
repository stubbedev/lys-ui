// Package imgcache caches decoded raster images and the resampled
// rasters widget.Image paints from, under one shared LRU budget. It is
// the raster-image counterpart of internal/icons: same shape (a keyed
// map with sticky decode failures behind a mutex), but entries evict
// least-recently-used under an entry cap and a byte cap, because media
// widgets swap album art and avatars far more often than themes move.
package imgcache

import (
	"container/list"
	"fmt"
	"image"
	"image/draw"
	"sync"

	"github.com/stubbedev/lys-lib/render"
	memory "github.com/stubbedev/memory-go"
)

// DefaultEntryLimit is the process-wide entry budget. A bar holds a
// handful of album covers and avatars at one or two sizes, so the
// default is generous without being unbounded.
const DefaultEntryLimit = 512

// DefaultByteLimit is the byte half of the default budget: 64 MiB of
// decoded/resampled pixels.
const DefaultByteLimit = 64 << 20

// RasterKey identifies one resampled raster: the source it was derived
// from (and which frame of an animated source), the source rectangle
// that was sampled, and the device-pixel size it was resampled into.
// The device scale is implied by W and H: identical keys want
// identical pixels. InvalidateSource drops every frame of a source.
type RasterKey struct {
	Source string
	Frame  int
	Src    image.Rectangle
	W, H   int
}

// entryKey is a cache key: a string for decoded sources, a RasterKey
// for resampled rasters. Both are comparable, so one map holds both.
type entryKey = any

type entry struct {
	key   entryKey
	img   image.Image // decoded source, or the resampled still image
	err   error       // sticky decode failure (sources only)
	bytes int
	el    *list.Element // the LRU list node; front is most recent
	// mem holds a resampled raster's pixels off the Go heap, in the
	// cache's pixels; empty for a source.
	mem memory.Slice[byte]
}

// Cache memoizes decoded sources and resampled rasters under one LRU
// budget. It is safe for concurrent use: the decode goroutine of an
// async image load consults it off the loop goroutine, while paints
// read it on the loop.
//
// Failures stick: a source that failed to decode keeps its error until
// InvalidateSource, Reset, or eviction drops it, so a corrupt file
// costs one decode attempt, not one per frame.
type Cache struct {
	mu    sync.Mutex
	items map[entryKey]*entry
	lru   *list.List // of *entry, front = most recently used

	// pixels holds the resampled rasters off the Go heap: the cache
	// owns them outright (a widget draws one per paint and keeps
	// none), so eviction hands their pages back rather than leaving
	// them to raise the GC's heap goal.
	pixels memory.Shared
	// forget drops an evicted raster's GPU texture (SetForget).
	forget func(render.ImageID)

	bytes      int
	maxEntries int
	maxBytes   int
	hits       int
	misses     int
}

// New returns a cache with the default limits.
func New() *Cache {
	return newWithLimits(DefaultEntryLimit, DefaultByteLimit)
}

func newWithLimits(maxEntries, maxBytes int) *Cache {
	return &Cache{
		items:      map[entryKey]*entry{},
		lru:        list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
	}
}

var (
	defaultCache *Cache
	defaultOnce  sync.Once
)

// Default returns the process-wide cache widget.Image resolves through.
func Default() *Cache {
	defaultOnce.Do(func() { defaultCache = New() })
	return defaultCache
}

// SetLimits changes the entry and byte budgets. It does not evict
// anything by itself; the next insert trims back under the new limits.
func (c *Cache) SetLimits(maxEntries, maxBytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxEntries, c.maxBytes = maxEntries, maxBytes
}

// Source returns the decoded form of the source called key, calling
// decode once on a miss. Whatever decode returns is normalized into an
// *image.RGBA (premultiplied, the canvas's format) so the byte
// accounting is exact and every consumer blends uniformly. An empty key
// skips the cache and decodes every call.
func (c *Cache) Source(key string, decode func() (image.Image, error)) (image.Image, error) {
	if key == "" {
		return decode()
	}
	c.mu.Lock()
	if e, ok := c.items[key]; ok {
		c.touchLocked(e)
		c.hits++
		c.mu.Unlock()
		return e.img, e.err
	}
	c.misses++
	c.mu.Unlock()

	img, err := decode()
	rgba, size := normalize(img)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.storeLocked(&entry{key: key, img: rgba, err: err, bytes: size})
	return rgba, err
}

// Raster returns the resampled raster for key, key.W x key.H, as a
// still image with its own identity, so every widget drawing it shares
// one GPU texture. On a miss fill writes the pixels into the cache's
// own memory, off the Go heap. A raster is valid for the frame that
// draws it: eviction frees it once every recording that might draw it
// has ended (render.Retire), so a caller draws it and keeps nothing.
func (c *Cache) Raster(key RasterKey, fill func(dst *image.RGBA) error) (render.StillImage, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok {
		c.touchLocked(e)
		c.hits++
		c.mu.Unlock()
		raster, _ := e.img.(render.StillImage)
		return raster, nil
	}
	c.misses++
	c.mu.Unlock()

	mem, err := memory.MakeSlice[byte](&c.pixels, 4*key.W*key.H)
	if err != nil {
		return render.StillImage{}, fmt.Errorf("imgcache: a %dx%d raster: %w", key.W, key.H, err)
	}
	dst := &image.RGBA{Pix: mem.Elems(), Stride: 4 * key.W, Rect: image.Rect(0, 0, key.W, key.H)}
	if err := fill(dst); err != nil {
		c.free(mem)
		return render.StillImage{}, fmt.Errorf("imgcache: resample %s into %dx%d: %w", key.Source, key.W, key.H, err)
	}
	img := render.NewStillImage(dst)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.storeLocked(&entry{key: key, img: img, bytes: pixelBytes(img.Rect), mem: mem})
	return img, nil
}

// SetForget sets what drops an evicted raster's GPU texture, nil for
// nothing: the GPU renderer's device, while one draws. It is called on
// whichever goroutine evicts.
func (c *Cache) SetForget(forget func(render.ImageID)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forget = forget
}

// free hands mem back once no recording can draw it.
func (c *Cache) free(mem memory.Slice[byte]) {
	render.Retire(func() {
		if err := memory.FreeSlice(&c.pixels, mem); err != nil {
			panic(fmt.Sprintf("imgcache: free a raster: %v", err))
		}
	})
}

// InvalidateSource drops the decoded source called key and every
// raster derived from it. SetImage calls it so a swapped source cannot
// leave stale pixels behind.
func (c *Cache) InvalidateSource(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.removeLocked(e)
	}
	for k, e := range c.items {
		if rk, ok := k.(RasterKey); ok && rk.Source == key {
			c.removeLocked(e)
		}
	}
}

// HasSource reports whether the source called key is cached (tests).
func (c *Cache) HasSource(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.items[key]
	return ok
}

// HasRaster reports whether the raster key is cached (tests).
func (c *Cache) HasRaster(key RasterKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.items[key]
	return ok
}

// Reset drops every entry.
func (c *Cache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.items {
		c.release(e)
	}
	c.items = map[entryKey]*entry{}
	c.lru.Init()
	c.bytes = 0
}

// Stats returns the cumulative hit and miss counts. A paint that hits
// the cache must not re-resample; tests read the miss counter to pin
// that.
func (c *Cache) Stats() (hits, misses int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}

// Len returns the number of cached sources and rasters (tests).
func (c *Cache) Len() (sources, rasters int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for k := range c.items {
		if _, ok := k.(RasterKey); ok {
			n++
		}
	}
	return len(c.items) - n, n
}

// Bytes returns the budgeted byte total of everything cached (tests).
func (c *Cache) Bytes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

// touchLocked marks e most recently used.
func (c *Cache) touchLocked(e *entry) { c.lru.MoveToFront(e.el) }

// storeLocked inserts e and trims the LRU from the back until the
// budgets hold again. The newest entry always survives: a budget
// smaller than one image degrades to no caching, not to churn.
func (c *Cache) storeLocked(e *entry) {
	e.el = c.lru.PushFront(e)
	c.items[e.key] = e
	c.bytes += e.bytes
	for c.bytes > c.maxBytes || c.lru.Len() > c.maxEntries {
		back := c.lru.Back()
		if back == nil || back.Value == e {
			break
		}
		if victim, ok := back.Value.(*entry); ok {
			c.removeLocked(victim)
		}
	}
}

func (c *Cache) removeLocked(e *entry) {
	c.lru.Remove(e.el)
	c.bytes -= e.bytes
	delete(c.items, e.key)
	c.release(e)
}

// release frees an evicted raster's pixels and drops its texture; a
// source's are the Go heap's to collect.
func (c *Cache) release(e *entry) {
	if e.mem.Len() == 0 {
		return
	}
	if raster, ok := e.img.(render.StillImage); ok && c.forget != nil {
		c.forget(raster.ID)
	}
	c.free(e.mem)
}

// normalize converts a decoded image into a premultiplied still image
// with an identity, which every widget showing it shares, and reports
// its byte cost. A nil input (a decode failure) costs zero. An image
// that reports its own cost (an animation's frames) is kept as is:
// flattening it would drop all but its first frame.
func normalize(img image.Image) (image.Image, int) {
	if img == nil {
		return nil, 0
	}
	if c, ok := img.(interface{ PixelBytes() int }); ok {
		return img, c.PixelBytes()
	}
	switch v := img.(type) {
	case render.StillImage:
		if v.ID == 0 {
			v.ID = render.NewImageID()
		}
		return v, pixelBytes(v.Rect)
	case *image.RGBA:
		return render.NewStillImage(v), pixelBytes(v.Rect)
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	return render.NewStillImage(rgba), pixelBytes(rgba.Rect)
}

func pixelBytes(r image.Rectangle) int {
	if r.Empty() {
		return 0
	}
	// RGBA packs 4 bytes per pixel, Dx per row.
	return r.Dy() * 4 * r.Dx()
}
