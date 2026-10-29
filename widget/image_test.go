package widget

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/imgcache"
)

// quadImage returns a 4x4 raster with solid 2x2 quadrants: red top
// left, green top right, blue bottom left, yellow bottom right.
func quadImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	fill := func(x0, y0 int, c color.NRGBA) {
		for y := y0; y < y0+2; y++ {
			for x := x0; x < x0+2; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	fill(0, 0, color.NRGBA{R: 0xff, A: 0xff})
	fill(2, 0, color.NRGBA{G: 0xff, A: 0xff})
	fill(0, 2, color.NRGBA{B: 0xff, A: 0xff})
	fill(2, 2, color.NRGBA{R: 0xff, G: 0xff, A: 0xff})
	return img
}

// rampImage returns a w x h raster with a distinct byte at every pixel,
// so a 1:1 blit can be compared exactly.
func rampImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*16 + y), G: 0x80, B: 0x40, A: 0xff})
		}
	}
	return img
}

func solidPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writePNG(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "art.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// paintImage measures, arranges, and paints im at the origin of a
// w x h logical canvas at the given device scale, over black. It
// returns the pixel data so tests can read back what painted.
func paintImage(im *Image, w, h, num, denom int) ([]byte, int) {
	dev := (max(w, h)*num + denom - 1) / denom
	data := make([]byte, render.Stride(dev)*dev)
	cv := render.NewScaled(data, render.Stride(dev), dev, dev, num, denom)
	cv.Clear(cv.Rect(), render.RGB(0, 0, 0))
	SizeWithin(im, Size{W: w, H: h})
	im.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	im.Paint(cv)
	return data, dev
}

func px(data []byte, dev, x, y int) render.Color {
	return render.ColorFromBytes(data[y*render.Stride(dev)+x*4:])
}

func resetImageCache(t *testing.T) {
	t.Helper()
	imgcache.Default().Reset()
	t.Cleanup(func() { imgcache.Default().Reset() })
}

// fakeInvoker stands in for the app-installed bridge: deliveries queue
// until drain runs them on the calling goroutine - the fake loop's
// pass, as in app's invoke tests.
type fakeInvoker struct {
	mu  sync.Mutex
	fns []func()
	got chan struct{}
}

func newFakeInvoker(t *testing.T) *fakeInvoker {
	t.Helper()
	f := &fakeInvoker{got: make(chan struct{}, 64)}
	SetInvoker(f.invoke)
	t.Cleanup(func() { SetInvoker(nil) })
	return f
}

func (f *fakeInvoker) invoke(fn func()) {
	f.mu.Lock()
	f.fns = append(f.fns, fn)
	f.mu.Unlock()
	f.got <- struct{}{}
}

// drain waits for n deliveries, then runs them in order.
func (f *fakeInvoker) drain(t *testing.T, n int) {
	t.Helper()
	for i := range n {
		select {
		case <-f.got:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for invoke %d of %d", i+1, n)
		}
	}
	f.mu.Lock()
	fns := f.fns
	f.fns = nil
	f.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func TestImageFitPaintsQuadrants(t *testing.T) {
	resetImageCache(t)
	im := NewImage(quadImage())
	data, dev := paintImage(im, 4, 4, 1, 1)
	want := map[[2]int]render.Color{
		{0, 0}: render.RGB(0xff, 0, 0),
		{3, 0}: render.RGB(0, 0xff, 0),
		{0, 3}: render.RGB(0, 0, 0xff),
		{3, 3}: render.RGB(0xff, 0xff, 0),
	}
	for at, c := range want {
		if got := px(data, dev, at[0], at[1]); got != c {
			t.Errorf("pixel %v = %v, want %v", at, got, c)
		}
	}
}

func TestImageFitUpscaleBlends(t *testing.T) {
	resetImageCache(t)
	// A black|white edge upscaled 4x: the cubic kernel blends across
	// the boundary; nearest-neighbor would only ever produce pure
	// black or pure white.
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{A: 0xff})
	src.SetNRGBA(1, 0, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	im := NewImage(src)

	data, dev := paintImage(im, 8, 8, 1, 1)
	if got := px(data, dev, 2, 2); got.R() == 0 || got.R() == 0xff {
		t.Errorf("edge pixel = %v, want a blend strictly between black and white", got)
	}
	if got := px(data, dev, 0, 0); got.R() != 0 {
		t.Errorf("far-left pixel = %v, want black", got)
	}
	if got := px(data, dev, 7, 3); got.R() != 0xff {
		t.Errorf("far-right pixel = %v, want white", got)
	}
}

func TestImageCoverFillsByCropping(t *testing.T) {
	resetImageCache(t)
	im := NewImage(quadImage())
	im.SetScale(ImageCover)
	// 4x4 source in a 4x2 box: the centered 4x2 crop is the middle
	// band (red/green on top, blue/yellow below) filling every pixel.
	data, dev := paintImage(im, 4, 2, 1, 1)
	want := map[[2]int]render.Color{
		{0, 0}: render.RGB(0xff, 0, 0),
		{3, 0}: render.RGB(0, 0xff, 0),
		{0, 1}: render.RGB(0, 0, 0xff),
		{3, 1}: render.RGB(0xff, 0xff, 0),
	}
	for at, c := range want {
		if got := px(data, dev, at[0], at[1]); got != c {
			t.Errorf("pixel %v = %v, want %v (cover must fill the box)", at, got, c)
		}
	}
}

func TestImageNoneIsPixelExact(t *testing.T) {
	resetImageCache(t)
	src := rampImage(8, 8)
	im := NewImage(src)
	im.SetScale(ImageNone)
	data, dev := paintImage(im, 4, 4, 1, 1)
	// The centered 4x4 window of the source lands one-to-one.
	want := map[[2]int][2]int{
		{0, 0}: {2, 2},
		{3, 0}: {5, 2},
		{0, 3}: {2, 5},
		{3, 3}: {5, 5},
	}
	for dat, sat := range want {
		wantC := src.NRGBAAt(sat[0], sat[1])
		wantCol := render.RGBA(wantC.R, wantC.G, wantC.B, wantC.A)
		if got := px(data, dev, dat[0], dat[1]); got != wantCol {
			t.Errorf("pixel %v = %v, want source pixel %v = %v", dat, got, sat, wantCol)
		}
	}
}

func TestImageNoneSkipsTheCache(t *testing.T) {
	resetImageCache(t)
	im := NewImage(rampImage(8, 8))
	im.SetScale(ImageNone)
	_, missesBefore := cacheStats()
	paintImage(im, 4, 4, 1, 1)
	paintImage(im, 4, 4, 1, 1)
	if _, misses := cacheStats(); misses != missesBefore {
		t.Errorf("ImageNone consulted the resample cache: misses %d -> %d", missesBefore, misses)
	}
}

func cacheStats() (hits, misses int) { return imgcache.Default().Stats() }

func TestImageCacheHitDoesNotRescale(t *testing.T) {
	resetImageCache(t)
	im := NewImage(quadImage())
	paintImage(im, 8, 8, 1, 1)
	_, misses := cacheStats()

	paintImage(im, 8, 8, 1, 1)
	paintImage(im, 8, 8, 1, 1)
	if _, got := cacheStats(); got != misses {
		t.Errorf("repaints at the same size rescaled: misses %d -> %d, want unchanged", misses, got)
	}
}

func TestImageCacheKeyTracksDeviceScale(t *testing.T) {
	resetImageCache(t)
	im := NewImage(quadImage())
	paintImage(im, 8, 8, 1, 1) // 1x: an 8x8 device raster
	_, misses1x := cacheStats()
	if _, got := cacheStats(); got != misses1x {
		t.Fatalf("first paint missed twice")
	}
	paintImage(im, 8, 8, 3, 2) // 1.5x: the box maps to 12x12 device pixels
	_, misses15x := cacheStats()
	if misses15x == misses1x {
		t.Fatal("painting at a new device scale reused the 1x raster")
	}
	paintImage(im, 8, 8, 3, 2)
	if _, got := cacheStats(); got != misses15x {
		t.Errorf("repaint at the same device scale rescaled: misses %d -> %d", misses15x, got)
	}
	// The cached raster is the device-sized one: crisp, not a rescale
	// of the 1x entry.
	key := imgcache.RasterKey{Source: im.src.key, Src: image.Rect(0, 0, 4, 4), W: 12, H: 12}
	if !imgcache.Default().HasRaster(key) {
		t.Errorf("no cache entry for the 12x12 device raster")
	}
}

func TestSetImageInvalidatesAndDropsCache(t *testing.T) {
	resetImageCache(t)
	im := NewImage(quadImage())
	paintImage(im, 8, 8, 1, 1)
	if _, dirty := CollectDamage(im); !dirty {
		t.Fatal("freshly painted widget reported no damage before the drain")
	}
	oldRaster := imgcache.RasterKey{Source: im.src.key, Src: image.Rect(0, 0, 4, 4), W: 8, H: 8}

	// A source with a different natural size, so the measure cache must
	// be dropped too.
	big := image.NewNRGBA(image.Rect(0, 0, 6, 6))
	for y := range 6 {
		for x := range 6 {
			big.SetNRGBA(x, y, color.NRGBA{B: 0xff, A: 0xff})
		}
	}
	im.SetImage(big)

	if _, dirty := CollectDamage(im); !dirty {
		t.Error("SetImage left no pending damage")
	}
	con := Size{W: 100, H: 100}
	if got := SizeWithin(im, con); got != (Size{W: 6, H: 6}) {
		t.Errorf("Measure after SetImage = %v, want 6x6 (measure cache was stale)", got)
	}
	if imgcache.Default().HasRaster(oldRaster) {
		t.Error("SetImage kept the old source's rescaled raster")
	}
	if im.Err() != nil || !im.Loaded() {
		t.Errorf("Err = %v Loaded = %v, want nil/true", im.Err(), im.Loaded())
	}
	data, dev := paintImage(im, 8, 8, 1, 1)
	if got := px(data, dev, 4, 4); got != render.RGB(0, 0, 0xff) {
		t.Errorf("pixel after SetImage = %v, want blue", got)
	}
}

func TestSetBytesSourceDropsDecodedSource(t *testing.T) {
	resetImageCache(t)
	data := solidPNG(t, 4, 4, color.NRGBA{R: 0xff, A: 0xff})
	im := NewBytesImage(data, "a.png")
	key := im.src.key
	if !imgcache.Default().HasSource(key) {
		t.Fatal("decoded bytes source is not cached under its hash")
	}
	im.SetImage(quadImage())
	if imgcache.Default().HasSource(key) {
		t.Error("SetImage kept the old bytes source in the cache")
	}
}

func TestImagePlaceholderPaintsWhileAsyncPending(t *testing.T) {
	resetImageCache(t)
	inv := newFakeInvoker(t)
	ph := render.RGB(0x30, 0x30, 0x40)
	im := NewFileImage(writePNG(t, solidPNG(t, 4, 4, color.NRGBA{R: 0xff, A: 0xff})))
	im.SetPlaceholderColor(ph)

	data, dev := paintImage(im, 6, 6, 1, 1)
	if got := px(data, dev, 3, 3); got != ph {
		t.Errorf("placeholder pixel = %v, want %v", got, ph)
	}
	if im.Loaded() {
		t.Error("async source reported loaded before its load landed")
	}
	// Settle the load this test kicked: leaving the delivery in flight
	// can land it in the next test's invoker and flake its assertions.
	inv.drain(t, 1)
}

func TestImageAsyncLoadFiresOnLoadedExactlyOnce(t *testing.T) {
	resetImageCache(t)
	inv := newFakeInvoker(t)
	im := NewFileImage(writePNG(t, solidPNG(t, 4, 4, color.NRGBA{G: 0xff, A: 0xff})))

	fired := 0
	im.OnLoaded = func(*Image) { fired++ }

	data, dev := paintImage(im, 6, 6, 1, 1) // kicks the load, paints placeholder
	if fired != 0 {
		t.Fatalf("OnLoaded fired before the load landed (%d times)", fired)
	}
	inv.drain(t, 1)

	if !im.Loaded() || im.Err() != nil {
		t.Fatalf("Loaded = %v Err = %v, want true/nil", im.Loaded(), im.Err())
	}
	if got := px(data, dev, 3, 3); got == render.RGB(0, 0xff, 0) {
		t.Error("the pixels from the same paint as the kick are already green: load was synchronous")
	}
	data, dev = paintImage(im, 6, 6, 1, 1)
	if got := px(data, dev, 3, 3); got != render.RGB(0, 0xff, 0) {
		t.Errorf("pixel after load = %v, want green", got)
	}
	paintImage(im, 6, 6, 1, 1)
	inv.drain(t, 0) // further paints must not queue more deliveries
	if fired != 1 {
		t.Errorf("OnLoaded fired %d times, want exactly 1", fired)
	}
}

func TestImageAsyncErrorSurfaces(t *testing.T) {
	resetImageCache(t)
	inv := newFakeInvoker(t)
	im := NewFileImage(filepath.Join(t.TempDir(), "missing.png"))

	fired := 0
	im.OnLoaded = func(*Image) { fired++ }

	paintImage(im, 6, 6, 1, 1)
	inv.drain(t, 1)

	if im.Err() == nil {
		t.Error("Err = nil, want the load failure")
	}
	if fired != 1 {
		t.Errorf("OnLoaded fired %d times on failure, want exactly 1", fired)
	}
	data, dev := paintImage(im, 6, 6, 1, 1)
	if got := px(data, dev, 3, 3); got != render.RGB(0, 0, 0) {
		t.Errorf("failed load painted %v, want untouched black", got)
	}
}

func TestImageSyncFilePaintsWithoutInvoker(t *testing.T) {
	resetImageCache(t)
	SetInvoker(nil) // no application: the load settles at the first paint
	im := NewFileImage(writePNG(t, solidPNG(t, 4, 4, color.NRGBA{B: 0xff, A: 0xff})))

	fired := 0
	im.OnLoaded = func(*Image) { fired++ }

	data, dev := paintImage(im, 6, 6, 1, 1)
	if got := px(data, dev, 3, 3); got != render.RGB(0, 0, 0xff) {
		t.Errorf("pixel = %v, want blue from the first synchronous paint", got)
	}
	if !im.Loaded() || im.Err() != nil {
		t.Errorf("Loaded = %v Err = %v, want true/nil", im.Loaded(), im.Err())
	}
	paintImage(im, 6, 6, 1, 1)
	if fired != 1 {
		t.Errorf("OnLoaded fired %d times, want exactly 1", fired)
	}
}

func TestImageStaleAsyncResultDropped(t *testing.T) {
	resetImageCache(t)
	inv := newFakeInvoker(t)
	im := NewFileImage(writePNG(t, solidPNG(t, 4, 4, color.NRGBA{R: 0xff, A: 0xff})))

	fired := 0
	im.OnLoaded = func(*Image) { fired++ }
	paintImage(im, 6, 6, 1, 1) // kick the load for the old source

	// Replace the source before the load lands: the stale result must
	// not overwrite the new one, and must not fire OnLoaded.
	im.SetImage(quadImage())
	inv.drain(t, 1)

	if fired != 0 {
		t.Errorf("OnLoaded fired %d times for a stale load, want 0", fired)
	}
	data, dev := paintImage(im, 6, 6, 1, 1)
	if got := px(data, dev, 0, 0); got != render.RGB(0xff, 0, 0) && got != render.RGB(0xff, 0xff, 0) && got != render.RGB(0, 0, 0xff) {
		t.Errorf("pixel = %v, want the replacement image's quadrant colors", got)
	}
	if im.Err() != nil {
		t.Errorf("Err = %v, want nil (the stale failure must not leak either)", im.Err())
	}
}

func TestImageBytesDecode(t *testing.T) {
	resetImageCache(t)
	// JPEG goes through image/jpeg: lossy, so compare with tolerance.
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetNRGBA(x, y, color.NRGBA{R: 0x40, G: 0x80, B: 0xc0, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}

	_, missesBefore := cacheStats()
	im := NewBytesImage(buf.Bytes(), "art.jpg")
	if !im.Loaded() || im.Err() != nil {
		t.Fatalf("Loaded = %v Err = %v, want true/nil", im.Loaded(), im.Err())
	}
	im2 := NewBytesImage(buf.Bytes(), "art.jpg")
	_, misses := cacheStats()
	if misses != missesBefore+1 {
		t.Errorf("identical bytes decoded twice: misses %d -> %d, want +1", missesBefore, misses)
	}
	if im2.src.key != im.src.key {
		t.Error("identical bytes hashed to different source keys")
	}

	data, dev := paintImage(im, 8, 8, 1, 1)
	got := px(data, dev, 4, 4)
	if abs(int(got.R())-0x40) > 12 || abs(int(got.G())-0x80) > 12 || abs(int(got.B())-0xc0) > 12 {
		t.Errorf("decoded pixel = %v, want within 12 of RGB(0x40,0x80,0xc0)", got)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestImageBytesDecodeError(t *testing.T) {
	resetImageCache(t)
	im := NewBytesImage([]byte("not an image at all"), "art.png")
	if !im.Loaded() {
		t.Fatal("a failed decode should still settle the load")
	}
	if im.Err() == nil {
		t.Error("Err = nil, want the decode failure")
	}
	data, dev := paintImage(im, 6, 6, 1, 1)
	if got := px(data, dev, 3, 3); got != render.RGB(0, 0, 0) {
		t.Errorf("undecodable bytes painted %v, want untouched black", got)
	}
}

func TestImageURLFetch(t *testing.T) {
	resetImageCache(t)
	inv := newFakeInvoker(t)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		hits++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(solidPNG(t, 4, 4, color.NRGBA{R: 0, G: 0xff, A: 0xff}))
	}))
	t.Cleanup(srv.Close)

	im := NewURLImage(srv.URL)
	fired := 0
	im.OnLoaded = func(*Image) { fired++ }

	paintImage(im, 6, 6, 1, 1)
	inv.drain(t, 1)
	if im.Err() != nil {
		t.Fatalf("Err = %v, want nil", im.Err())
	}
	data, dev := paintImage(im, 6, 6, 1, 1)
	if got := px(data, dev, 3, 3); got != render.RGB(0, 0xff, 0) {
		t.Errorf("pixel = %v, want green from the fetched PNG", got)
	}
	if fired != 1 {
		t.Errorf("OnLoaded fired %d times, want 1", fired)
	}

	// A failing URL surfaces through Err, still firing OnLoaded once.
	bad := NewURLImage(srv.URL + "/gone")
	badFired := 0
	bad.OnLoaded = func(*Image) { badFired++ }
	paintImage(bad, 6, 6, 1, 1)
	inv.drain(t, 1)
	if bad.Err() == nil {
		t.Error("404 left Err nil")
	}
	if badFired != 1 {
		t.Errorf("OnLoaded fired %d times on the failed fetch, want 1", badFired)
	}
	if hits != 1 {
		t.Errorf("server saw %d fetches, want 1 (the 404 never reached the handler)", hits)
	}
}

func TestImageMeasureUnsettledThenNatural(t *testing.T) {
	resetImageCache(t)
	inv := newFakeInvoker(t)
	im := NewFileImage(writePNG(t, solidPNG(t, 8, 8, color.NRGBA{A: 0xff})))

	con := Size{W: 100, H: 100}
	if got := SizeWithin(im, con); got != (Size{}) {
		t.Errorf("unsettled Measure = %v, want 0x0", got)
	}
	paintImage(im, 6, 6, 1, 1) // kicks the load
	inv.drain(t, 1)
	if got := SizeWithin(im, con); got != (Size{W: 8, H: 8}) {
		t.Errorf("Measure after the load = %v, want 8x8 (the stale 0x0 cache must be dropped)", got)
	}
}

// imageCaptor records the images a recording draws; it takes no other
// call.
type imageCaptor struct {
	render.Player
	imgs []image.Image
}

func (c *imageCaptor) DrawImageDevice(_ render.State, img image.Image, _, _ int) {
	c.imgs = append(c.imgs, img)
}

func (c *imageCaptor) DrawImageCover(_ render.State, img image.Image, _ render.Rect, _ render.Corners) {
	c.imgs = append(c.imgs, img)
}

func (c *imageCaptor) PaintImageTiles(_ render.State, img image.Image, _ render.Tile, _ render.Rect, _ render.Corners) {
	c.imgs = append(c.imgs, img)
}

func (c *imageCaptor) FillRectDevice(render.State, render.Rect, render.Color)                     {}
func (c *imageCaptor) RoundedRect(render.State, render.Rect, int, render.Color)                   {}
func (c *imageCaptor) RoundedRectCorners(render.State, render.Rect, render.Corners, render.Color) {}
func (c *imageCaptor) RoundedBorderSides(render.State, render.Rect, render.Corners, render.Insets, [4]render.Color) {
}

// drawnImages paints w into a recording and returns the images it
// draws.
func drawnImages(w Widget, width, height int) []image.Image {
	rec := render.NewRecorder(width, height, 1, 1)
	w.Paint(rec)
	c := &imageCaptor{}
	rec.Recording().Play(c)
	return c.imgs
}

// Every image an Image, Avatar or ScreenPick draws is one a GPU
// renderer keeps as a texture: an app's *image.RGBA is held still
// without a copy, any other model converted once, a versioned image
// keeps its version, and natural-size and resampled draws alike (#313).
func TestWidgetImagesStayTexturable(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(0, 0, 4, 4))
	versioned := render.VersionedImage{RGBA: image.NewRGBA(image.Rect(0, 0, 4, 4)), Version: 7}
	for _, tc := range []struct {
		name string
		w    Widget
		size int
	}{
		{"image rgba 1:1", NewImage(rgba), 4},
		{"image nrgba scaled", NewImage(quadImage()), 16},
		{"image versioned 1:1", NewImage(versioned), 4},
		{"image versioned scaled", NewImage(versioned), 16},
		{"avatar", func() Widget { a := NewAvatar(testFace(t), 16, "A B"); a.SetImage(quadImage()); return a }(), 16},
		{"screen pick scaled", NewScreenPick(quadImage()), 16},
	} {
		SizeWithin(tc.w, Size{W: tc.size, H: tc.size})
		tc.w.Arrange(render.Rect{W: tc.size, H: tc.size})
		imgs := drawnImages(tc.w, tc.size, tc.size)
		if len(imgs) == 0 {
			t.Fatalf("%s: drew no image", tc.name)
		}
		for _, img := range imgs {
			switch v := img.(type) {
			case render.StillImage:
			case render.VersionedImage:
				if v.Version != 7 {
					t.Errorf("%s: version %d, want the source's 7", tc.name, v.Version)
				}
			default:
				t.Errorf("%s: drew a %T, which a GPU renderer cannot keep", tc.name, img)
			}
		}
	}
	if s, ok := heldImage(rgba).(render.StillImage); !ok || s.RGBA != rgba {
		t.Error("an *image.RGBA was copied or not held still")
	}
}

// A translucent swatch tiles one cached still checkerboard: the same
// texture paint after paint, never a fresh raster (#314).
func TestSwatchCheckerboardStaysOneTexture(t *testing.T) {
	s := NewColorSwatch(render.RGBA(200, 0, 0, 128))
	SizeWithin(s, Size{W: 24, H: 24})
	s.Arrange(render.Rect{W: 24, H: 24})
	first, second := drawnImages(s, 24, 24), drawnImages(s, 24, 24)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("drew %d then %d images, want the checkerboard once each", len(first), len(second))
	}
	a, ok := first[0].(render.StillImage)
	if b, okB := second[0].(render.StillImage); !ok || !okB || a.RGBA != b.RGBA {
		t.Errorf("checkerboards %T and %T are not one still raster", first[0], second[0])
	}
}

// A source decodes divided by the largest power of two that still
// gives its box every pixel it shows, under each fit; ImageNone, an
// unknown size or no box yet keep every pixel.
func TestSourceShrink(t *testing.T) {
	box := render.Rect{W: 96, H: 96}
	cover := image.Pt(3000, 3000)
	for _, tc := range []struct {
		natural image.Point
		box     render.Rect
		fit     render.ImageScale
		want    int
	}{
		{cover, box, render.ImageFit, 4},
		{cover, box, render.ImageCover, 4},
		{cover, box, render.ImageScaleDown, 4},
		{cover, box, render.ImageStretch, 4},
		{cover, box, render.ImageNone, 0},
		{image.Pt(3000, 300), box, render.ImageCover, 1},
		{image.Pt(150, 150), box, render.ImageFit, 0},
		{image.Point{}, box, render.ImageFit, 0},
		{cover, render.Rect{}, render.ImageFit, 0},
	} {
		got := sourceShrink(tc.natural, tc.box, tc.fit)
		if got != tc.want {
			t.Errorf("sourceShrink(%v, %v, %v) = %d, want %d", tc.natural, tc.box, tc.fit, got, tc.want)
			continue
		}
		if got > 0 {
			d := 1 << got
			kept := image.Pt((tc.natural.X+d-1)/d, (tc.natural.Y+d-1)/d)
			if src, dst := render.PlaceImage(kept.X, kept.Y, tc.box.W, tc.box.H, tc.fit, render.ImageCentered); src.Dx() < dst.W || src.Dy() < dst.H {
				t.Errorf("%v shrunk by %d keeps %v, too few pixels for a %dx%d draw", tc.natural, d, kept, dst.W, dst.H)
			}
		}
	}
}

// A file shown in a small box keeps a shrunk decode, measures at its
// natural size, and decodes finer once its box outgrows that (#412).
func TestFileImageKeepsADecodeTheSizeOfItsBox(t *testing.T) {
	resetImageCache(t)
	im := NewFileImage(writePNG(t, solidPNG(t, 1024, 512, color.NRGBA{R: 0xff, A: 0xff})))
	data, dev := paintImage(im, 64, 64, 1, 1)
	if got := px(data, dev, 32, 32); got != render.RGB(0xff, 0, 0) {
		t.Fatalf("the tile painted %v, want the picture's red", got)
	}
	if b := im.img.Bounds(); b.Dx() != 64 || b.Dy() != 32 {
		t.Errorf("a 64 px tile keeps a %dx%d decode of a 1024x512 picture, want 64x32", b.Dx(), b.Dy())
	}
	if got := SizeWithin(im, Size{W: 2000, H: 2000}); got != (Size{W: 1024, H: 512}) {
		t.Errorf("the shrunk picture measures %v, want its natural 1024x512", got)
	}
	paintImage(im, 600, 600, 1, 1)
	if b := im.img.Bounds(); b.Dx() < 600 {
		t.Errorf("a 600 px box still shows a %dx%d decode", b.Dx(), b.Dy())
	}
}

// An image over an in-memory picture drops the rasters resampled from
// it when it leaves the tree: its source key is its own, so nothing
// could hit them again. Content-keyed sources, which other widgets
// share, stay cached.
func TestRemovedImageDropsItsOwnRasters(t *testing.T) {
	resetImageCache(t)
	pic := image.NewRGBA(image.Rect(0, 0, 64, 64))
	own := NewImage(pic)
	shared := NewBytesImage(solidPNG(t, 64, 64, color.NRGBA{B: 0xff, A: 0xff}), "art")
	box := NewBox(Column, 0, 0)
	box.Append(own, false).Append(shared, false)
	for _, im := range []*Image{own, shared} {
		paintImage(im, 16, 16, 1, 1)
	}
	if _, rasters := imgcache.Default().Len(); rasters != 2 {
		t.Fatalf("%d rasters cached for two resampled images, want 2", rasters)
	}
	box.Remove(own)
	box.Remove(shared)
	if _, rasters := imgcache.Default().Len(); rasters != 1 {
		t.Errorf("%d rasters cached after removing both, want only the shared source's", rasters)
	}
}

// A picture is no Tab stop: focus passes from the button before it to
// the one after. With an OnPasteImage hook it is a paste target, and
// Tab lands on it.
func TestImageTakesFocusOnlyAsAPasteTarget(t *testing.T) {
	before, after := NewButton(NewBox(Row, 0, 0), 0, 0), NewButton(NewBox(Row, 0, 0), 0, 0)
	img := NewImage(solidImage(8, 8, color.NRGBA{A: 255}))
	box := NewBox(Row, 0, 0)
	box.Append(before, false)
	box.Append(img, false)
	box.Append(after, false)
	arrangeTree(t, box, 200, 50)
	r := &Router{Root: box}
	if err := r.SetFocus(img); !errors.Is(err, ErrNotFocusable) {
		t.Errorf("focusing a picture: %v, want ErrNotFocusable", err)
	}
	r.SetFocus(before)
	r.FocusNext()
	if r.Focused() != Widget(after) {
		t.Errorf("Tab from the button went to %T, want past the picture", r.Focused())
	}
	img.OnPasteImage = func(image.Image) {}
	r.SetFocus(before)
	r.FocusNext()
	if r.Focused() != Widget(img) {
		t.Errorf("Tab from the button went to %T, want the paste target", r.Focused())
	}
}
