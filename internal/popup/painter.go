// The per-frame paint pipeline for a popup surface, factored out of
// Run so two kinds of host loops can drive it without a second
// wayland dispatcher: Run (a nested loop on the caller's goroutine,
// for grabbed menus) and the application loop (for tooltips, which
// must never dispatch — see docs/threading.md: one event-loop
// goroutine owns the connection).
package popup

import (
	"errors"
	"sync/atomic"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// Renderer draws a popup's frames on the GPU, as its window's are: the
// buffers it renders into and the frame a paint becomes there. A frame
// that fails reports false, after which Failed holds and the popup
// draws on the CPU; an error ends the popup.
type Renderer interface {
	NewBuffer(sess *wlsession.Session, bufW, bufH, scale int) (*buffer.Buffer, error)
	Frame(b *buffer.Buffer, num int, paint func(cv *render.Canvas)) (bool, error)
	Failed() bool
}

// Painter owns one popup surface's buffer pool and frame pacing.
// Pass drives at most one frame per call; the owning loop calls it
// once per pass and parks however it likes between passes.
type Painter struct {
	p *Popup

	sess    *wlsession.Session
	gpu     Renderer // nil draws on the CPU
	pool    *buffer.Pool
	surf    *wl.Surface
	frac120 uint32
	bg      render.Color
	noPlate bool          // Config.NoPlate: the content paints its own look
	gutter  int           // the shadow margin the content is inset by
	visual  widget.Widget // the morph wrapper around the content, plate included
	// w, h is the size the pool's buffers have, so a resize retires
	// them.
	w, h int

	frameReady   bool
	framePending bool
	mapped       bool
	dirty        atomic.Bool
}

// NewPainter wraps root in the tween view (seeded from the
// coordinator, so the first frame is the hidden or offset one, never a
// flash of the finished surface) and returns the frame driver. A
// non-nil gpu draws the frames on the GPU.
func (p *Popup) NewPainter(sess *wlsession.Session, gpu Renderer, frac120 uint32, root widget.Widget, bg render.Color) *Painter {
	if frac120 == 0 {
		frac120 = scale.Denom
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	pc := &Painter{
		p:       p,
		sess:    sess,
		gpu:     gpu,
		surf:    p.WLSurface,
		frac120: frac120,
		bg:      bg,
		noPlate: p.cfg.NoPlate,
		gutter:  p.gutter,
	}
	pc.pool = buffer.New(pc.create, 2)
	reveal, morph := p.fx.Visual()
	p.anim = newAnimView(root, reveal, morph, p.gravity, p.gutter, pc.paintPlate)
	pc.visual = p.anim.morph
	pc.w, pc.h = p.Size()
	pc.dirty.Store(true)
	p.painter = pc
	return pc
}

// create allocates a buffer at the popup's current size: a GPU target
// while the popup draws on the GPU, shared memory otherwise.
func (pc *Painter) create() (*buffer.Buffer, error) {
	w, h := pc.p.Size()
	dw, dh, s := scale.DeviceSize(w, pc.frac120), scale.DeviceSize(h, pc.frac120), scale.IntegerScale(pc.frac120)
	if pc.gpu != nil {
		return pc.gpu.NewBuffer(pc.sess, dw, dh, s)
	}
	return buffer.NewFile(pc.sess.Shm(), dw, dh, s)
}

// dropGPU turns the popup to the CPU after a GPU frame failed: the pool
// retires its targets for shared-memory buffers.
func (pc *Painter) dropGPU() {
	pc.gpu = nil
	pc.pool.Resize(pc.create)
}

// frameDone flips ready when the compositor reports the frame as taken
// and unregisters the callback: done is a destructor event, so the
// object is dead on both sides and its id must rejoin the client's
// pool. A frame loop that skips this leaks a proxy per frame.
type frameDone struct {
	ready *bool
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (f frameDone) HandleCallbackDone(ev wl.CallbackDoneEvent) {
	ev.C.Unregister()
	*f.ready = true
}

// Owes reports whether the painter has a frame to paint: a tween
// callback or content change marked it. Owners skip the Pass call
// while it reads false, the popover-style gate.
func (pc *Painter) Owes() bool { return pc.dirty.Load() }

// Pass paints at most one frame: when a repaint is owed and the
// compositor has taken the previous one, measure, arrange, paint under
// the current reveal, commit, and arm the next frame callback. The
// whole wire sequence holds p.mu, serialized against the dismissal
// state machine's SealInput and Destroy. It returns false when the
// popup is destroyed and the painter is spent.
func (pc *Painter) Pass() (bool, error) {
	if pc.p.Destroyed() {
		return false, nil
	}
	if pc.p.dirty.CompareAndSwap(true, false) {
		pc.dirty.Store(true)
	}
	if pc.frameReady {
		pc.frameReady = false
		pc.framePending = false
	}
	if !pc.dirty.Load() || pc.framePending {
		return true, nil
	}
	pc.dirty.Store(false)
	if pc.gpu != nil && pc.gpu.Failed() {
		pc.dropGPU()
	}
	if w, h := pc.p.Size(); w != pc.w || h != pc.h {
		// The popup was re-placed at a new size: buffers of the old one
		// retire as they come back.
		pc.pool.Resize(pc.create)
		pc.w, pc.h = w, h
	}
	b, err := pc.pool.Acquire()
	if errors.Is(err, buffer.ErrBusy) {
		// The release event (dispatched by the owning loop) wakes it.
		pc.dirty.Store(true)
		return true, nil
	}
	if err != nil {
		return false, err
	}
	p := pc.p
	p.mu.Lock()
	if p.Destroyed() {
		p.mu.Unlock()
		buffer.Cancel(b)
		return false, nil
	}
	w, h := p.Size()
	_ = p.sc.Apply(pc.frac120, w, h)
	pc.visual.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
	pc.visual.Arrange(render.Rect{W: w, H: h})

	drawn, err := pc.paint(b)
	if err != nil {
		p.mu.Unlock()
		return false, err
	}
	if !drawn {
		p.mu.Unlock()
		pc.dirty.Store(true)
		return true, nil
	}

	if err := pc.surf.Attach(b.WL, 0, 0); err != nil {
		p.mu.Unlock()
		return false, err
	}
	if err := pc.surf.DamageBuffer(0, 0, intconv.Int32(b.Width), intconv.Int32(b.Height)); err != nil {
		p.mu.Unlock()
		return false, err
	}
	// The frame request is double-buffered state: it must precede the
	// commit it rides on. Requested after, it would wait for a commit
	// that never comes - this pass only commits again once the callback
	// it is waiting for has fired.
	cb, err := pc.surf.Frame()
	if err != nil {
		p.mu.Unlock()
		return false, err
	}
	wlclient.CallbackAddListener(cb, frameDone{ready: &pc.frameReady})
	if err := pc.surf.Commit(); err != nil {
		p.mu.Unlock()
		return false, err
	}
	p.mu.Unlock()
	pc.framePending = true
	if !pc.mapped {
		pc.mapped = true
		p.checkSide(intconv.Int32(h))
	}
	debug.Log("frame", "popup %d frame %dx%d", pc.surf.Id(), b.Width, b.Height)
	return true, nil
}

// paint draws the visual into b: through the GPU renderer into a
// target, onto shared memory otherwise. A GPU frame that failed never
// reaches the compositor: b goes back, the popup turns to the CPU, and
// paint reports false for the next pass to repaint.
func (pc *Painter) paint(b *buffer.Buffer) (bool, error) {
	// The surface raw-clears to transparent: with a shadow gutter the
	// falloff must blend over whatever the popup floats above, and a
	// recycled buffer must not leak its last frame through. The morph
	// paints shadow, plate and content together, so the motion carries
	// the elevation with the popup.
	draw := func(cv *render.Canvas) {
		cv.ClearDevice(cv.Rect(), render.Color(0))
		pc.visual.Paint(cv)
	}
	if b.Target == nil {
		draw(render.NewScaled(b.Data, b.Stride, b.Width, b.Height, int(pc.frac120), scale.Denom))
		return true, nil
	}
	ok, err := pc.gpu.Frame(b, int(pc.frac120), draw)
	if err != nil || ok {
		return ok, err
	}
	b.Release()
	pc.dropGPU()
	return false, nil
}

// Close releases the painter's pool storage back to the session arena.
func (pc *Painter) Close() { pc.pool.Close() }

// paintPlate fills the content rect with the popup background and,
// when the theme reserves a shadow gutter, paints the elevation around
// it first - unless the content paints its own (Config.NoPlate) — the shared elevation look every popup carries, content
// kind aside. The plate rounds at the theme radius, matching the
// shadow silhouette and the menu plates painted on top of it.
func (pc *Painter) paintPlate(cv *render.Canvas, content render.Rect) {
	if pc.noPlate {
		return
	}
	t := widget.Current()
	if pc.gutter > 0 {
		t.DrawShadow(cv, content, t.Radius)
		cv.RoundedRect(content, t.Radius, pc.bg)
		return
	}
	cv.FillRect(content, pc.bg)
}
