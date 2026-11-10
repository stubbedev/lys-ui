package app

import (
	"errors"
	"fmt"
	"image"
	"slices"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/dmabufwl"
	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/udmabuf"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/widget"
)

var (
	// ErrNoViewporter reports a compositor without wp_viewporter, which
	// a Video's surface scales its frames with.
	ErrNoViewporter = errors.New("app: the compositor has no wp_viewporter")
	// ErrNoYUVDmabuf reports a compositor that imports neither linear
	// YUV420 nor linear NV12 dmabufs.
	ErrNoYUVDmabuf = errors.New("app: the compositor imports no linear YUV420 or NV12 dmabufs")
	// ErrNoColorRepresentation reports a compositor without
	// wp_color_representation_v1, so a Video's YUV samples would be
	// read with a matrix and range the compositor guesses.
	ErrNoColorRepresentation = errors.New("app: the compositor has no wp_color_representation_v1")
	// ErrVideoSurfaceGone reports a Present on a closed video surface.
	ErrVideoSurfaceGone = errors.New("app: the video surface is closed")
)

// videoRing is how many buffers a video surface cycles through: one on
// screen, one the compositor may still read, one being filled. A frame
// arriving while none is free is dropped, never allocated for.
const videoRing = 3

// videoStrideAlign is the row alignment of a video buffer's planes,
// the strictest a GPU importing linear dmabufs asks for (AMD).
const videoStrideAlign = 256

// videoFormats are the YUV dmabuf formats a video surface writes, in
// preference order: YUV420 holds a frame's planes as decoded, NV12
// interleaves its chroma.
var videoFormats = []fourcc.Format{fourcc.YUV420, fourcc.NV12}

// videoPlane is where one plane of a video buffer lies.
type videoPlane struct{ offset, stride int }

// videoLayout is how a frame lies in a video buffer: its format, its
// planes and the bytes they span.
type videoLayout struct {
	format        fourcc.Format
	width, height int
	planes        []videoPlane
	size          int
}

func alignStride(n int) int { return (n + videoStrideAlign - 1) &^ (videoStrideAlign - 1) }

func newVideoLayout(format fourcc.Format, width, height int) videoLayout {
	cw, ch := (width+1)/2, (height+1)/2
	l := videoLayout{format: format, width: width, height: height}
	luma := videoPlane{stride: alignStride(width)}
	chromaAt := luma.stride * height
	if format == fourcc.NV12 {
		uv := videoPlane{offset: chromaAt, stride: alignStride(2 * cw)}
		l.planes, l.size = []videoPlane{luma, uv}, uv.offset+uv.stride*ch
		return l
	}
	u := videoPlane{offset: chromaAt, stride: alignStride(cw)}
	v := videoPlane{offset: u.offset + u.stride*ch, stride: u.stride}
	l.planes, l.size = []videoPlane{luma, u, v}, v.offset+v.stride*ch
	return l
}

// write copies f's planes into dst, a buffer of the layout's size,
// touching no byte outside them: width x height x 1.5 bytes for even
// sizes.
func (l videoLayout) write(dst []byte, f *media.YUVFrame) {
	y, u, v := f.Planes()
	cw, ch := f.ChromaSize()
	copyPlane(dst, l.planes[0], y, f.Width, f.Height)
	if l.format != fourcc.NV12 {
		copyPlane(dst, l.planes[1], u, cw, ch)
		copyPlane(dst, l.planes[2], v, cw, ch)
		return
	}
	p := l.planes[1]
	for row := range ch {
		out := dst[p.offset+row*p.stride:][:2*cw]
		ur, vr := u[row*cw:][:cw], v[row*cw:][:cw]
		for x := range cw {
			out[2*x], out[2*x+1] = ur[x], vr[x]
		}
	}
}

func copyPlane(dst []byte, p videoPlane, src []byte, w, h int) {
	if p.stride == w {
		copy(dst[p.offset:][:w*h], src)
		return
	}
	for row := range h {
		copy(dst[p.offset+row*p.stride:][:w], src[row*w:][:w])
	}
}

// dmabuf describes a buffer of the layout in the dmabuf fd.
func (l videoLayout) dmabuf(fd int) dmabuf.Buffer {
	b := dmabuf.Buffer{Width: l.width, Height: l.height, Format: l.format, Modifier: fourcc.Linear}
	for _, p := range l.planes {
		b.Planes = append(b.Planes, dmabuf.Plane{FD: fd, Offset: uint32(p.offset), Stride: uint32(p.stride)}) //nolint:gosec // a frame the compositor imports is far below 4 GiB
	}
	return b
}

// videoMemory is a video buffer's memory: the dmabuf the compositor
// imports, the CPU's mapping of it, and the bracket around a write
// through the mapping.
type videoMemory interface {
	fd() int
	bytes() []byte
	beginWrite() error
	endWrite() error
	close() error
}

// udmabufMemory is video buffer memory from /dev/udmabuf.
type udmabufMemory struct{ b *udmabuf.Buffer }

func newUdmabufMemory(size int) (videoMemory, error) {
	b, err := udmabuf.New(size)
	if err != nil {
		return nil, err
	}
	return udmabufMemory{b}, nil
}

func (m udmabufMemory) fd() int           { return m.b.FD }
func (m udmabufMemory) bytes() []byte     { return m.b.Mem }
func (m udmabufMemory) beginWrite() error { return dmabuf.BeginCPUWrite(m.b.FD) }
func (m udmabufMemory) endWrite() error   { return dmabuf.EndCPUWrite(m.b.FD) }
func (m udmabufMemory) close() error      { return m.b.Close() }

// colorRepresentationOf is the wp_color_representation_v1 pair for c.
func colorRepresentationOf(c media.YUVColor) wlsession.ColorRepresentation {
	r := wlsession.ColorRepresentation{Coefficients: wlr.WpColorRepresentationSurfaceV1CoefficientsBt601, Range: wlr.WpColorRepresentationSurfaceV1RangeLimited}
	if c.Range == media.FullRange {
		r.Range = wlr.WpColorRepresentationSurfaceV1RangeFull
	}
	return r
}

// videoFormatFor is the first of videoFormats the compositor imports
// linear.
func (a *Application) videoFormatFor() (fourcc.Format, error) {
	if a.sess.Dmabuf() == nil {
		return 0, ErrNoDmabuf
	}
	for _, f := range videoFormats {
		if a.sess.DmabufImports(wlsession.DmabufLayout{Format: f, Modifier: fourcc.Linear}) {
			return f, nil
		}
	}
	return 0, ErrNoYUVDmabuf
}

func (a *Application) videoSurfaceFor(v *widget.Video, format widget.VideoFormat, wake func()) (widget.VideoSurface, error) {
	hw, subc, err := a.videoWindow(v)
	if err != nil {
		return nil, err
	}
	pixel, err := a.videoFormatFor()
	if err != nil {
		return nil, err
	}
	s := &videoSurface{layout: newVideoLayout(pixel, format.Width, format.Height)}
	if err := s.open(a, hw, subc, wake); err != nil {
		return nil, err
	}
	if err := s.init(format.Color); err != nil {
		s.Close()
		return nil, err
	}
	debug.Log("frame", "video surface %d: %dx%d %s", s.surf.Id(), format.Width, format.Height, pixel)
	return s, nil
}

// videoWindow is the window v shows in, for a subsurface of its frames,
// and the subcompositor that makes one: ErrVideoOffscreen before v is
// in a window, an error for a compositor that cannot place and scale a
// subsurface.
func (a *Application) videoWindow(v *widget.Video) (*hostWindow, *wl.Subcompositor, error) {
	hw := a.windowOf(v)
	if hw == nil || hw.host == nil {
		return nil, nil, widget.ErrVideoOffscreen
	}
	subc := a.sess.Subcompositor()
	if subc == nil {
		return nil, nil, ErrNoSubsurfaces
	}
	if a.sess.Viewporter() == nil {
		return nil, nil, ErrNoViewporter
	}
	return hw, subc, nil
}

// videoOutput is what every Video subsurface is: a fixed ring of
// dmabufs the compositor imports, scaled by the viewport to the
// video's placement. A frame arriving while every buffer is the
// compositor's is dropped, and the next buffer freed wakes the Video.
type videoOutput struct {
	app *Application
	hw  *hostWindow
	subsurface
	// device is the GPU a texture surface draws on, nil for YUV.
	device *gpu.Device
	ring   [videoRing]videoBuffer
	wake   func()
	bounds render.Rect
	src    image.Rectangle
	// dropped is a frame dropped for want of a free buffer: the next
	// one freed wakes the Video.
	dropped bool
	err     error
	closed  bool
}

func (o *videoOutput) open(a *Application, hw *hostWindow, subc *wl.Subcompositor, wake func()) error {
	parent := hw.host.HostSurface()
	ss, err := a.newSubsurface(subc, parent)
	if err != nil {
		return fmt.Errorf("app: video: %w", err)
	}
	o.app, o.hw, o.subsurface, o.wake = a, hw, ss, wake
	for i := range o.ring {
		o.ring[i].out = o
	}
	hw.video = append(hw.video, o)
	return nil
}

func (o *videoOutput) check(err error) {
	if err != nil {
		debug.Log("frame", "video surface %d: %v", o.surf.Id(), err)
	}
}

func (o *videoOutput) Place(dst render.Rect, src image.Rectangle) {
	if o.closed || dst == o.bounds && src == o.src {
		return
	}
	hide := dst.Empty() && !o.bounds.Empty()
	o.bounds, o.src = dst, src
	o.hw.dirty = true
	if hide {
		o.check(o.surf.Attach(nil, 0, 0))
		o.check(o.surf.Commit())
		return
	}
	if dst.Empty() {
		return
	}
	o.check(o.sub.SetPosition(intconv.Clamp[int32](dst.X), intconv.Clamp[int32](dst.Y)))
	o.check(o.vp.SetSource(float32(src.Min.X), float32(src.Min.Y), float32(src.Dx()), float32(src.Dy())))
	o.check(o.vp.SetDestination(intconv.Clamp[int32](dst.W), intconv.Clamp[int32](dst.H)))
	o.check(o.setOpaque(dst.W, dst.H))
	o.check(o.surf.Commit())
}

func (o *videoOutput) setOpaque(w, h int) error {
	region, err := o.app.sess.Compositor().CreateRegion()
	if err != nil {
		return fmt.Errorf("app: video opaque region: %w", err)
	}
	defer func() { _ = region.Destroy() }()
	if err := region.Add(0, 0, intconv.Clamp[int32](w), intconv.Clamp[int32](h)); err != nil {
		return fmt.Errorf("app: video opaque region add: %w", err)
	}
	return o.surf.SetOpaqueRegion(region)
}

// writable is the index of a buffer to present a frame in, -1 when the
// frame is dropped: while the video is hidden, or every buffer is the
// compositor's.
func (o *videoOutput) writable() (int, error) {
	switch {
	case o.closed:
		return -1, ErrVideoSurfaceGone
	case o.err != nil:
		return -1, o.err
	case o.bounds.Empty():
		return -1, nil
	}
	for i := range o.ring {
		if b := &o.ring[i]; b.wl != nil && !b.busy {
			return i, nil
		}
	}
	o.dropped = true
	return -1, nil
}

// show attaches buffer i, a w x h frame, and commits it.
func (o *videoOutput) show(i, w, h int) error {
	b := &o.ring[i]
	if err := o.surf.Attach(b.wl, 0, 0); err != nil {
		return fmt.Errorf("app: video attach: %w", err)
	}
	if err := o.surf.DamageBuffer(0, 0, intconv.Clamp[int32](w), intconv.Clamp[int32](h)); err != nil {
		return fmt.Errorf("app: video damage: %w", err)
	}
	if err := o.surf.Commit(); err != nil {
		return fmt.Errorf("app: video commit: %w", err)
	}
	b.busy = true
	return nil
}

// freed wakes the Video when a buffer frees after a dropped frame.
func (o *videoOutput) freed() {
	if !o.dropped || o.closed {
		return
	}
	o.dropped = false
	o.wake()
}

// close destroys the buffers and the subsurface; release frees what
// backs the buffers once the compositor's handles to them are gone.
func (o *videoOutput) close(release func()) {
	if o.closed {
		return
	}
	o.closed = true
	for i := range o.ring {
		o.ring[i].drop()
	}
	release()
	o.app.sess.SetSurfaceInput(o.surf, nil)
	o.destroy()
	o.hw.video = slices.DeleteFunc(o.hw.video, func(v *videoOutput) bool { return v == o })
	o.hw.dirty = true
}

// videoSurface is a Video's subsurface for a stream's YUV frames: its
// ring is linear YUV dmabufs the frames are written into, tagged with
// the stream's colour.
type videoSurface struct {
	videoOutput
	color  *wlr.WpColorRepresentationSurfaceV1
	layout videoLayout
	mem    [videoRing]videoMemory
}

func (s *videoSurface) init(c media.YUVColor) error {
	var err error
	s.color, err = s.app.sess.NewColorRepresentation(s.surf, colorRepresentationOf(c))
	switch {
	case errors.Is(err, wlsession.ErrProtocolUnavailable):
		return ErrNoColorRepresentation
	case err != nil:
		return fmt.Errorf("app: video: %w", err)
	}
	if err := setVideoContent(s.app, s.surf); err != nil {
		return err
	}
	for i := range s.ring {
		mem, err := s.app.videoMemory(s.layout.size)
		if err != nil {
			return fmt.Errorf("app: video buffer %d of %d: %w", i+1, videoRing, err)
		}
		s.mem[i] = mem
		if err := s.ring[i].importDmabuf(s.layout.dmabuf(mem.fd())); err != nil {
			return fmt.Errorf("app: video buffer %d of %d: %w", i+1, videoRing, err)
		}
	}
	return nil
}

func setVideoContent(a *Application, surf *wl.Surface) error {
	if err := a.sess.SetContentType(surf, wlsession.ContentVideo); err != nil && !errors.Is(err, wlsession.ErrProtocolUnavailable) {
		return fmt.Errorf("app: video content type: %w", err)
	}
	return nil
}

func (s *videoSurface) Present(f *media.YUVFrame) (bool, error) {
	if f.Width != s.layout.width || f.Height != s.layout.height {
		return false, fmt.Errorf("app: a %dx%d frame on a %dx%d video surface", f.Width, f.Height, s.layout.width, s.layout.height)
	}
	i, err := s.writable()
	if i < 0 {
		return false, err
	}
	mem := s.mem[i]
	if err := mem.beginWrite(); err != nil {
		return false, fmt.Errorf("app: video frame: %w", err)
	}
	s.layout.write(mem.bytes(), f)
	if err := mem.endWrite(); err != nil {
		return false, fmt.Errorf("app: video frame: %w", err)
	}
	if err := s.show(i, f.Width, f.Height); err != nil {
		return false, err
	}
	return true, nil
}

func (s *videoSurface) Close() {
	s.close(func() {
		for i, m := range s.mem {
			if m != nil {
				_ = m.close()
				s.mem[i] = nil
			}
		}
		if s.color != nil {
			_ = s.color.Destroy()
		}
	})
}

// videoBuffer is one buffer of a video surface's ring.
type videoBuffer struct {
	out    *videoOutput
	desc   dmabuf.Buffer
	params *wlr.ZwpBufferParamsV1
	wl     *wl.Buffer
	busy   bool
}

func (b *videoBuffer) importDmabuf(d dmabuf.Buffer) error {
	b.desc = d
	params, err := dmabufwl.Params(b.out.app.sess.Dmabuf(), d)
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	b.params = params
	params.AddCreatedHandler(b)
	params.AddFailedHandler(b)
	w, errW := intconv.Exact[int32](d.Width)
	h, errH := intconv.Exact[int32](d.Height)
	if err := errors.Join(errW, errH); err != nil {
		return fmt.Errorf("app: video size: %w", err)
	}
	if err := params.Create(w, h, uint32(d.Format), 0); err != nil {
		return fmt.Errorf("app: dmabuf create: %w", err)
	}
	return nil
}

func (b *videoBuffer) HandleZwpBufferParamsV1Created(ev wlr.ZwpBufferParamsV1CreatedEvent) {
	b.finishImport()
	if b.out.closed {
		_ = ev.Buffer.Destroy()
		return
	}
	b.wl = ev.Buffer
	b.wl.AddReleaseHandler(b)
	b.out.freed()
}

func (b *videoBuffer) HandleZwpBufferParamsV1Failed(wlr.ZwpBufferParamsV1FailedEvent) {
	b.finishImport()
	if b.out.err == nil {
		b.out.err = fmt.Errorf("%w: %dx%d format %s modifier %s", ErrDmabufRejected, b.desc.Width, b.desc.Height, b.desc.Format, b.desc.Modifier)
	}
	b.out.freed()
}

func (b *videoBuffer) HandleBufferRelease(wl.BufferReleaseEvent) {
	b.busy = false
	b.out.freed()
}

func (b *videoBuffer) finishImport() {
	if b.params != nil {
		_ = b.params.Destroy()
		b.params = nil
	}
}

func (b *videoBuffer) drop() {
	b.finishImport()
	if b.wl != nil {
		_ = b.wl.Destroy()
		b.wl = nil
	}
	b.busy = false
}
