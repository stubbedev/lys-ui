package widget

import (
	"image"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/widget/css"
)

// Video is GTK's GtkVideo: a media.Stream's frames fitted into the
// bounds by the scale policy (ImageFit by default), with
// MediaControls along the bottom that show while the pointer is over
// the video or the stream is not playing. The Video does not own the
// stream; whoever made it closes it. It styles as `video`.
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
	v := &Video{view: &videoView{}, controls: NewMediaControls(face, sizePx, nil)}
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
	v.view.stream = stream
	v.view.painted = 0
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
	}
}

// Scale returns the fit policy.
func (v *Video) Scale() ImageScale { return v.view.scale }

func (v *Video) changed() {
	if v.view.stream.Frame().Version != v.view.painted {
		v.view.Invalidate()
	}
	v.syncControls()
}

func (v *Video) syncControls() {
	s := v.view.stream
	v.reveal.SetRevealed(s != nil && (v.hovered || !s.Playing()))
}

type videoView struct {
	node
	stream  *media.Stream
	scale   ImageScale
	painted uint64
	raster  *image.RGBA
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

func (p *videoView) HitTest(pt Point) Widget { return p.HitLeaf(p, pt) }

func (p *videoView) Paint(cv *render.Canvas) {
	if p.stream == nil {
		return
	}
	frame := p.stream.Frame()
	if frame.RGBA == nil {
		return
	}
	seq := frame.Version
	box := cv.MapRect(p.bounds)
	b := frame.Rect
	srcRect, dst := render.PlaceImage(b.Dx(), b.Dy(), box.W, box.H, p.scale, render.ImageCentered)
	if dst.Empty() {
		return
	}
	dw, dh, dx, dy := dst.W, dst.H, box.X+dst.X, box.Y+dst.Y
	if debug.Enabled && p.painted != seq {
		debug.Log("frame", "video frame %d painted", seq)
	}
	p.painted = seq
	if srcRect.Dx() == dw && srcRect.Dy() == dh {
		cv.DrawImageDevice(frame.SubImage(srcRect), dx, dy)
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
