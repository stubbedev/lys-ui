// XDG output: optional xdg_output_unstable_v1 support — the stable
// output identities (DP-1, HDMI-A-1) that compositor configs are
// written against, plus the output's logical position and size in the
// global compositor space. Without it outputs are only identifiable
// by their registry order and their Name stays empty. The manager
// global is feature-detected; below manager version 2 the protocol
// simply sends no names.
package wlsession

import (
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxXdgOutputVersion is the xdg_output_unstable_v1 version we bind.
// v2 added the name and description events — the identities wayle
// config matches on; v3 replaced zxdg_output.done with wl_output.done
// as the atomicity barrier.
const maxXdgOutputVersion = 3

// xdgOutputMaker creates the xdg_output for a wl_output, seen through
// the narrow interface below so tests can fake the objects. The wire
// proxy satisfies it through wireXdgOutputMaker.
type xdgOutputMaker interface {
	GetOutput(out *wl.Output) (xdgOutputAPI, error)
}

// xdgOutputAPI is the request and listener side of a zxdg_output_v1.
// *wlr.ZxdgOutputV1 satisfies it; tests substitute a fake.
type xdgOutputAPI interface {
	Destroy() error
	AddLogicalPositionHandler(h wlr.ZxdgOutputV1LogicalPositionHandler)
	AddLogicalSizeHandler(h wlr.ZxdgOutputV1LogicalSizeHandler)
	AddDoneHandler(h wlr.ZxdgOutputV1DoneHandler)
	AddNameHandler(h wlr.ZxdgOutputV1NameHandler)
	AddDescriptionHandler(h wlr.ZxdgOutputV1DescriptionHandler)
}

// wireXdgOutputMaker adapts the generated manager proxy to the narrow
// interface above.
type wireXdgOutputMaker struct{ mgr *wlr.ZxdgOutputManagerV1 }

// GetOutput implements xdgOutputMaker.
func (m wireXdgOutputMaker) GetOutput(out *wl.Output) (xdgOutputAPI, error) {
	o, err := m.mgr.GetOutput(out)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// XdgOutputAvailable reports whether the compositor advertised
// zxdg_output_manager_v1 at a version that carries names (2+).
// Without it the outputs' Name fields stay empty.
func (s *Session) XdgOutputAvailable() bool { return s.xdgOutputMgr != nil }

// bindXdgOutputManager binds the optional xdg-output manager global
// and back-fills the outputs already discovered.
func (s *Session) bindXdgOutputManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZxdgOutputManagerV1(ctx)
	if !s.bindOptional(ev, maxXdgOutputVersion, mgr) {
		return
	}
	s.xdgOutputMgr = wireXdgOutputMaker{mgr: mgr}
	debug.Log("output", "xdg-output-v1 bound")
	s.ensureXdgOutputs()
}

// ensureXdgOutputs creates an xdg_output for every tracked output
// that lacks one. The manager and the outputs arrive in arbitrary
// registry order, so both the wl_output and the manager bind paths
// call this; it is safe to repeat.
func (s *Session) ensureXdgOutputs() {
	if s.xdgOutputMgr == nil {
		return
	}
	for _, out := range s.outputs {
		if out.xdg != nil || out.WL == nil {
			continue
		}
		req, err := s.xdgOutputMgr.GetOutput(out.WL)
		if err != nil {
			continue
		}
		out.xdg = req
		ev := &xdgOutputEvents{sess: s, out: out}
		req.AddLogicalPositionHandler(ev)
		req.AddLogicalSizeHandler(ev)
		req.AddDoneHandler(ev)
		req.AddNameHandler(ev)
		req.AddDescriptionHandler(ev)
	}
}

// xdgOutputEvents tracks one output's xdg_output state; the binding
// dispatches events without naming their object, so each output gets
// its own listener (mirroring outputEvents for wl_output itself).
type xdgOutputEvents struct {
	sess *Session
	out  *Output
}

// HandleZxdgOutputV1LogicalPosition implements
// wlr.ZxdgOutputV1LogicalPositionHandler.
func (e *xdgOutputEvents) HandleZxdgOutputV1LogicalPosition(ev wlr.ZxdgOutputV1LogicalPositionEvent) {
	e.out.LogicalX, e.out.LogicalY = ev.X, ev.Y
}

// HandleZxdgOutputV1LogicalSize implements
// wlr.ZxdgOutputV1LogicalSizeHandler.
func (e *xdgOutputEvents) HandleZxdgOutputV1LogicalSize(ev wlr.ZxdgOutputV1LogicalSizeEvent) {
	e.out.LogicalW, e.out.LogicalH = ev.Width, ev.Height
}

// HandleZxdgOutputV1Done implements wlr.ZxdgOutputV1DoneHandler:
// deprecated since version 3, where wl_output.done took over as the
// atomicity barrier; the properties are applied as they arrive.
func (e *xdgOutputEvents) HandleZxdgOutputV1Done(wlr.ZxdgOutputV1DoneEvent) {}

// HandleZxdgOutputV1Name implements wlr.ZxdgOutputV1NameHandler: the
// stable identity (DP-1, HDMI-A-1) landed; notify the host once it
// changes.
func (e *xdgOutputEvents) HandleZxdgOutputV1Name(ev wlr.ZxdgOutputV1NameEvent) {
	if e.out.Name == ev.Name {
		return
	}
	e.out.Name = ev.Name
	e.sess.notifyOutputIdentity(e.out)
}

// HandleZxdgOutputV1Description implements
// wlr.ZxdgOutputV1DescriptionHandler.
func (e *xdgOutputEvents) HandleZxdgOutputV1Description(ev wlr.ZxdgOutputV1DescriptionEvent) {
	e.out.Description = ev.Description
}
