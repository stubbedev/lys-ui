package app

import (
	"errors"
	"fmt"
	"image"
	"slices"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/dmabufwl"
	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

var (
	// ErrNoSubsurfaces reports a compositor without wl_subcompositor,
	// where a GPUArea cannot have a surface of its own.
	ErrNoSubsurfaces = errors.New("app: the compositor has no wl_subcompositor")
	// ErrNoDmabuf reports a compositor without zwp_linux_dmabuf_v1
	// (version 2 or later): GPUArea imports no dmabufs and presents CPU
	// images only.
	ErrNoDmabuf = errors.New("app: the compositor has no linux-dmabuf")
	// ErrDmabufLayout reports a format and modifier pair the compositor
	// did not advertise; importing it would be a fatal protocol error.
	ErrDmabufLayout = errors.New("app: the compositor does not import this dmabuf format and modifier")
	// ErrDmabufRejected reports a dmabuf the compositor failed to
	// import.
	ErrDmabufRejected = errors.New("app: the compositor rejected the dmabuf")
	// ErrGPUSurfaceGone reports a buffer imported for a surface that
	// has since closed (the area left the screen or the window closed):
	// import it again in the next OnFrame.
	ErrGPUSurfaceGone = errors.New("app: the GPUArea surface this buffer belonged to is gone")

	errForeignBuffer   = errors.New("app: the buffer was imported by another GPUArea")
	errBufferDestroyed = errors.New("app: the GPUArea buffer was destroyed")
)

func (a *Application) gpuSurfaceFor(area *widget.GPUArea) (widget.GPUSurface, error) {
	hw := a.windowOf(area)
	if hw == nil || hw.host == nil {
		return nil, errors.New("app: the GPUArea is not in a window")
	}
	sub := a.sess.Subcompositor()
	if sub == nil {
		return nil, ErrNoSubsurfaces
	}
	parent := hw.host.HostSurface()
	surf, err := a.sess.Compositor().CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("app: GPUArea surface: %w", err)
	}
	s := &gpuSurface{app: a, hw: hw, area: area, surf: surf}
	if err := s.init(sub, parent); err != nil {
		s.Close()
		return nil, err
	}
	hw.gpu = append(hw.gpu, s)
	debug.Log("frame", "GPUArea surface %d below %d", surf.Id(), parent.Id())
	return s, nil
}

type gpuSurface struct {
	app     *Application
	hw      *hostWindow
	area    *widget.GPUArea
	surf    *wl.Surface
	sub     *wl.Subsurface
	vp      *wlr.WpViewport
	bounds  render.Rect
	armed   bool
	owed    bool
	waiting *gpuBuffer
	buffers []*gpuBuffer
	closed  bool
}

func (s *gpuSurface) init(sub *wl.Subcompositor, parent *wl.Surface) error {
	var err error
	if s.sub, err = sub.GetSubsurface(s.surf, parent); err != nil {
		return fmt.Errorf("app: get_subsurface: %w", err)
	}
	if err := s.sub.PlaceBelow(parent); err != nil {
		return fmt.Errorf("app: subsurface place_below: %w", err)
	}
	if err := s.sub.SetDesync(); err != nil {
		return fmt.Errorf("app: subsurface set_desync: %w", err)
	}
	region, err := s.app.sess.Compositor().CreateRegion()
	if err != nil {
		return fmt.Errorf("app: GPUArea input region: %w", err)
	}
	err = s.surf.SetInputRegion(region)
	_ = region.Destroy()
	if err != nil {
		return fmt.Errorf("app: GPUArea set_input_region: %w", err)
	}
	if vp := s.app.sess.Viewporter(); vp != nil {
		if s.vp, err = vp.GetViewport(s.surf); err != nil {
			return fmt.Errorf("app: GPUArea viewport: %w", err)
		}
	}
	return nil
}

func (s *gpuSurface) deviceSize() widget.Size {
	if s.vp == nil {
		k := scale.IntegerScale(s.hw.frac120)
		return widget.Size{W: s.bounds.W * k, H: s.bounds.H * k}
	}
	return widget.Size{W: scale.DeviceSize(s.bounds.W, s.hw.frac120), H: scale.DeviceSize(s.bounds.H, s.hw.frac120)}
}

func (s *gpuSurface) Place(r render.Rect) {
	if s.closed || r == s.bounds {
		return
	}
	moved, resized := r.X != s.bounds.X || r.Y != s.bounds.Y, r.W != s.bounds.W || r.H != s.bounds.H
	s.bounds = r
	if moved {
		s.check(s.sub.SetPosition(intconv.Clamp[int32](r.X), intconv.Clamp[int32](r.Y)))
	}
	if resized {
		s.applyScale()
		s.check(s.surf.Commit())
		s.RequestFrame()
	}
}

func (s *gpuSurface) applyScale() {
	if s.vp != nil {
		s.check(s.vp.SetDestination(intconv.Clamp[int32](s.bounds.W), intconv.Clamp[int32](s.bounds.H)))
		return
	}
	s.check(s.surf.SetBufferScale(intconv.Clamp[int32](scale.IntegerScale(s.hw.frac120))))
}

func (s *gpuSurface) rescaled() {
	if s.closed || s.bounds.Empty() {
		return
	}
	s.applyScale()
	s.check(s.surf.Commit())
	s.RequestFrame()
}

func (s *gpuSurface) check(err error) {
	if err != nil {
		debug.Log("frame", "GPUArea surface %d: %v", s.surf.Id(), err)
	}
}

func (s *gpuSurface) RequestFrame() {
	if s.closed || s.armed || s.owed {
		return
	}
	s.owed = true
	s.app.Invoke(s.deliver)
}

func (s *gpuSurface) deliver() {
	s.owed = false
	if s.closed || s.bounds.Empty() || s.area.OnFrame == nil {
		return
	}
	s.area.OnFrame(widget.GPUFrame{Size: s.deviceSize()})
}

func (s *gpuSurface) HandleCallbackDone(ev wl.CallbackDoneEvent) {
	ev.C.Unregister()
	s.armed = false
	if !s.owed {
		s.deliver()
	}
}

func (s *gpuSurface) commit(b *wl.Buffer, w, h int) error {
	if err := s.surf.Attach(b, 0, 0); err != nil {
		return fmt.Errorf("app: GPUArea attach: %w", err)
	}
	if err := s.surf.DamageBuffer(0, 0, intconv.Clamp[int32](w), intconv.Clamp[int32](h)); err != nil {
		return fmt.Errorf("app: GPUArea damage: %w", err)
	}
	if !s.armed {
		cb, err := s.surf.Frame()
		if err != nil {
			return fmt.Errorf("app: GPUArea frame callback: %w", err)
		}
		cb.AddDoneHandler(s)
		s.armed = true
	}
	if err := s.surf.Commit(); err != nil {
		return fmt.Errorf("app: GPUArea commit: %w", err)
	}
	debug.Log("frame", "GPUArea frame committed %dx%d", w, h)
	return nil
}

func (s *gpuSurface) PresentImage(img *image.RGBA) error {
	if s.closed {
		return ErrGPUSurfaceGone
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("app: GPUArea image %v is empty", img.Rect)
	}
	b, err := buffer.NewFile(s.app.sess.Shm(), w, h, 1)
	if err != nil {
		return fmt.Errorf("app: GPUArea image buffer %dx%d: %w", w, h, err)
	}
	for y := range h {
		src := img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):][:4*w]
		dst := b.Data[y*b.Stride:][:4*w]
		for x := 0; x < len(src); x += 4 {
			dst[x], dst[x+1], dst[x+2], dst[x+3] = src[x+2], src[x+1], src[x], src[x+3]
		}
	}
	if err := s.commit(b.WL, w, h); err != nil {
		buffer.Cancel(b)
		return err
	}
	return nil
}

func (s *gpuSurface) Import(d dmabuf.Buffer) (widget.GPUBuffer, error) {
	if s.closed {
		return nil, ErrGPUSurfaceGone
	}
	mgr := s.app.sess.Dmabuf()
	if mgr == nil {
		return nil, ErrNoDmabuf
	}
	if !s.app.sess.DmabufImports(wlsession.DmabufLayout{Format: d.Format, Modifier: d.Modifier}) {
		return nil, fmt.Errorf("%w: format %s modifier %s", ErrDmabufLayout, d.Format, d.Modifier)
	}
	params, err := dmabufwl.Params(mgr, d)
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}
	b := &gpuBuffer{s: s, desc: d, params: params}
	params.AddCreatedHandler(b)
	params.AddFailedHandler(b)
	w, err := intconv.Exact[int32](d.Width)
	if err != nil {
		_ = params.Destroy()
		return nil, fmt.Errorf("app: dmabuf width: %w", err)
	}
	h, err := intconv.Exact[int32](d.Height)
	if err != nil {
		_ = params.Destroy()
		return nil, fmt.Errorf("app: dmabuf height: %w", err)
	}
	if err := params.Create(w, h, uint32(d.Format), 0); err != nil {
		_ = params.Destroy()
		return nil, fmt.Errorf("app: dmabuf create: %w", err)
	}
	s.buffers = append(s.buffers, b)
	return b, nil
}

func (s *gpuSurface) Present(gb widget.GPUBuffer) error {
	b, ok := gb.(*gpuBuffer)
	if !ok || b.s != s {
		return errForeignBuffer
	}
	if err := b.Err(); err != nil {
		return err
	}
	if b.wl == nil {
		s.waiting = b
		return nil
	}
	if err := s.commit(b.wl, b.desc.Width, b.desc.Height); err != nil {
		return err
	}
	b.busy = true
	return nil
}

func (s *gpuSurface) Close() {
	if s.closed {
		return
	}
	s.closed = true
	for _, b := range s.buffers {
		b.drop()
	}
	s.buffers, s.waiting = nil, nil
	if s.vp != nil {
		_ = s.vp.Destroy()
	}
	if s.sub != nil {
		_ = s.sub.Destroy()
	}
	_ = s.surf.Destroy()
	s.hw.gpu = slices.DeleteFunc(s.hw.gpu, func(o *gpuSurface) bool { return o == s })
	s.hw.dirty = true
}

func (w *hostWindow) gpuHoles() []render.Rect {
	var holes []render.Rect
	for _, s := range w.gpu {
		if !s.bounds.Empty() {
			holes = append(holes, s.bounds)
		}
	}
	return holes
}

type gpuBuffer struct {
	s      *gpuSurface
	desc   dmabuf.Buffer
	params *wlr.ZwpBufferParamsV1
	wl     *wl.Buffer
	busy   bool
	err    error
}

func (b *gpuBuffer) HandleZwpBufferParamsV1Created(ev wlr.ZwpBufferParamsV1CreatedEvent) {
	b.finishImport()
	b.wl = ev.Buffer
	b.wl.AddReleaseHandler(b)
	if b.err != nil {
		_ = b.wl.Destroy()
		b.wl = nil
		return
	}
	if b.s.waiting == b {
		b.s.waiting = nil
		if err := b.s.Present(b); err != nil {
			b.err = err
		}
	}
}

func (b *gpuBuffer) HandleZwpBufferParamsV1Failed(wlr.ZwpBufferParamsV1FailedEvent) {
	b.finishImport()
	if b.err == nil {
		b.err = fmt.Errorf("%w: %dx%d format %s modifier %s", ErrDmabufRejected, b.desc.Width, b.desc.Height, b.desc.Format, b.desc.Modifier)
	}
	if b.s.waiting == b {
		b.s.waiting = nil
	}
}

func (b *gpuBuffer) finishImport() {
	if b.params != nil {
		_ = b.params.Destroy()
		b.params = nil
	}
}

func (b *gpuBuffer) HandleBufferRelease(wl.BufferReleaseEvent) { b.busy = false }

func (b *gpuBuffer) Ready() bool { return b.wl != nil && b.err == nil }

func (b *gpuBuffer) Busy() bool { return b.busy }

func (b *gpuBuffer) Err() error { return b.err }

func (b *gpuBuffer) Destroy() {
	if b.err == nil {
		b.err = errBufferDestroyed
	}
	b.drop()
	b.s.buffers = slices.DeleteFunc(b.s.buffers, func(o *gpuBuffer) bool { return o == b })
	if b.s.waiting == b {
		b.s.waiting = nil
	}
}

func (b *gpuBuffer) drop() {
	if b.err == nil {
		b.err = ErrGPUSurfaceGone
	}
	if b.wl != nil {
		_ = b.wl.Destroy()
		b.wl = nil
	}
	b.busy = false
}
