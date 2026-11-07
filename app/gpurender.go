package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/wlr"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/imgcache"
	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/internal/popup"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// rendererEnv picks how windows draw: "cpu" on the CPU into shared
// memory, "gpu" on the GPU into dmabufs (failing where it cannot),
// unset or "auto" on the GPU where the device and the compositor allow
// it, else on the CPU.
const rendererEnv = "LYS_UI_RENDERER"

// gpuRenderer draws an application's windows with Vulkan into dmabufs
// the compositor imports: one device for every window, so layers and
// glyph uploads are shared.
type gpuRenderer struct {
	dev       *gpu.Device
	name      string
	modifiers []fourcc.Modifier
	// rec is the recording canvas a frame paints into before the GPU
	// renders it; windows paint one at a time, on the loop.
	rec *render.Canvas
	// required is LYS_UI_RENDERER=gpu: a failed frame is fatal rather
	// than a switch to the CPU.
	required bool
	// failed is the first frame error; once set, every window draws on
	// the CPU from its next frame.
	failed error
	// refused is an import the compositor refused under
	// LYS_UI_RENDERER=gpu, where it fails the next buffer instead of
	// sending the windows to the CPU.
	refused error
	plan    rendererPlan
	// render draws a frame on the device, a seam for tests: explicit for a
	// consumer told when it is done through its sync_file, which never waits.
	render func(l *render.List, t *gpu.Target, carry []gpu.Carry, explicit bool) (gpuFrame, error)
	// timelines are the device's timeline syncobjs where the compositor takes
	// explicit synchronization, nil where it does not.
	timelines syncTimelines
	// wake arms the loop to wake after a delay: the retry of a frame the
	// device refused, which no commit will call back for.
	wake func(time.Duration)
	// invoke runs a worker's answer on the loop.
	invoke func(func())
	lost   func(*gpuRenderer, error)
	// prepare is the device's PrepareTarget, a seam for tests.
	prepare func(w, h int, mods []fourcc.Modifier) (*gpu.Target, error)
	// workers are the target allocations running; closed marks the device
	// closed, after which a target landing is dropped.
	workers sync.WaitGroup
	closed  bool
	// drawn marks a frame since the buffer trim last looked, and
	// trimmed a device trimmed since its last frame (trimIdle).
	drawn, trimmed bool
}

type gpuFrame struct {
	incomplete bool
	syncFile   int
}

func (r *gpuRenderer) deviceFrame(l *render.List, t *gpu.Target, carry []gpu.Carry, explicit bool) (gpuFrame, error) {
	if explicit {
		fd, err := r.dev.RenderExplicit(l, t, carry...)
		return gpuFrame{incomplete: r.dev.FrameIncomplete(), syncFile: fd}, err
	}
	err := r.dev.Render(l, t, carry...)
	return gpuFrame{incomplete: r.dev.FrameIncomplete(), syncFile: -1}, err
}

func (r *gpuRenderer) deviceTarget(w, h int, mods []fourcc.Modifier) (*gpu.Target, error) {
	return r.dev.PrepareTarget(w, h, fourcc.ARGB8888, mods)
}

// trimIdle trims the device once it has drawn nothing for a whole trim
// interval: its scratch canvases and oversized staging go back, which a
// device still drawing would only allocate again. It reports whether
// the device drew since the last look, so the trim timer keeps running
// until it has gone idle.
func (r *gpuRenderer) trimIdle() bool {
	if r.drawn {
		r.drawn = false
		return true
	}
	if !r.trimmed {
		r.trimmed = true
		if err := r.dev.Trim(); err != nil {
			r.fault(fmt.Errorf("app: trim the GPU device: %w", err))
		}
	}
	return false
}

func (r *gpuRenderer) fault(err error) {
	if errors.Is(err, gpu.ErrDeviceLost) && !r.required {
		r.lose(err)
		return
	}
	r.fail(err)
}

func (r *gpuRenderer) lose(err error) {
	if r.failed != nil {
		return
	}
	r.failed = err
	r.invoke(func() { r.lost(r, err) })
}

// fail records a frame error and reports the switch to the CPU, once.
func (r *gpuRenderer) fail(err error) {
	if r.failed != nil {
		return
	}
	r.failed = err
	logutil.L().Error("app: a GPU frame failed; every window draws on the CPU from now on", "device", r.name, "err", err)
}

// refuse takes a dmabuf import the compositor refused: every window
// goes to the CPU, or, under LYS_UI_RENDERER=gpu, the next buffer
// fails with it. Buffers of a device the compositor named are an error;
// a compositor that named none refusing the device guessed at is the
// expected end of that guess, a warning.
func (r *gpuRenderer) refuse(err error) {
	if r.required {
		r.refused = err
		return
	}
	if r.failed != nil {
		return
	}
	r.failed = err
	level := slog.LevelWarn
	if r.plan.hasNode {
		level = slog.LevelError
	}
	logutil.L().Log(context.Background(), level, "app: the compositor refused the GPU's buffers; every window draws on the CPU from now on",
		"device", r.name, "named_by_compositor", r.plan.hasNode, "err", err)
}

// renderer is the application's GPU renderer; nil draws on the CPU. Under
// LYS_UI_RENDERER=gpu the device opens here, and a GPU that cannot draw for
// this compositor is an error. Otherwise it opens on a worker, the windows
// drawing on the CPU until it is there (adoptRenderer).
func (a *Application) renderer() (*gpuRenderer, error) {
	if a.gpuTried {
		return a.gpu, a.gpuErr
	}
	a.gpuTried = true
	mode := os.Getenv(rendererEnv)
	switch mode {
	case "cpu":
		return nil, nil
	case "", "auto":
		plan, err := a.rendererPlan()
		if err != nil {
			logutil.L().Info("app: no GPU renderer; every window draws on the CPU", "err", err)
			return nil, nil
		}
		a.startRenderer(plan, openRenderer)
		return nil, nil
	case "gpu":
	default:
		a.gpuErr = fmt.Errorf("app: %s=%q: want cpu, gpu or auto", rendererEnv, mode)
		return nil, a.gpuErr
	}
	plan, err := a.rendererPlan()
	var r *gpuRenderer
	if err == nil {
		r, err = openRenderer(plan)
	}
	if err != nil {
		a.gpuErr = fmt.Errorf("app: %s=gpu: %w", rendererEnv, err)
		return nil, a.gpuErr
	}
	r.required = true
	a.installRenderer(r)
	return r, nil
}

type rendererPlan struct {
	node    uint64
	hasNode bool
	imports []fourcc.Modifier
	syncobj *wlr.WpDrmSyncobjManagerV1
}

func (a *Application) rendererPlan() (rendererPlan, error) {
	if a.sess == nil || a.sess.Dmabuf() == nil {
		return rendererPlan{}, errors.New("the compositor has no linux-dmabuf")
	}
	node, hasNode := a.sess.DmabufDevice()
	return rendererPlan{node: node, hasNode: hasNode, imports: a.sess.DmabufModifiers(fourcc.ARGB8888), syncobj: a.sess.SyncobjManager()}, nil
}

func (a *Application) startRenderer(plan rendererPlan, open func(rendererPlan) (*gpuRenderer, error)) {
	a.gpuOpen = open
	sess := a.sess
	a.gpuOpening.Go(func() {
		r, err := open(plan)
		if err != nil {
			a.Invoke(func() {
				logutil.L().Info("app: no GPU renderer; every window draws on the CPU", "err", err)
			})
			return
		}
		a.gpuLanding.Store(r)
		a.Invoke(func() { a.adoptRenderer(sess) })
	})
}

func (a *Application) adoptRenderer(sess *wlsession.Session) {
	r := a.gpuLanding.Swap(nil)
	if r == nil {
		return
	}
	if sess != a.sess {
		r.dev.Close()
		if plan, err := a.rendererPlan(); err == nil {
			a.startRenderer(plan, a.gpuOpen)
		}
		return
	}
	a.installRenderer(r)
	for _, w := range a.windows {
		w.useRenderer(r)
	}
}

func (a *Application) installRenderer(r *gpuRenderer) {
	r.wake, r.invoke, r.lost = func(d time.Duration) { a.wake(d) }, a.Invoke, a.loseRenderer
	if t, ok := r.timelines.(*drmTimelines); ok {
		t.setInvoke(a.Invoke)
	}
	debug.Log("gpu", "drawing on %s in %d modifier(s)", r.dev.Name(), len(r.modifiers))
	a.gpu = r
	// Evicted image rasters drop their textures with their pixels.
	imgcache.Default().SetForget(r.dev.Forget)
}

// openRenderer opens the GPU device and agrees on the dmabuf layouts it
// renders and the compositor imports. The device is the compositor's
// main device, the one whose memory it imports; a compositor that names
// none (linux-dmabuf before v4) gets the first device that exports.
func openRenderer(plan rendererPlan) (*gpuRenderer, error) {
	dev, err := openDevice(plan.node, plan.hasNode)
	if err != nil {
		return nil, err
	}
	mods, err := agreeModifiers(dev, plan.imports)
	if err != nil {
		dev.Close()
		return nil, err
	}
	r := &gpuRenderer{dev: dev, name: dev.Name(), modifiers: mods, rec: render.NewRecorder(1, 1, 1, 1), plan: plan}
	r.render, r.prepare = r.deviceFrame, r.deviceTarget
	if plan.syncobj != nil {
		r.timelines = explicitTimelines(dev)
	}
	return r, nil
}

// openDevice opens the Vulkan device behind the DRM node dev when the
// compositor named it, else the first that exports.
func openDevice(dev uint64, named bool) (*gpu.Device, error) {
	if !named {
		return gpu.Open()
	}
	d, err := gpu.OpenNode(dev)
	if err != nil {
		return nil, fmt.Errorf("the compositor's main device %d:%d: %w", unix.Major(dev), unix.Minor(dev), err)
	}
	return d, nil
}

// agreeModifiers is the ARGB8888 layouts dev renders and a compositor
// imports.
func agreeModifiers(dev *gpu.Device, imports []fourcc.Modifier) ([]fourcc.Modifier, error) {
	mods, err := dev.Modifiers(fourcc.ARGB8888)
	if err != nil {
		return nil, err
	}
	mods = slices.DeleteFunc(mods, func(m fourcc.Modifier) bool { return !slices.Contains(imports, m) })
	if len(mods) == 0 {
		return nil, fmt.Errorf("%s renders ARGB8888 in no modifier the compositor imports", dev.Name())
	}
	return mods, nil
}

// reagreeRenderer agrees the renderer's layouts again with a new
// session's compositor, after a reconnect: a compositor that imports
// none the device renders sends the app to the CPU, logged, or, under
// LYS_UI_RENDERER=gpu, fails the reconnect.
func (a *Application) reagreeRenderer() error {
	r := a.gpu
	if r == nil {
		return nil
	}
	plan, err := a.rendererPlan()
	var mods []fourcc.Modifier
	if err == nil {
		mods, err = agreeModifiers(r.dev, plan.imports)
	}
	if err == nil && (plan.hasNode != r.plan.hasNode || plan.node != r.plan.node) {
		err = fmt.Errorf("the new compositor imports from device %d:%d, not the renderer's", unix.Major(plan.node), unix.Minor(plan.node))
	}
	if err == nil {
		r.modifiers, r.plan = mods, plan
		return nil
	}
	if r.required {
		return fmt.Errorf("app: %s=gpu after reconnecting: %w", rendererEnv, err)
	}
	logutil.L().Warn("app: the new compositor takes no GPU frames; drawing on the CPU", "device", r.name, "err", err)
	a.closeRenderer()
	return nil
}

// NewBuffer is a dmabuf buffer of r for a bufW x bufH surface, with a timeline
// of its own where the compositor takes explicit synchronization.
func (r *gpuRenderer) NewBuffer(sess *wlsession.Session, bufW, bufH, scale int) (*buffer.Buffer, error) {
	return r.newBufferIn(sess, bufW, bufH, scale, nil, r.refuse)
}

func (r *gpuRenderer) newBufferIn(sess *wlsession.Session, bufW, bufH, scale int, mods []fourcc.Modifier, rejected func(error)) (*buffer.Buffer, error) {
	if r.refused != nil {
		return nil, fmt.Errorf("app: %s=gpu: %w", rendererEnv, r.refused)
	}
	b, err := buffer.NewDmabuf(r.allocateIn(mods), sess.Dmabuf(), bufW, bufH, scale, rejected)
	if err != nil || r.timelines == nil || r.plan.syncobj == nil {
		return b, err
	}
	if b.Timeline, err = r.newTimeline(); err != nil {
		return nil, err
	}
	b.Timeline.Released = b.Release
	return b, nil
}

func (r *gpuRenderer) newTimeline() (*buffer.Timeline, error) {
	s, fd, err := r.timelines.create()
	if err != nil {
		return nil, fmt.Errorf("app: a buffer's timeline: %w", err)
	}
	wlt, err := r.plan.syncobj.ImportTimeline(uintptr(fd))
	if cerr := unix.Close(fd); err == nil && cerr != nil {
		err = cerr
	}
	if err != nil {
		r.timelines.destroy(s)
		return nil, fmt.Errorf("app: import a buffer's timeline: %w", err)
	}
	timelines := r.timelines
	return buffer.NewTimeline(wlt, s, func() {
		_ = wlt.Destroy()
		timelines.destroy(s)
	}), nil
}

func (r *gpuRenderer) syncFrame(b *buffer.Buffer, syncFile int) error {
	t := b.Timeline
	acquire, release := t.Next()
	err := r.timelines.acquire(t.Syncobj, acquire, syncFile)
	if syncFile >= 0 {
		if cerr := unix.Close(syncFile); err == nil && cerr != nil {
			err = cerr
		}
	}
	if err != nil {
		return fmt.Errorf("app: a frame's acquire point %d: %w", acquire, err)
	}
	if err := r.timelines.watch(t.Syncobj, release, t.Released); err != nil {
		return fmt.Errorf("app: a frame's release point %d: %w", release, err)
	}
	return nil
}

func (r *gpuRenderer) allocateIn(mods []fourcc.Modifier) buffer.Allocate {
	return func(w, h int, done func(*gpu.Target, error)) {
		in := mods
		if in == nil {
			in = r.modifiers
		}
		r.workers.Go(func() {
			t, err := r.prepare(w, h, in)
			r.invoke(func() {
				if r.closed {
					done(nil, gpu.ErrDeviceClosed)
					return
				}
				done(t, err)
			})
		})
	}
}

const gpuBusyRetry = 4 * time.Millisecond

// Frame renders paint into b's target at device scale num over
// scale.Denom, after the copies carry makes into it (repaint.Region.Blit),
// in the same submission. A device too far behind refuses the frame, having
// done nothing, and the loop wakes to retry it: no commit went out, so
// no frame callback will. A frame drawn without an image still on its way
// to the device is incomplete: it goes out, and what it painted stays owed.
// A failed frame turns every surface to the CPU (fault), until a new
// device opens where the old one was lost; with the GPU required it is
// the error.
func (r *gpuRenderer) Frame(b *buffer.Buffer, num int, carry []gpu.Carry, paint func(cv *render.Canvas)) (buffer.FrameOutcome, error) {
	r.drawn, r.trimmed = true, false
	cv := r.rec.ResetRecorder(b.Width, b.Height, num, scale.Denom)
	paint(cv)
	drawn, err := r.render(cv.Recording(), b.Target, carry, b.Timeline != nil)
	if err == nil && b.Timeline != nil {
		err = r.syncFrame(b, drawn.syncFile)
	}
	if drawn.incomplete || (err != nil && !errors.Is(err, gpu.ErrBusy)) {
		widget.RepaintKeptLayers()
	}
	// The recording is done once drawn: ending it now releases the
	// rasters retired while it was open (render.Retire), which an idle
	// window would otherwise hold back until its next frame.
	cv.Recording().End()
	switch {
	case err == nil && drawn.incomplete:
		return buffer.FrameIncomplete, nil
	case err == nil:
		return buffer.FrameDrawn, nil
	case errors.Is(err, gpu.ErrBusy):
		r.wake(gpuBusyRetry)
		return buffer.FrameRefused, nil
	case r.required:
		return buffer.FrameLost, fmt.Errorf("app: GPU frame: %w", err)
	}
	r.fault(err)
	return buffer.FrameLost, nil
}

// Failed reports whether a frame failed, turning every surface to the
// CPU.
func (r *gpuRenderer) Failed() bool { return r.failed != nil }

// popupRenderer is what popups draw with: the GPU renderer while it
// draws, nil (the CPU) otherwise.
func (a *Application) popupRenderer() popup.Renderer {
	if a.gpu == nil || a.gpu.Failed() {
		return nil
	}
	return a.gpu
}

// ForgetImage drops the GPU's copy of the raster id names once the
// frame drawing it is done, instead of after it idles out: an owner
// that frees a raster's pixels (render.Retire), or stops showing a large
// one for good, calls it so the texture goes with them. Drawing on the
// CPU there is no copy to drop. Call it on the event-loop goroutine. A
// zero id, no identity, panics: such a raster is the garbage
// collector's to free.
func (a *Application) ForgetImage(id render.ImageID) {
	if id == 0 {
		panic("app: ForgetImage of a raster without an ImageID")
	}
	if a.gpu != nil {
		a.gpu.dev.Forget(id)
	}
}

func (a *Application) loseRenderer(r *gpuRenderer, err error) {
	if a.gpu != r {
		return
	}
	for _, w := range a.windows {
		if w.renderer == r {
			w.renderer = nil
			w.reallocate()
		}
	}
	a.releaseRenderer()
	if !a.gpuLosses.Record(time.Now(), err) {
		logutil.L().Error("app: the GPU device was lost too often; every window draws on the CPU from now on",
			"device", r.name, "losses", a.gpuLosses.Count(), "window", gpu.LossWindow, "err", a.gpuLosses.Err())
		return
	}
	logutil.L().Warn("app: the GPU device was lost; drawing on the CPU until a new one opens",
		"device", r.name, "losses", a.gpuLosses.Count(), "err", err)
	a.startRenderer(r.plan, a.gpuOpen)
}

// closeRenderer releases the GPU once every window has gone.
func (a *Application) closeRenderer() {
	a.gpuOpening.Wait()
	if r := a.gpuLanding.Swap(nil); r != nil {
		r.dev.Close()
	}
	a.releaseRenderer()
}

func (a *Application) releaseRenderer() {
	r := a.gpu
	if r == nil {
		return
	}
	imgcache.Default().SetForget(nil)
	r.closed = true
	r.dev.Close()
	r.workers.Wait()
	if r.timelines != nil {
		r.timelines.close()
	}
	a.gpu = nil
}
