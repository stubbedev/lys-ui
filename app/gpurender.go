package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"os"
	"slices"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/internal/popup"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
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
	modifiers []uint64
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
	// node is the compositor's main device the renderer was opened on;
	// hasNode is false where the compositor named none.
	node    uint64
	hasNode bool
	// render is the device's Render, a seam for tests.
	render func(*render.List, *gpu.Target) error
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
	if r.hasNode {
		level = slog.LevelError
	}
	logutil.L().Log(context.Background(), level, "app: the compositor refused the GPU's buffers; every window draws on the CPU from now on",
		"device", r.name, "named_by_compositor", r.hasNode, "err", err)
}

// renderer is the application's GPU renderer, opened on first use;
// nil draws on the CPU. With LYS_UI_RENDERER=gpu a GPU that cannot
// draw for this compositor is an error.
func (a *Application) renderer() (*gpuRenderer, error) {
	if a.gpuTried {
		return a.gpu, a.gpuErr
	}
	a.gpuTried = true
	mode := os.Getenv(rendererEnv)
	switch mode {
	case "cpu":
		return nil, nil
	case "", "auto", "gpu":
	default:
		a.gpuErr = fmt.Errorf("app: %s=%q: want cpu, gpu or auto", rendererEnv, mode)
		return nil, a.gpuErr
	}
	r, err := a.openRenderer()
	if err == nil {
		r.required = mode == "gpu"
	}
	if err != nil {
		if mode == "gpu" {
			a.gpuErr = fmt.Errorf("app: %s=gpu: %w", rendererEnv, err)
			return nil, a.gpuErr
		}
		logutil.L().Info("app: no GPU renderer; every window draws on the CPU", "err", err)
		return nil, nil
	}
	debug.Log("gpu", "drawing on %s in %d modifier(s)", r.dev.Name(), len(r.modifiers))
	a.gpu = r
	return r, nil
}

// openRenderer opens the GPU device and agrees on the dmabuf layouts it
// renders and the compositor imports. The device is the compositor's
// main device, the one whose memory it imports; a compositor that names
// none (linux-dmabuf before v4) gets the first device that exports.
func (a *Application) openRenderer() (*gpuRenderer, error) {
	if a.sess == nil || a.sess.Dmabuf() == nil {
		return nil, errors.New("the compositor has no linux-dmabuf")
	}
	node, hasNode := a.sess.DmabufDevice()
	dev, err := openDevice(node, hasNode)
	if err != nil {
		return nil, err
	}
	mods, err := a.agreeModifiers(dev)
	if err != nil {
		dev.Close()
		return nil, err
	}
	return &gpuRenderer{
		dev: dev, name: dev.Name(), modifiers: mods, rec: render.NewRecorder(1, 1, 1, 1), render: dev.Render,
		node: node, hasNode: hasNode,
	}, nil
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

// agreeModifiers is the ARGB8888 layouts dev renders and the session's
// compositor imports.
func (a *Application) agreeModifiers(dev *gpu.Device) ([]uint64, error) {
	if a.sess == nil || a.sess.Dmabuf() == nil {
		return nil, errors.New("the compositor has no linux-dmabuf")
	}
	mods, err := dev.Modifiers(dmabuf.FormatARGB8888)
	if err != nil {
		return nil, err
	}
	imports := a.sess.DmabufModifiers(dmabuf.FormatARGB8888)
	mods = slices.DeleteFunc(mods, func(m uint64) bool { return !slices.Contains(imports, m) })
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
	mods, err := a.agreeModifiers(r.dev)
	if node, hasNode := a.sess.DmabufDevice(); err == nil && (hasNode != r.hasNode || node != r.node) {
		err = fmt.Errorf("the new compositor imports from device %d:%d, not the renderer's", unix.Major(node), unix.Minor(node))
	}
	if err == nil {
		r.modifiers = mods
		return nil
	}
	if r.required {
		return fmt.Errorf("app: %s=gpu after reconnecting: %w", rendererEnv, err)
	}
	logutil.L().Warn("app: the new compositor takes no GPU frames; drawing on the CPU", "device", r.name, "err", err)
	a.closeRenderer()
	return nil
}

// NewBuffer is a dmabuf buffer of r for a bufW x bufH surface.
func (r *gpuRenderer) NewBuffer(sess *wlsession.Session, bufW, bufH, scale int) (*buffer.Buffer, error) {
	if r.refused != nil {
		return nil, fmt.Errorf("app: %s=gpu: %w", rendererEnv, r.refused)
	}
	return buffer.NewDmabuf(r.dev, sess.Dmabuf(), bufW, bufH, scale, r.modifiers, r.refuse)
}

// Frame renders paint into b's target at device scale num over
// scale.Denom. A failed frame turns every surface to the CPU (fail)
// and reports false; with the GPU required it is the error.
func (r *gpuRenderer) Frame(b *buffer.Buffer, num int, paint func(cv *render.Canvas)) (bool, error) {
	cv := r.rec.ResetRecorder(b.Width, b.Height, num, scale.Denom)
	paint(cv)
	if err := r.render(cv.Recording(), b.Target); err != nil {
		if r.required {
			return false, fmt.Errorf("app: GPU frame: %w", err)
		}
		r.fail(err)
		return false, nil
	}
	return true, nil
}

// Copy copies r, in buffer pixels, from src's target into dst's. A
// failed copy turns every surface to the CPU and reports false, or,
// with the GPU required, is the error.
func (r *gpuRenderer) Copy(dst, src *buffer.Buffer, rect render.Rect) (bool, error) {
	err := r.dev.CopyRects(dst.Target, src.Target, []image.Rectangle{image.Rect(rect.X, rect.Y, rect.X+rect.W, rect.Y+rect.H)})
	if err == nil {
		return true, nil
	}
	if r.required {
		return false, fmt.Errorf("app: GPU copy: %w", err)
	}
	r.fail(err)
	return false, nil
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

// closeRenderer releases the GPU once every window has gone.
func (a *Application) closeRenderer() {
	if a.gpu != nil {
		a.gpu.dev.Close()
		a.gpu = nil
	}
}
