package icons

import (
	"path/filepath"
	"slices"
	"sync"

	"github.com/stubbedev/lys-lib/render"
)

// maxEntries caps the raster cache; the least recently used entry
// evicts first. A bar holds a few dozen icons at one or two sizes, so
// the default is generous without being unbounded, and a view that
// pages through a large icon set leaves the icons still in use cached.
var maxEntries = 256

// maskMode says which sources a tint applies to.
type maskMode uint8

const (
	maskNone     maskMode = iota // never recolor
	maskSymbolic                 // recolor symbolic sources only
	maskAlways                   // recolor whatever matched
)

type cacheKey struct {
	name string
	size int
	frac uint32
}

type cacheEntry struct {
	err error

	symbolic bool
	natural  *render.Icon

	tinted      *render.Icon
	tintedColor render.Color
}

// Cache resolves themed icons and caches the decoded rasters, one per
// (name, size, scale) key with the scale in the 120-based convention of
// internal/scale (120 is 1x, 240 is 2x). It is safe for concurrent use.
//
// SetTheme, SetSearchPaths and InvalidateTheme drop every cached
// raster, bump Generation, and run the OnReset listeners; the
// application follows the desktop's icon theme through the settings
// portal and applies each switch with ApplyIconTheme. widget.Icon
// watches Generation so its next damage pass picks a change up.
type Cache struct {
	mu    sync.Mutex
	theme string
	paths []string
	gen   uint64

	// themeListeners hear every followed icon-theme switch (follow.go).
	themeListeners []func(string)
	// resetListeners hear every reset, whatever caused it.
	resetListeners []func()

	entries map[cacheKey]*cacheEntry
	// order lists the keys from least to most recently used.
	order []cacheKey

	// chain is the theme inheritance chain lookups walk, parsed once;
	// files indexes the directories they read. A reset drops both.
	chain []*Theme
	files dirIndex
}

// DefaultTheme is GTK's default icon theme, the one lookups start on
// until a setting names another.
const DefaultTheme = "Adwaita"

// New returns a cache for the theme called name; empty means the
// default chain alone (Adwaita, then hicolor).
// Lookups search the standard XDG paths until SetSearchPaths overrides
// them.
func New(name string) *Cache {
	return &Cache{theme: name, entries: map[cacheKey]*cacheEntry{}}
}

var (
	defaultCache *Cache
	defaultOnce  sync.Once
)

// Default returns the process-wide cache widget.NewThemeIcon resolves
// through, starting on DefaultTheme, as GTK does.
func Default() *Cache {
	defaultOnce.Do(func() { defaultCache = New(DefaultTheme) })
	return defaultCache
}

// Theme returns the theme this cache looks icons up in.
func (c *Cache) Theme() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.theme
}

// SetTheme switches the theme icons resolve against (empty means the
// default chain alone) and drops every cached raster.
func (c *Cache) SetTheme(name string) {
	c.mu.Lock()
	if name == c.theme {
		c.mu.Unlock()
		return
	}
	c.theme = name
	listeners := c.resetLocked()
	c.mu.Unlock()
	runAll(listeners)
}

// SetSearchPaths overrides the base directories lookups search (the
// slice is copied); nil restores the XDG default. Changing paths drops
// every cached raster.
func (c *Cache) SetSearchPaths(paths []string) {
	c.mu.Lock()
	if slices.Equal(c.paths, paths) && (c.paths == nil) == (paths == nil) {
		c.mu.Unlock()
		return
	}
	c.paths = slices.Clone(paths)
	listeners := c.resetLocked()
	c.mu.Unlock()
	runAll(listeners)
}

// SearchPaths returns the base directories lookups search: what
// SetSearchPaths set, else the XDG default.
func (c *Cache) SearchPaths() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.searchPathsLocked())
}

// Generation counts cache-wide resets: every SetTheme, SetSearchPaths,
// or InvalidateTheme call bumps it. Widgets compare their resolved
// generation against it to notice a theme switch.
func (c *Cache) Generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// InvalidateTheme drops every cached raster and bumps Generation: the
// hook for a theme switch or for icon files installed or removed under
// the search paths, since a miss is cached as firmly as a hit.
func (c *Cache) InvalidateTheme() {
	c.mu.Lock()
	listeners := c.resetLocked()
	c.mu.Unlock()
	runAll(listeners)
}

// OnReset registers fn to run after every reset (SetTheme,
// SetSearchPaths, InvalidateTheme, an applied ApplyIconTheme), once
// Generation has moved. Callbacks run on the caller's goroutine; bridge
// into the loop with Application.Invoke, where the usual reaction is
// requesting a repaint.
func (c *Cache) OnReset(fn func()) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetListeners = append(c.resetListeners, fn)
}

// resetLocked drops every cached raster and bumps Generation; it
// returns the reset listeners for the caller to run once unlocked.
func (c *Cache) resetLocked() []func() {
	c.entries = map[cacheKey]*cacheEntry{}
	c.order = nil
	c.chain, c.files = nil, nil
	c.gen++
	return slices.Clone(c.resetListeners)
}

func runAll(fns []func()) {
	for _, fn := range fns {
		fn()
	}
}

func (c *Cache) searchPathsLocked() []string {
	if c.paths != nil {
		return c.paths
	}
	return defaultSearchPaths()
}

// Icon returns the icon called name at logical size in its natural
// colors, rasterized into a DeviceBox(size, frac120) pixel box. The
// returned icon is shared between callers and cache entries; treat it
// as read-only.
func (c *Cache) Icon(name string, size int, frac120 uint32) (*render.Icon, error) {
	return c.load(name, size, frac120, 0, maskNone)
}

// SymbolicIcon returns the icon called name, mask-recolored to tint
// when the matched source is symbolic - a -symbolic file or an SVG
// painting with currentColor - so symbolic glyphs follow the theme
// accent while ordinary app icons keep their colors.
func (c *Cache) SymbolicIcon(name string, size int, frac120 uint32, tint render.Color) (*render.Icon, error) {
	return c.load(name, size, frac120, tint, maskSymbolic)
}

// Tinted returns the icon called name mask-recolored to tint whatever
// it matched: every pixel keeps its alpha and takes tint's color.
func (c *Cache) Tinted(name string, size int, frac120 uint32, tint render.Color) (*render.Icon, error) {
	return c.load(name, size, frac120, tint, maskAlways)
}

// load resolves and decodes once per key, then applies the tint policy.
// A tinted raster is derived from the cached natural one per tint color
// (one slot per entry: switching tints re-derives, it does not
// re-decode).
func (c *Cache) load(name string, size int, frac120 uint32, tint render.Color, mode maskMode) (*render.Icon, error) {
	if tint == 0 {
		mode = maskNone
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, err := c.entryLocked(name, size, frac120)
	if err != nil {
		return nil, err
	}
	if mode == maskNone || (mode == maskSymbolic && !e.symbolic) {
		return e.natural, nil
	}
	if e.tinted == nil || e.tintedColor != tint {
		e.tinted = e.natural.Tint(tint)
		e.tintedColor = tint
	}
	return e.tinted, nil
}

// entryLocked returns the cached entry for the key, resolving and
// decoding on first use. Failures stick in the entry until a reset
// drops it, so a missing icon costs one lookup rather than one per
// frame.
func (c *Cache) entryLocked(name string, size int, frac120 uint32) (*cacheEntry, error) {
	key := cacheKey{name: name, size: size, frac: frac120}
	if e, ok := c.entries[key]; ok {
		i := slices.Index(c.order, key)
		c.order = append(slices.Delete(c.order, i, i+1), key)
		return e, e.err
	}
	e := &cacheEntry{}
	path, err := c.resolveLocked(name, size, frac120)
	if err == nil {
		e.decode(path, size, frac120)
	} else {
		e.err = err
	}
	c.entries[key] = e
	c.order = append(c.order, key)
	if len(c.order) > maxEntries {
		delete(c.entries, c.order[0])
		c.order = slices.Delete(c.order, 0, 1)
	}
	return e, e.err
}

// decode reads and rasterizes the matched file at the key's device box.
func (e *cacheEntry) decode(path string, size int, frac120 uint32) {
	data, err := readIcon(path)
	if err != nil {
		e.err = err
		return
	}
	e.symbolic = IsSymbolic(filepath.Base(path), data)
	e.natural, e.err = DecodeIcon(data, filepath.Base(path), size, frac120)
}
