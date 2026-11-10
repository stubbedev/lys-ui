package widget

import (
	"errors"
	"image"
	"sync/atomic"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/imgcache"
	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/widget/css"
)

// VideoFormat is the frames a VideoSurface carries: their size in
// pixels and how their YUV samples become RGB.
type VideoFormat struct {
	Width, Height int
	Color         media.YUVColor
}

// videoPlacement is what every surface a Video shows its frames on
// does: lie over the fitted frame, and go.
type videoPlacement interface {
	// Place shows the frames' src rectangle scaled to dst, a rect in
	// the window's logical coordinates; an empty dst hides them.
	Place(dst render.Rect, src image.Rectangle)
	Close()
}

// VideoSurface is the surface package app gives a Video on screen to
// show a stream's YUV frames as they are decoded, on a subsurface of
// its own below the window instead of in the window's pixels.
// Applications use Video instead.
type VideoSurface interface {
	videoPlacement
	// Present shows f, of the surface's VideoFormat. It is false, the
	// frame dropped, while every buffer is still the compositor's; the
	// wake the host was given runs once one is free again.
	Present(f *media.YUVFrame) (bool, error)
}

// TextureSurface is the surface package app gives a Video on screen
// whose stream decodes on the GPU: a subsurface the frames are drawn
// onto on Device, the GPU the windows draw with, which the stream
// decodes on. Applications use Video instead.
type TextureSurface interface {
	videoPlacement
	Device() *gpu.Device
	// DeviceLost tells the host that decoding found Device lost, with
	// the error that said so. The host answers as it does when its own
	// drawing finds it: through the lost and wake it was given.
	DeviceLost(err error)
	// Present draws f onto the surface. It is false, the frame
	// dropped, while every buffer is still the compositor's; the wake
	// the host was given runs once one is free again.
	Present(f *gpu.VideoFrame) (bool, error)
}

// VideoHost gives an on-screen Video its VideoSurface for frames of
// format, or says why it has none; wake runs on the loop when a
// buffer frees after a dropped frame.
type VideoHost func(v *Video, format VideoFormat, wake func()) (VideoSurface, error)

// TextureHost gives an on-screen Video of a stream decoding on the GPU
// its TextureSurface for width x height frames, or says why it has
// none; wake runs on the loop when a buffer frees after a dropped
// frame, and when the device opens after ErrVideoDevicePending. lost
// runs on the loop when the surface's device is going away, before it
// does: the Video closes the surface and has its stream let go of the
// device (media.Stream.ReleaseDevice), and the host's next wake has it
// ask for a surface again, on the device opened since or to learn that
// there is none.
type TextureHost func(v *Video, width, height int, wake, lost func()) (TextureSurface, error)

var (
	videoHost   atomic.Pointer[VideoHost]
	textureHost atomic.Pointer[TextureHost]
)

// SetVideoHost installs the host that gives on-screen Videos their
// surfaces; package app installs it. nil disables, and Videos paint
// their frames into the window.
func SetVideoHost(fn VideoHost) {
	if fn == nil {
		videoHost.Store(nil)
		return
	}
	videoHost.Store(&fn)
}

// SetTextureHost installs the host that gives on-screen Videos of
// streams decoding on the GPU their surfaces; package app installs it.
// nil disables, and such Videos fail their streams.
func SetTextureHost(fn TextureHost) {
	if fn == nil {
		textureHost.Store(nil)
		return
	}
	textureHost.Store(&fn)
}

// ErrVideoOffscreen is a VideoHost's answer for a Video not in a
// window yet: the Video asks again when it is next arranged.
var ErrVideoOffscreen = errors.New("widget: the Video is not in a window yet")

// ErrVideoDevicePending is a TextureHost's answer while the GPU the
// windows draw with is still opening: the Video asks again when the
// host calls its wake.
var ErrVideoDevicePending = errors.New("widget: the GPU device is still opening")

var (
	errNoVideoHost   = errors.New("widget: no video host is installed")
	errNoTextureHost = errors.New("widget: no host shows frames decoded on the GPU")
	errRGBAStream    = errors.New("widget: the stream decodes no 4:2:0 YUV")
)

// Video is GTK's GtkVideo: a media.Stream's frames fitted into the
// bounds by the scale policy (ImageFit by default), with
// MediaControls along the bottom that show while the pointer is over
// the video or the stream is not playing. On screen, a stream of YUV
// frames plays on a subsurface of its own when the host gives one
// (VideoSurface): its frames go to the compositor as decoded, with no
// conversion to RGBA and no repaint of the window, whose controls stay
// above it. Otherwise the frames are painted into the window. A stream
// decoding on the GPU always plays on a subsurface (TextureSurface),
// decoded on the device the windows draw with and drawn onto it there;
// when that device is lost the stream follows the windows to the one
// opened in its place, playing on from its position, and a Video that
// gets no such surface fails the stream. The Video does
// not own the stream; whoever made it closes it. It styles as `video`.
type Video struct {
	composite
	view     *videoView
	controls *MediaControls
	reveal   *Revealer
	hovered  bool
	autoplay bool
	unsub    func()
}

// NewVideo returns a video showing stream (nil for none), its controls
// in face at sizePx.
func NewVideo(face render.Font, sizePx float64, stream *media.Stream) *Video {
	v := &Video{controls: NewMediaControls(face, sizePx, nil)}
	v.view = &videoView{video: v}
	v.view.SetElement("picture")
	v.controls.AddClass(css.OSD)
	v.reveal = NewRevealer(v.controls)
	v.reveal.SetTransition(RevealSlideUp)
	over := NewOverlay()
	over.Append(v.view)
	over.AppendAligned(v.reveal, AlignFill, AlignEnd)
	v.initComposite(v, over)
	v.SetElement("video")
	v.SetOnHoverWithin(func(on bool) {
		v.hovered = on
		v.syncControls()
	})
	v.SetStream(stream)
	return v
}

// Stream returns the stream shown.
func (v *Video) Stream() *media.Stream { return v.view.stream }

// SetStream shows stream instead, playing it at once under autoplay.
func (v *Video) SetStream(stream *media.Stream) {
	if v.unsub != nil {
		v.unsub()
		v.unsub = nil
	}
	v.view.closeOffload()
	v.view.offloadErr = nil
	v.view.stream = stream
	v.view.painted, v.view.shown = 0, 0
	v.controls.SetStream(stream)
	if stream != nil {
		v.unsub = stream.Subscribe(v.changed)
		if v.autoplay {
			stream.Play()
		}
	}
	v.syncControls()
	v.view.InvalidateLayout()
}

// Controls returns the video's MediaControls.
func (v *Video) Controls() *MediaControls { return v.controls }

// SetAutoplay plays every stream as soon as it is shown, the current
// one included.
func (v *Video) SetAutoplay(on bool) {
	v.autoplay = on
	if on && v.view.stream != nil {
		v.view.stream.Play()
	}
}

// Autoplay reports whether streams play as soon as they are shown.
func (v *Video) Autoplay() bool { return v.autoplay }

// SetScale selects how frames fill the bounds: ImageFit (the
// default), ImageCover, ImageStretch, ImageScaleDown or ImageNone.
func (v *Video) SetScale(s ImageScale) {
	if s != v.view.scale {
		v.view.scale = s
		v.view.Invalidate()
		v.view.place()
	}
}

// Scale returns the fit policy.
func (v *Video) Scale() ImageScale { return v.view.scale }

// OffloadErr returns why the frames are painted into the window rather
// than shown on a VideoSurface, nil while they are on one or before
// the video is first on screen.
func (v *Video) OffloadErr() error { return v.view.offloadErr }

func (v *Video) changed() {
	v.view.changed()
	v.syncControls()
}

func (v *Video) syncControls() {
	s := v.view.stream
	show := s != nil && (v.hovered || !s.Playing())
	v.controls.setHidden(!show)
	v.reveal.SetRevealed(show)
}

type videoView struct {
	node
	video   *Video
	stream  *media.Stream
	scale   ImageScale
	painted uint64
	// offload shows the frames while the view has one, the frame
	// numbered shown last; offloadErr is why it has none.
	offload    videoPlacement
	offloadErr error
	// show presents the frame to show on offload.
	show   func() (bool, error)
	shown  uint64
	raster *image.RGBA
	// rasterID is raster's identity, a new one with every new raster.
	rasterID render.ImageID
	drawn    uint64
	src      image.Rectangle
	// version moves with every resample into raster.
	version uint64
}

func (p *videoView) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := p.measureHit(axis, forSize); ok {
		return sz
	}
	var sz Size
	if p.stream != nil {
		sz.W, sz.H = p.stream.Size()
	}
	return p.measureStore(axis, forSize, pictureRequest(sz, axis, forSize))
}

func (p *videoView) changed() {
	seq := p.stream.FrameSeq()
	switch {
	case p.offload != nil && seq != p.shown:
		p.present()
	case p.offload == nil && seq != p.painted:
		p.Invalidate()
	}
}

// Arrange places the offload surface over the fitted frame, opening
// it the first time the view lands on screen.
func (p *videoView) Arrange(r render.Rect) {
	arrangeVolatility++
	p.node.Arrange(r)
	p.place()
}

// fit is the frame's rect shown and where it lands, in logical
// coordinates.
func (p *videoView) fit() (image.Rectangle, render.Rect) {
	w, h := p.stream.Size()
	src, dst := render.PlaceImage(w, h, p.bounds.W, p.bounds.H, p.scale, render.ImageCentered)
	dst.X += p.bounds.X
	dst.Y += p.bounds.Y
	return src, dst
}

// place moves the offload surface to the fitted frame, hiding it while
// the frame has no room; a hidden surface shows nothing, so the frame
// is presented again once it has.
func (p *videoView) place() {
	if p.stream == nil {
		return
	}
	if p.offload == nil && p.offloadErr == nil && !p.bounds.Empty() {
		p.openOffload()
	}
	if p.offload == nil {
		return
	}
	src, dst := p.fit()
	if dst.Empty() {
		p.shown = 0
	}
	p.offload.Place(dst, src)
	if p.stream.FrameSeq() != p.shown {
		p.present()
	}
}

func (p *videoView) openOffload() {
	if p.stream.DecodesOnGPU() {
		p.openTextureOffload()
		return
	}
	host := videoHost.Load()
	if host == nil {
		p.offloadErr = errNoVideoHost
		return
	}
	c, ok := p.stream.YUV()
	if !ok {
		p.offloadErr = errRGBAStream
		return
	}
	w, h := p.stream.Size()
	s, err := (*host)(p.video, VideoFormat{Width: w, Height: h, Color: c}, p.wake)
	switch {
	case errors.Is(err, ErrVideoOffscreen):
		return
	case err != nil:
		p.offloadErr = err
		debug.Log("frame", "video frames paint into the window: %v", err)
		return
	}
	p.offload, p.shown = s, 0
	p.show = func() (bool, error) {
		f := p.stream.FrameYUV()
		if f == nil {
			return false, nil
		}
		return s.Present(f)
	}
	p.stream.SkipRGBA(true)
	p.Invalidate()
}

// openTextureOffload gives a stream decoding on the GPU the surface
// its frames are drawn onto and the device they decode on. A stream
// that gets none has nowhere to show its frames: it fails.
func (p *videoView) openTextureOffload() {
	host := textureHost.Load()
	if host == nil {
		p.failTextures(errNoTextureHost)
		return
	}
	w, h := p.stream.Size()
	s, err := (*host)(p.video, w, h, p.wake, p.deviceGone)
	switch {
	case errors.Is(err, ErrVideoOffscreen) || errors.Is(err, ErrVideoDevicePending):
		return
	case err != nil:
		p.failTextures(err)
		return
	}
	if err := p.stream.UseDevice(s.Device(), s.DeviceLost); err != nil {
		s.Close()
		p.offloadErr = err
		return
	}
	p.offload, p.shown = s, 0
	p.show = func() (bool, error) {
		f := p.stream.Texture()
		if f == nil {
			return false, nil
		}
		return s.Present(f)
	}
	p.Invalidate()
}

func (p *videoView) failTextures(err error) {
	p.offloadErr = err
	p.stream.Fail(err)
}

func (p *videoView) deviceGone() {
	p.closeOffload()
	p.stream.ReleaseDevice()
}

// wake is the host's call back: a buffer freed after a dropped frame,
// or the device a surface waited for opened.
func (p *videoView) wake() {
	switch {
	case p.stream == nil:
	case p.offload == nil && p.offloadErr == nil:
		p.place()
	case p.offload != nil && p.stream.FrameSeq() != p.shown:
		p.present()
	}
}

func (p *videoView) present() {
	shown, err := p.show()
	if err != nil {
		p.closeOffload()
		p.offloadErr = err
		if p.stream.DecodesOnGPU() {
			p.stream.Fail(err)
		}
		debug.Log("frame", "video surface failed: %v", err)
		return
	}
	if shown {
		p.shown = p.stream.FrameSeq()
		debug.Log("frame", "video frame %d shown on its surface", p.shown)
	}
}

// closeOffload returns the view to painting its frames.
func (p *videoView) closeOffload() {
	if p.offload == nil {
		return
	}
	p.offload.Close()
	p.offload, p.show = nil, nil
	p.stream.SkipRGBA(false)
	p.Invalidate()
}

func (p *videoView) detach() {
	p.closeOffload()
	p.offloadErr = nil
}

func (p *videoView) HitTest(pt Point) Widget { return p.HitLeaf(p, pt) }

func (p *videoView) Paint(cv *render.Canvas) {
	if p.stream == nil {
		return
	}
	if p.offload != nil {
		_, dst := p.fit()
		cv.Clear(dst, 0)
		return
	}
	frame := p.stream.Frame()
	if frame.RGBA == nil {
		return
	}
	seq := p.stream.FrameSeq()
	box := cv.MapRect(p.bounds)
	b := frame.Rect
	srcRect, dst := render.PlaceImage(b.Dx(), b.Dy(), box.W, box.H, p.scale, render.ImageCentered)
	if dst.Empty() {
		return
	}
	dw, dh, dx, dy := dst.W, dst.H, box.X+dst.X, box.Y+dst.Y
	if debug.Enabled && p.painted != seq {
		debug.Log("frame", "video frame %d shown in the window", seq)
	}
	p.painted = seq
	if srcRect.Dx() == dw && srcRect.Dy() == dh {
		cv.DrawImageDevice(frame.SubImage(srcRect), dx, dy)
		return
	}
	if frame.Version == 0 && frame.ID != 0 {
		raster, err := imgcache.Default().Raster(imgcache.RasterKey{Image: frame.ID, Src: srcRect, W: dw, H: dh}, func(dst *image.RGBA) error {
			return render.ResampleTo(dst, frame.RGBA, srcRect)
		})
		if err != nil {
			imageWarn(err)
			return
		}
		cv.DrawImageDevice(raster, dx, dy)
		return
	}
	if p.raster == nil || p.raster.Rect.Dx() != dw || p.raster.Rect.Dy() != dh {
		p.raster, p.rasterID, p.drawn = image.NewRGBA(image.Rect(0, 0, dw, dh)), render.NewImageID(), 0
	}
	if p.drawn != seq || p.src != srcRect {
		render.ResampleInto(p.raster, frame.RGBA, srcRect)
		p.drawn, p.src = seq, srcRect
		p.version++
	}
	cv.DrawImageDevice(render.VersionedImage{RGBA: p.raster, ID: p.rasterID, Version: p.version}, dx, dy)
}
