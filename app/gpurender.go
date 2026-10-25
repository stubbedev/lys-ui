package app

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/stubbedev/lys-ui/dmabuf"
	"github.com/stubbedev/lys-ui/gpu"
	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
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
		debug.Log("gpu", "drawing on the CPU: %v", err)
		return nil, nil
	}
	debug.Log("gpu", "drawing on %s in %d modifier(s)", r.dev.Name(), len(r.modifiers))
	a.gpu = r
	return r, nil
}

// openRenderer opens the GPU device and agrees on the dmabuf layouts it
// renders and the compositor imports.
func (a *Application) openRenderer() (*gpuRenderer, error) {
	if a.sess == nil || a.sess.Dmabuf() == nil {
		return nil, errors.New("the compositor has no linux-dmabuf")
	}
	dev, err := gpu.Open()
	if err != nil {
		return nil, err
	}
	mods, err := dev.Modifiers(dmabuf.FormatARGB8888)
	if err != nil {
		dev.Close()
		return nil, err
	}
	imports := a.sess.DmabufModifiers(dmabuf.FormatARGB8888)
	mods = slices.DeleteFunc(mods, func(m uint64) bool { return !slices.Contains(imports, m) })
	if len(mods) == 0 {
		dev.Close()
		return nil, fmt.Errorf("%s renders ARGB8888 in no modifier the compositor imports", dev.Name())
	}
	return &gpuRenderer{dev: dev, name: dev.Name(), modifiers: mods, rec: render.NewRecorder(1, 1, 1, 1), render: dev.Render}, nil
}

// newBuffer is a dmabuf buffer of r for a bufW x bufH window.
func (r *gpuRenderer) newBuffer(sess *wlsession.Session, bufW, bufH, scale int) (*buffer.Buffer, error) {
	return buffer.NewDmabuf(r.dev, sess.Dmabuf(), bufW, bufH, scale, r.modifiers)
}

// closeRenderer releases the GPU once every window has gone.
func (a *Application) closeRenderer() {
	if a.gpu != nil {
		a.gpu.dev.Close()
		a.gpu = nil
	}
}
