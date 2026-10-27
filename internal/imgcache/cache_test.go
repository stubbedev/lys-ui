package imgcache

import (
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// redRGBA returns a fresh w x h red raster; every entry gets its own
// backing array so pointer identity cannot mask an eviction.
func redRGBA(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{R: 0xff, A: 0xff})
		}
	}
	return img
}

func TestSourceDecodesOnce(t *testing.T) {
	c := New()
	decodes := 0
	decode := func() (image.Image, error) {
		decodes++
		return redRGBA(4, 2), nil
	}

	a, err := c.Source("k", decode)
	if err != nil || decodes != 1 {
		t.Fatalf("first Source: err=%v decodes=%d, want nil/1", err, decodes)
	}
	b, err := c.Source("k", decode)
	if err != nil || decodes != 1 {
		t.Fatalf("second Source re-decoded: err=%v decodes=%d, want nil/1", err, decodes)
	}
	if a != b {
		t.Error("Source returned different images for one key")
	}
	hits, misses := c.Stats()
	if hits != 1 || misses != 1 {
		t.Errorf("stats = (%d hits, %d misses), want (1, 1)", hits, misses)
	}
	sources, rasters := c.Len()
	if sources != 1 || rasters != 0 {
		t.Errorf("Len = (%d, %d), want (1, 0)", sources, rasters)
	}
	if got, want := c.Bytes(), 4*2*4; got != want {
		t.Errorf("Bytes = %d, want %d", got, want)
	}
}

// A decoded source comes back premultiplied as a still image with an
// identity, the same on every hit, so every widget showing it shares
// one GPU texture.
func TestSourceNormalizesToAnIdentifiedStillImage(t *testing.T) {
	c := New()
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0x40})
	src.SetNRGBA(1, 0, color.NRGBA{R: 0xff, G: 0x00, B: 0x00, A: 0xff})

	img, err := c.Source("k", func() (image.Image, error) { return src, nil })
	if err != nil {
		t.Fatal(err)
	}
	rgba, ok := img.(render.StillImage)
	if !ok || rgba.ID == 0 {
		t.Fatalf("Source returned %T (%v), want a render.StillImage with an identity", img, rgba.ID)
	}
	if again, _ := c.Source("k", func() (image.Image, error) { return src, nil }); again.(render.StillImage).ID != rgba.ID {
		t.Error("a cache hit returned the source under another identity")
	}
	// Premultiplied: alpha 0x40 scales the channels down.
	if got, want := rgba.RGBAAt(0, 0), (color.RGBA{R: 0x04, G: 0x08, B: 0x0c, A: 0x40}); got != want {
		t.Errorf("premultiplied pixel = %v, want %v", got, want)
	}
	if got := rgba.RGBAAt(1, 0); got.R != 0xff {
		t.Errorf("opaque pixel = %v, want full red", got)
	}
}

func TestSourceStickyError(t *testing.T) {
	c := New()
	decodes := 0
	decode := func() (image.Image, error) {
		decodes++
		return nil, errBoom
	}
	if _, err := c.Source("bad", decode); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
	if _, err := c.Source("bad", decode); !errors.Is(err, errBoom) {
		t.Fatalf("second err = %v, want errBoom", err)
	}
	if decodes != 1 {
		t.Errorf("decode attempts = %d, want 1 (failures stick)", decodes)
	}
	// Dropping the entry retries exactly once.
	c.InvalidateSource("bad")
	if _, err := c.Source("bad", decode); !errors.Is(err, errBoom) {
		t.Fatalf("post-invalidate err = %v, want errBoom", err)
	}
	if decodes != 2 {
		t.Errorf("decode attempts = %d, want 2", decodes)
	}
}

var errBoom = errorBoom{}

type errorBoom struct{}

func (errorBoom) Error() string { return "boom" }

// fillRed paints dst red, a raster's fill.
func fillRed(dst *image.RGBA) {
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i], dst.Pix[i+3] = 0xff, 0xff
	}
}

// mustRaster is c.Raster, failing the test on an error.
func mustRaster(t *testing.T, c *Cache, key RasterKey, fill func(dst *image.RGBA) error) render.StillImage {
	t.Helper()
	img, err := c.Raster(key, fill)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestRasterHitDoesNotRebuild(t *testing.T) {
	c := New()
	key := RasterKey{Source: "s", Src: image.Rect(0, 0, 4, 4), W: 8, H: 8}
	builds := 0
	build := func(dst *image.RGBA) error {
		builds++
		fillRed(dst)
		return nil
	}
	a := mustRaster(t, c, key, build)
	if builds != 1 {
		t.Fatalf("builds = %d after first Raster, want 1", builds)
	}
	if b := mustRaster(t, c, key, build); b != a {
		t.Error("Raster hit returned a different raster")
	}
	if builds != 1 {
		t.Errorf("cache hit rebuilt the raster: builds = %d, want 1", builds)
	}
	// A different device size is a different key.
	mustRaster(t, c, RasterKey{Source: "s", Src: key.Src, W: 16, H: 16}, build)
	if builds != 2 {
		t.Errorf("builds = %d after a new size, want 2", builds)
	}
}

func TestLRUEvictsUnderEntryCap(t *testing.T) {
	c := newWithLimits(2, 1<<30)
	builds := map[string]int{}
	build := func(id string) func(dst *image.RGBA) error {
		return func(dst *image.RGBA) error {
			builds[id]++
			fillRed(dst)
			return nil
		}
	}
	key := func(id string) RasterKey {
		return RasterKey{Source: id, Src: image.Rect(0, 0, 1, 1), W: 1, H: 1}
	}

	mustRaster(t, c, key("a"), build("a"))
	mustRaster(t, c, key("b"), build("b"))
	mustRaster(t, c, key("a"), build("a")) // touch a: b becomes the LRU entry
	mustRaster(t, c, key("c"), build("c")) // evicts b

	if builds["b"] != 1 {
		t.Fatalf("b rebuilt too early: %d builds", builds["b"])
	}
	mustRaster(t, c, key("b"), build("b")) // must miss now
	if builds["b"] != 2 {
		t.Errorf("b was not evicted: builds = %d, want 2", builds["b"])
	}
	if builds["a"] != 1 || builds["c"] != 1 {
		t.Errorf("a/c builds = %d/%d, want 1/1 (a's re-lookup hit, c was new)", builds["a"], builds["c"])
	}
	if _, rasters := c.Len(); rasters != 2 {
		t.Errorf("rasters = %d, want the capped 2", rasters)
	}
}

func TestByteCapEvictsLRU(t *testing.T) {
	// One 2x1 raster costs 2*4 = 8 bytes; three entries need 24, so a
	// 20-byte cap always evicts exactly the least recent one.
	c := newWithLimits(100, 20)
	builds := map[string]int{}
	build := func(id string) func(dst *image.RGBA) error {
		return func(dst *image.RGBA) error {
			builds[id]++
			fillRed(dst)
			return nil
		}
	}
	key := func(id string) RasterKey {
		return RasterKey{Source: id, W: 2, H: 1}
	}
	mustRaster(t, c, key("a"), build("a"))
	mustRaster(t, c, key("b"), build("b"))
	mustRaster(t, c, key("c"), build("c"))
	if builds["a"] != 1 {
		t.Errorf("a rebuilt before the cap: %d builds", builds["a"])
	}
	mustRaster(t, c, key("a"), build("a"))
	if builds["a"] != 2 {
		t.Errorf("a was not the LRU eviction victim: builds = %d", builds["a"])
	}
	if got, want := c.Bytes(), 16; got != want {
		t.Errorf("Bytes = %d, want %d", got, want)
	}
}

func TestNewestSurvivesTinyBudget(t *testing.T) {
	// A budget smaller than one entry degrades to no caching, never to
	// churn or a negative byte count.
	c := newWithLimits(100, 2)
	builds := 0
	build := func(dst *image.RGBA) error {
		builds++
		fillRed(dst)
		return nil
	}
	key := RasterKey{Source: "s", W: 4, H: 4}
	mustRaster(t, c, key, build)
	mustRaster(t, c, key, build)
	if builds != 1 {
		t.Errorf("builds = %d, want 1 (the single entry survives and stays cached)", builds)
	}
	if got := c.Bytes(); got != 64 {
		t.Errorf("Bytes = %d, want the one surviving entry's 64", got)
	}
	if _, rasters := c.Len(); rasters != 1 {
		t.Errorf("rasters = %d, want 1", rasters)
	}
}

func TestInvalidateSourceDropsDerivedRasters(t *testing.T) {
	c := New()
	if _, err := c.Source("gone", func() (image.Image, error) { return redRGBA(4, 4), nil }); err != nil {
		t.Fatal(err)
	}
	mustRaster(t, c, RasterKey{Source: "gone", W: 8, H: 8}, func(dst *image.RGBA) error { fillRed(dst); return nil })
	mustRaster(t, c, RasterKey{Source: "gone", W: 4, H: 4}, func(dst *image.RGBA) error { fillRed(dst); return nil })
	mustRaster(t, c, RasterKey{Source: "kept", W: 4, H: 4}, func(dst *image.RGBA) error { fillRed(dst); return nil })

	c.InvalidateSource("gone")

	sources, rasters := c.Len()
	if sources != 0 {
		t.Errorf("sources = %d, want 0 after invalidate", sources)
	}
	if rasters != 1 {
		t.Errorf("rasters = %d, want the 1 entry of the untouched source", rasters)
	}
	if got, want := c.Bytes(), 4*4*4; got != want {
		t.Errorf("Bytes = %d, want %d", got, want)
	}
}

func TestReset(t *testing.T) {
	c := New()
	if _, err := c.Source("k", func() (image.Image, error) { return redRGBA(2, 2), nil }); err != nil {
		t.Fatal(err)
	}
	mustRaster(t, c, RasterKey{Source: "k", W: 2, H: 2}, func(dst *image.RGBA) error { fillRed(dst); return nil })
	c.Reset()
	if got := c.Bytes(); got != 0 {
		t.Errorf("Bytes after reset = %d, want 0", got)
	}
	sources, rasters := c.Len()
	if sources != 0 || rasters != 0 {
		t.Errorf("Len after reset = (%d, %d), want (0, 0)", sources, rasters)
	}
}

// A raster lives off the Go heap in the cache's memory, filled where it
// lies; evicting it drops its GPU texture and frees its pages once no
// recording can draw it, and Reset frees them all (#412).
func TestRastersLiveOffTheHeapAndEvictionFreesThem(t *testing.T) {
	c := newWithLimits(1, 1<<30)
	var forgotten []render.ImageID
	c.SetForget(func(id render.ImageID) { forgotten = append(forgotten, id) })
	a := mustRaster(t, c, RasterKey{Source: "a", W: 4, H: 4}, func(dst *image.RGBA) error { fillRed(dst); return nil })
	if st := c.pixels.Stats(); st.Live != 1 || st.Bytes < 4*4*4 {
		t.Fatalf("raster memory %+v, want one 4x4 raster off the heap", st)
	}
	if a.RGBAAt(3, 3).R != 0xff {
		t.Error("the raster does not hold what fill wrote")
	}
	mustRaster(t, c, RasterKey{Source: "b", W: 4, H: 4}, func(dst *image.RGBA) error { fillRed(dst); return nil })
	if st := c.pixels.Stats(); st.Live != 1 {
		t.Errorf("%d rasters live after evicting one under a one-entry cap", st.Live)
	}
	if len(forgotten) != 1 || forgotten[0] != a.ID {
		t.Errorf("forgot %v on eviction, want the evicted raster's %v", forgotten, a.ID)
	}
	if _, err := c.Raster(RasterKey{Source: "c", W: 2, H: 2}, func(*image.RGBA) error { return errBoom }); err == nil {
		t.Error("a failed fill came back as a raster")
	}
	c.Reset()
	if st := c.pixels.Stats(); st.Live != 0 {
		t.Errorf("%d rasters outlived Reset", st.Live)
	}
}
