package widget

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-ui/internal/imgcache"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/transfer"
)

// ImageScale picks how an Image fills its arranged box. It aliases the
// render type so call sites never need render just for the constant.
type ImageScale = render.ImageScale

const (
	// ImageFit scales the image to fit the box with its aspect ratio
	// kept; the default.
	ImageFit = render.ImageFit
	// ImageCover fills the box completely, cropping the overflow.
	ImageCover = render.ImageCover
	// ImageNone draws the image at its natural size, one source pixel
	// per device pixel, centered and clipped.
	ImageNone = render.ImageNone
	// ImageStretch fills the box exactly, ignoring the aspect ratio.
	ImageStretch = render.ImageStretch
	// ImageScaleDown draws the natural size when it fits and shrinks to
	// fit (aspect kept) when it does not; it never enlarges.
	ImageScaleDown = render.ImageScaleDown
)

// imageKind says where an image's pixels come from, and with it
// whether decoding is synchronous or asynchronous.
type imageKind uint8

const (
	imageRaster imageKind = iota // decoded by the caller
	imageFile                    // fetched from a file path, async
	imageURL                     // fetched over http(s), async
	imageBytes                   // fetched from memory, decoded synchronously
)

// imageSource is one Image's pixel source. It is an immutable snapshot:
// the async decode goroutine reads a copy of it off the loop goroutine.
type imageSource struct {
	kind imageKind
	img  image.Image // imageRaster
	path string      // imageFile
	url  string      // imageURL
	data []byte      // imageBytes
	name string      // imageBytes: a name for decode error messages
	key  string      // imgcache source key; empty until a load resolves it
}

// imageFetchLimit caps how much of a URL body is read, so a runaway
// download cannot balloon the pixel cache.
const imageFetchLimit = 64 << 20

// imageHTTP bounds URL fetches; media servers stall more often than
// they lie about Content-Length.
var imageHTTP = &http.Client{Timeout: 15 * time.Second}

// memSourceSeq hands every in-memory source a distinct cache key.
// Pointers would alias after a garbage collection cycle; a fresh id
// cannot, and the entry dies with the source via InvalidateSource.
var memSourceSeq atomic.Uint64

// invoker delivers asynchronous load results onto the event-loop
// goroutine. Application creation installs Application.Invoke - the
// only sanctioned bridge (docs/threading.md). Without an invoker there
// is no loop to deliver to, and file/URL sources settle synchronously
// at their first paint instead.
var invoker atomic.Pointer[func(func())]

// SetInvoker installs the function asynchronous image loads use to run
// their completion on the event-loop goroutine; app calls it with
// Application.Invoke. Nil removes the bridge.
func SetInvoker(fn func(func())) {
	if fn == nil {
		invoker.Store(nil)
		return
	}
	invoker.Store(&fn)
}

func hasInvoker() bool { return invoker.Load() != nil }

func invoke(fn func()) {
	if p := invoker.Load(); p != nil {
		(*p)(fn)
		return
	}
	fn()
}

// Image paints a raster image - album art, an avatar, a rendered
// snapshot - into its arranged box. It is the raster counterpart of
// Icon, which covers the SVG side.
//
// Sources come in two decoding flavors:
//
//   - Synchronous: NewImage (a decoded image.Image) and NewBytesImage
//     decode where they are constructed and simply paint from the
//     first frame on.
//   - Asynchronous: NewFileImage and NewURLImage fetch and decode on a
//     goroutine the first time they paint. Until the load settles they
//     paint their placeholder color (transparent by default), and the
//     result lands back on the loop goroutine through the invoker -
//     Application.Invoke, the only sanctioned bridge (see
//     docs/threading.md). OnLoaded then fires exactly once for that
//     load, success or failure; check Err when it fires. Sources
//     settle synchronously when no invoker is installed (no
//     application running), and still fire OnLoaded - inline, on the
//     goroutine that painted.
//
// Pixels are resampled with the CatmullRom kernel to the device rect
// the policy places them in, so fractional scales stay crisp
// (stubbedev/lys-ui#14). Resamples are cached per (source, source rect,
// device size) in a shared LRU with a byte budget
// (internal/imgcache): painting the same image at the same size hits
// the cache and never rescales, and SetImage drops the old source's
// entries. A policy of ImageNone skips the cache entirely - nothing
// resamples.
type Image struct {
	node
	src imageSource

	// scale is the fit policy; fit-contain by default.
	scale ImageScale
	// placeholder fills the bounds while an async load is in flight;
	// the zero color paints nothing.
	placeholder Color
	// OnLoaded fires on the loop goroutine once per settled
	// asynchronous load, with the settled widget; check Err. It does
	// not fire for synchronous sources, which are ready at
	// construction.
	OnLoaded func(img *Image)
	// OnPasteImage fires on the loop goroutine when a pasted image
	// (the ctrl+v payload it takes as a ContentPaster) finished decoding:
	// the widget has already taken it as its source. The Image is the
	// paste surface — Entry and TextArea are text-only by design.
	OnPasteImage func(img image.Image)

	// load state: img is the decoded source, loadErr the sticky decode
	// failure, loaded/loading where the load stands, and gen bumps on
	// every source change so a result landing after SetImage is
	// dropped instead of overwriting the new source.
	img     image.Image
	loadErr error
	loaded  bool
	loading bool
	gen     uint64

	// play steps an animated source (image_anim.go).
	play imagePlayback
}

// NewImage returns an image widget around a decoded image. The widget
// keeps the reference; treat it as read-only after handoff. To show
// changed pixels, SetImage the raster again, as a render.VersionedImage
// at its next version so a GPU renderer updates its texture in place.
// A nil img paints nothing.
func NewImage(img image.Image) *Image {
	im := &Image{}
	im.setSource(imageSource{kind: imageRaster, img: img, key: memKey()})
	return im
}

// NewFileImage returns an image widget that loads an image file (png,
// jpeg, gif) on a goroutine at its first paint and draws it scaled by
// the widget's policy.
func NewFileImage(path string) *Image {
	return &Image{src: imageSource{kind: imageFile, path: path}}
}

// NewURLImage returns an image widget that fetches an image over
// http(s) on a goroutine at its first paint and draws it scaled by the
// widget's policy.
func NewURLImage(url string) *Image {
	return &Image{src: imageSource{kind: imageURL, url: url}}
}

// NewBytesImage returns an image widget around encoded image bytes
// (png, jpeg, gif), decoded synchronously from the cache when possible.
// name only labels decode errors.
func NewBytesImage(data []byte, name string) *Image {
	im := &Image{}
	im.setSource(imageSource{kind: imageBytes, data: data, name: name, key: hashKey(data)})
	return im
}

// SetImage swaps the source for a decoded image, dropping the previous
// source's cache entries and the measure caches that still describe
// the old size. The image is handed over as to NewImage.
func (im *Image) SetImage(img image.Image) {
	im.setSource(imageSource{kind: imageRaster, img: img, key: memKey()})
}

// SetFile swaps the source for a file path, reloaded asynchronously at
// the next paint.
func (im *Image) SetFile(path string) {
	im.setSource(imageSource{kind: imageFile, path: path})
}

// SetURL swaps the source for a URL, fetched asynchronously at the
// next paint.
func (im *Image) SetURL(url string) {
	im.setSource(imageSource{kind: imageURL, url: url})
}

// setSource installs src: in-flight results for the old source are
// dropped, the old source's cache entries go away, synchronous sources
// settle now, and the frame remeasures and repaints.
func (im *Image) setSource(src imageSource) {
	old := im.src.key
	if old != "" && old != src.key {
		imgcache.Default().InvalidateSource(old)
	}
	im.src = src
	im.gen++
	im.img, im.loadErr = nil, nil
	im.loaded, im.loading = false, false
	switch src.kind {
	case imageRaster:
		im.img, im.loaded = heldImage(src.img), true
	case imageBytes:
		img, err := imgcache.Default().Source(src.key, func() (image.Image, error) {
			return decodeImageData(src.data, src.name)
		})
		im.img, im.loadErr, im.loaded = img, err, true
	}
	im.restartPlayback()
	im.InvalidateLayout()
}

// SetScale selects how the image fills its box: ImageFit (the
// default), ImageCover, or ImageNone.
func (im *Image) SetScale(s ImageScale) {
	if s == im.scale {
		return
	}
	im.scale = s
	im.Invalidate()
}

// Scale returns the fit policy.
func (im *Image) Scale() ImageScale { return im.scale }

// SetPlaceholderColor sets what fills the bounds while an asynchronous
// load is in flight; the zero color (the default) paints nothing.
func (im *Image) SetPlaceholderColor(c Color) {
	if c == im.placeholder {
		return
	}
	im.placeholder = c
	im.Invalidate()
}

// PlaceholderColor returns what fills the bounds while an asynchronous
// load is in flight; the zero color paints nothing.
func (im *Image) PlaceholderColor() Color { return im.placeholder }

// Err returns the load failure of the current source: a missing or
// undecodable file, a failed fetch, or undecodable bytes. Nil until a
// load tried and failed, and sticky until SetImage or friends replace
// the source.
func (im *Image) Err() error { return im.loadErr }

// Loaded reports whether the current source has settled. Synchronous
// sources are loaded from construction on.
func (im *Image) Loaded() bool { return im.loaded }

// Measure returns the decoded image's natural size, clamped to con. An
// unsettled source measures 0 x 0; when the load lands, InvalidateLayout
// drops this cache so the next frame remeasures.
func (im *Image) Measure(con Constraints) Size {
	if sz, ok := im.measureHit(con); ok {
		return sz
	}
	w, h := 0, 0
	if im.img != nil {
		b := im.img.Bounds()
		w, h = b.Dx(), b.Dy()
	}
	return im.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Paint settles the source if needed, then draws: the placeholder
// while an async load is in flight, nothing after a failure, and
// otherwise the cached resample for the device rect the policy picked.
func (im *Image) Paint(cv *render.Canvas) {
	im.ensureSettled()
	if im.img == nil {
		if im.placeholder != 0 {
			cv.FillRect(im.bounds, im.placeholder)
		}
		return
	}
	box := cv.MapRect(im.bounds)
	pic, frame := im.currentFrame()
	if rgba, ok := pic.(*image.RGBA); ok {
		// An animation's frame: decoded once, never changed.
		pic = render.StillImage{RGBA: rgba}
	}
	b := pic.Bounds()
	fit, pos := im.placement(cv, box)
	srcRect, dst := render.PlaceImage(b.Dx(), b.Dy(), box.W, box.H, fit, pos)
	if dst.Empty() {
		return
	}
	dw, dh, dx, dy := dst.W, dst.H, box.X+dst.X, box.Y+dst.Y
	if srcRect.Dx() == dw && srcRect.Dy() == dh {
		// Natural pixels land one-to-one (ImageNone, a fitting
		// ImageScaleDown, any 1:1 fit); no resample, no cache entry.
		if sub, ok := pic.(interface {
			SubImage(r image.Rectangle) image.Image
		}); ok {
			cv.DrawImageDevice(sub.SubImage(srcRect), dx, dy)
			return
		}
	}
	if im.src.key == "" {
		cv.DrawImageDevice(render.Resample(pic, srcRect, dw, dh), dx, dy)
		return
	}
	raster := imgcache.Default().Raster(imgcache.RasterKey{
		Source: im.src.key,
		Frame:  frame,
		Src:    srcRect,
		W:      dw,
		H:      dh,
	}, func() *image.RGBA {
		return render.Resample(pic, srcRect, dw, dh)
	})
	cv.DrawImageDevice(render.StillImage{RGBA: raster}, dx, dy)
}

// placement is the scale policy and position the image draws by in
// the device rect box: object-fit and object-position where the
// stylesheet declares them (CSS wins), else SetScale's policy
// centered. A position's pixel offsets map to device pixels.
func (im *Image) placement(cv *render.Canvas, box render.Rect) (render.ImageScale, render.ImagePos) {
	v := im.style(im)
	fit, pos := im.scale, render.ImageCentered
	if v.Has(style.PropObjectFit) {
		fit = v.Layout.ObjectFit
	}
	if v.Has(style.PropObjectPosition) {
		x, y := v.Layout.ObjectPosition[0], v.Layout.ObjectPosition[1]
		off := cv.MapRect(render.Rect{X: im.bounds.X + int(math.Round(x.Px)), Y: im.bounds.Y + int(math.Round(y.Px))})
		pos = render.ImagePos{X: x.Frac, Y: y.Frac, OffX: off.X - box.X, OffY: off.Y - box.Y}
	}
	return fit, pos
}

// HitTest returns the image when p is inside its bounds.
func (im *Image) HitTest(p Point) Widget {
	return im.HitLeaf(im, p)
}

// PasteMimes implements ContentPaster: the image types, best first.
func (im *Image) PasteMimes() []string { return transfer.ImageMimes }

// PasteContent implements ContentPaster through PasteImage.
func (im *Image) PasteContent(mime string, data []byte) { im.PasteImage(data, mime) }

// PasteImage is the ctrl+v image path: the pasted encoded bytes (with
// the mime they arrived as, for error messages) become the widget's
// source, decoded off the loop goroutine through the same invoker
// bridge as file loads — a large PNG never stalls the loop — and then
// handed to OnPasteImage. Without an invoker the decode settles
// synchronously, like every other source.
func (im *Image) PasteImage(data []byte, mime string) {
	gen := im.gen + 1
	im.gen = gen
	im.img, im.loadErr = nil, nil
	im.loaded = false
	im.InvalidateLayout()
	if !hasInvoker() {
		img, err := decodeImageData(data, "pasted "+mime)
		im.applyPaste(gen, img, err)
		return
	}
	im.loading = true
	go func() {
		img, err := decodeImageData(data, "pasted "+mime)
		invoke(func() { im.applyPaste(gen, img, err) })
	}()
}

// applyPaste records a settled paste on the loop goroutine and fires
// OnPasteImage; a superseded source drops the result, like
// applyLoad.
func (im *Image) applyPaste(gen uint64, img image.Image, err error) {
	if gen != im.gen {
		return
	}
	im.img, im.loadErr, im.loaded, im.loading = img, err, true, false
	im.restartPlayback()
	if err == nil && im.OnPasteImage != nil {
		im.OnPasteImage(img)
	}
	im.InvalidateLayout()
}

// KeyAction implements KeyActionHandler: an Image with an
// OnPasteImage hook is a paste target, so it joins focus traversal —
// the actions themselves are none of its own.
func (im *Image) KeyAction(KeyAction, Mods) {}

// ensureSettled kicks the first load of a file/URL source. Without an
// invoker installed there is no loop goroutine to deliver through, so
// the load settles synchronously instead - the paint itself is
// loop-goroutine code whenever it runs inside Run.
func (im *Image) ensureSettled() {
	if im.loaded || im.loading {
		return
	}
	if im.src.kind != imageFile && im.src.kind != imageURL {
		return
	}
	if !hasInvoker() {
		key, img, err := loadImage(im.src)
		im.applyLoad(im.gen, key, img, err)
		return
	}
	im.loading = true
	gen := im.gen
	src := im.src
	go func() {
		key, img, err := loadImage(src)
		invoke(func() { im.applyLoad(gen, key, img, err) })
	}()
}

// applyLoad records a settled load on the loop goroutine. Results for
// a superseded source (the gen moved on via SetImage and friends) are
// dropped: they must not overwrite the new source's state.
func (im *Image) applyLoad(gen uint64, key string, img image.Image, err error) {
	if gen != im.gen {
		return
	}
	im.loading = false
	im.loaded = true
	im.src.key = key
	im.img, im.loadErr = img, err
	im.restartPlayback()
	// The natural size may have appeared or changed: drop the measure
	// caches up the tree and owe the frame a repaint.
	im.InvalidateLayout()
	if fn := im.OnLoaded; fn != nil {
		fn(im)
	}
}

// loadImage fetches and decodes src off the loop goroutine; it reads
// only its snapshot and the shared cache, never widget state.
func loadImage(src imageSource) (key string, img image.Image, err error) {
	data, err := fetchSource(src)
	if err != nil {
		return "", nil, err
	}
	key = hashKey(data)
	img, err = imgcache.Default().Source(key, func() (image.Image, error) {
		return decodeImageData(data, src.name)
	})
	return key, img, err
}

func fetchSource(src imageSource) ([]byte, error) {
	switch src.kind {
	case imageFile:
		return os.ReadFile(src.path)
	case imageURL:
		return fetchURL(src.url)
	default:
		return src.data, nil
	}
}

func fetchURL(url string) ([]byte, error) {
	resp, err := imageHTTP.Get(url)
	if err != nil {
		return nil, fmt.Errorf("image: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image: fetch %s: status %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, imageFetchLimit))
	if err != nil {
		return nil, fmt.Errorf("image: fetch %s: %w", url, err)
	}
	return data, nil
}

// decodeImageData decodes image bytes - PNG, JPEG, GIF, WebP, and
// animated GIF and APNG as a render.Animation (render.DecodeImage).
func decodeImageData(data []byte, name string) (image.Image, error) {
	img, err := render.DecodeImage(data)
	if err != nil {
		return nil, fmt.Errorf("image: decode %s: %w", name, err)
	}
	return heldImage(img), nil
}

// heldImage is img as a widget holds an image handed to it read-only:
// a still, versioned or animated image as it is, an *image.RGBA as a
// still image over the same pixels, and any other image converted once
// (render.Still), so a GPU renderer keeps each as a texture.
func heldImage(img image.Image) image.Image {
	switch v := img.(type) {
	case nil, render.StillImage, render.VersionedImage, *render.Animation:
		return img
	case *image.RGBA:
		return render.StillImage{RGBA: v}
	}
	return render.Still(img)
}

// hashKey builds a source key from content: a changed file or URL body
// gets a fresh key and a fresh decode without any invalidation
// protocol. 128 bits of digest keep collisions theoretical.
func hashKey(data []byte) string {
	sum := sha256.Sum256(data)
	return "raw:" + hex.EncodeToString(sum[:16])
}

func memKey() string {
	return "mem:" + strconv.FormatUint(memSourceSeq.Add(1), 10)
}
