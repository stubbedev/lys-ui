// Foreign toplevels: optional
// zwlr_foreign_toplevel_management_unstable_v1 support — the window
// list a taskbar or alt-tab drives. The manager global is
// feature-detected: compositors without it keep the session running
// with no toplevels and every request a no-op. The compositor sends a
// handle for every toplevel surface, including our own windows, so a
// lys-ui process that binds the manager sees itself in the list under
// its xdg-toplevel title and app-id.
package wlsession

import (
	"slices"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxForeignToplevelVersion is the
// zwlr_foreign_toplevel_management_unstable_v1 version we bind: v2
// added the fullscreen state and requests, v3 the parent event; the
// generated binding understands both.
const maxForeignToplevelVersion = 3

// Toplevel is one remote window the compositor advertises through the
// foreign-toplevel protocol. Its fields track the latest batch the
// compositor sent (a done event applies title, app-id, and state
// together); requests go through the methods, which are no-ops when
// the compositor lacks the protocol.
type Toplevel struct {
	// Title and AppID are the window's current title and application
	// id.
	Title string
	AppID string

	// State flags from the latest state batch.
	Maximized, Minimized, Activated, Fullscreen bool

	// req is the wire handle, seen through the narrow interface below
	// so tests can record the requests; nil on the no-protocol path.
	req toplevelHandleAPI

	// sess routes the requests' seat lookup and list bookkeeping.
	sess *Session
}

// toplevelHandleAPI is the request and listener side of a
// zwlr_foreign_toplevel_handle_v1. *wlr.ZwlrForeignToplevelHandleV1
// satisfies it; tests substitute a recorder.
type toplevelHandleAPI interface {
	SetMaximized() error
	UnsetMaximized() error
	SetMinimized() error
	UnsetMinimized() error
	Activate(seat *wl.Seat) error
	Close() error
	Destroy() error
	AddTitleHandler(h wlr.ZwlrForeignToplevelHandleV1TitleHandler)
	AddAppIdHandler(h wlr.ZwlrForeignToplevelHandleV1AppIdHandler)
	AddStateHandler(h wlr.ZwlrForeignToplevelHandleV1StateHandler)
	AddDoneHandler(h wlr.ZwlrForeignToplevelHandleV1DoneHandler)
	AddClosedHandler(h wlr.ZwlrForeignToplevelHandleV1ClosedHandler)
}

// foreignToplevelManagerAPI is the manager side the session drives:
// register for new handles, stop the stream on teardown.
// *wlr.ZwlrForeignToplevelManagerV1 satisfies it.
type foreignToplevelManagerAPI interface {
	Stop() error
	AddToplevelHandler(h wlr.ZwlrForeignToplevelManagerV1ToplevelHandler)
}

// ForeignToplevelAvailable reports whether the compositor advertised
// zwlr_foreign_toplevel_manager_v1. When false, Toplevels stays empty
// and every toplevel request is a no-op.
func (s *Session) ForeignToplevelAvailable() bool { return s.foreignToplevelMgr != nil }

// Toplevels returns the windows the compositor currently advertises,
// in arrival order. The slice is the session's live view: entries
// disappear on closed, and their contents update on the dispatch
// goroutine.
func (s *Session) Toplevels() []*Toplevel { return s.toplevels }

// bindForeignToplevelManager binds the optional foreign toplevel
// manager global.
func (s *Session) bindForeignToplevelManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwlrForeignToplevelManagerV1(ctx)
	if !s.bindOptional(ev, maxForeignToplevelVersion, mgr) {
		return
	}
	s.foreignToplevelMgr = mgr
	mgr.AddToplevelHandler(s)
	debug.Log("shell", "foreign-toplevel-management-v1 bound")
}

// HandleZwlrForeignToplevelManagerV1Toplevel implements
// wlr.ZwlrForeignToplevelManagerV1ToplevelHandler: a toplevel exists;
// start tracking it.
func (s *Session) HandleZwlrForeignToplevelManagerV1Toplevel(ev wlr.ZwlrForeignToplevelManagerV1ToplevelEvent) {
	if ev.Toplevel == nil {
		return
	}
	s.addToplevel(ev.Toplevel)
}

// HandleZwlrForeignToplevelManagerV1Finished implements
// wlr.ZwlrForeignToplevelManagerV1FinishedHandler: the compositor
// stopped the stream (only after our own stop request), so no further
// toplevels will arrive.
func (s *Session) HandleZwlrForeignToplevelManagerV1Finished(wlr.ZwlrForeignToplevelManagerV1FinishedEvent) {
	debug.Log("shell", "foreign-toplevel stream finished")
}

// addToplevel tracks one new handle: wire its event listeners, list
// it, and fire the host hook. Separated from the wire event so tests
// can drive it with a fake handle.
func (s *Session) addToplevel(req toplevelHandleAPI) *Toplevel {
	tl := &Toplevel{req: req, sess: s}
	ev := &toplevelEvents{sess: s, tl: tl}
	req.AddTitleHandler(ev)
	req.AddAppIdHandler(ev)
	req.AddStateHandler(ev)
	req.AddDoneHandler(ev)
	req.AddClosedHandler(ev)
	s.toplevels = append(s.toplevels, tl)
	if s.OnToplevelAdded != nil {
		s.OnToplevelAdded(tl)
	}
	return tl
}

// removeToplevel drops a closed toplevel from the list and fires the
// host hook.
func (s *Session) removeToplevel(tl *Toplevel) {
	for i, t := range s.toplevels {
		if t == tl {
			s.toplevels = slices.Delete(s.toplevels, i, i+1)
			break
		}
	}
	if s.OnToplevelRemoved != nil {
		s.OnToplevelRemoved(tl)
	}
}

// Activate asks the compositor to focus the window, using the seat
// bound at request time. No-op without the protocol.
func (t *Toplevel) Activate() {
	if t == nil || t.req == nil || t.sess == nil || t.sess.seat == nil {
		return
	}
	_ = t.req.Activate(t.sess.seat)
}

// SetMaximized asks the compositor to maximize the window. No-op
// without the protocol.
func (t *Toplevel) SetMaximized() {
	if t != nil && t.req != nil {
		_ = t.req.SetMaximized()
	}
}

// UnsetMaximized asks the compositor to leave the maximized state.
// No-op without the protocol.
func (t *Toplevel) UnsetMaximized() {
	if t != nil && t.req != nil {
		_ = t.req.UnsetMaximized()
	}
}

// SetMinimized asks the compositor to minimize the window. No-op
// without the protocol.
func (t *Toplevel) SetMinimized() {
	if t != nil && t.req != nil {
		_ = t.req.SetMinimized()
	}
}

// UnsetMinimized asks the compositor to leave the minimized state.
// No-op without the protocol.
func (t *Toplevel) UnsetMinimized() {
	if t != nil && t.req != nil {
		_ = t.req.UnsetMinimized()
	}
}

// Close asks the compositor to close the window. No-op without the
// protocol.
func (t *Toplevel) Close() {
	if t != nil && t.req != nil {
		_ = t.req.Close()
	}
}

// toplevelEvents collects one toplevel's events until done applies
// them. The binding dispatches handle events without naming their
// object, so each toplevel carries its own listener.
type toplevelEvents struct {
	sess *Session
	tl   *Toplevel

	// pending batch: the fields keep the last applied values, so a
	// done that only carries a fresh state batch preserves the last
	// known title and app-id.
	title  string
	appID  string
	states []int32
}

// HandleZwlrForeignToplevelHandleV1Title implements
// wlr.ZwlrForeignToplevelHandleV1TitleHandler.
func (e *toplevelEvents) HandleZwlrForeignToplevelHandleV1Title(ev wlr.ZwlrForeignToplevelHandleV1TitleEvent) {
	e.title = ev.Title
}

// HandleZwlrForeignToplevelHandleV1AppId implements
// wlr.ZwlrForeignToplevelHandleV1AppIdHandler.
func (e *toplevelEvents) HandleZwlrForeignToplevelHandleV1AppId(ev wlr.ZwlrForeignToplevelHandleV1AppIdEvent) {
	e.appID = ev.AppId
}

// HandleZwlrForeignToplevelHandleV1State implements
// wlr.ZwlrForeignToplevelHandleV1StateHandler: the state event
// replaces the whole batch.
func (e *toplevelEvents) HandleZwlrForeignToplevelHandleV1State(ev wlr.ZwlrForeignToplevelHandleV1StateEvent) {
	e.states = ev.State
}

// HandleZwlrForeignToplevelHandleV1Done implements
// wlr.ZwlrForeignToplevelHandleV1DoneHandler: apply the pending batch
// atomically and notify the host.
func (e *toplevelEvents) HandleZwlrForeignToplevelHandleV1Done(wlr.ZwlrForeignToplevelHandleV1DoneEvent) {
	tl := e.tl
	tl.Title, tl.AppID = e.title, e.appID
	tl.Maximized, tl.Minimized, tl.Activated, tl.Fullscreen = decodeToplevelState(e.states)
	if e.sess.OnToplevelUpdated != nil {
		e.sess.OnToplevelUpdated(tl)
	}
}

// HandleZwlrForeignToplevelHandleV1Closed implements
// wlr.ZwlrForeignToplevelHandleV1ClosedHandler: the window is gone;
// retire the handle and drop it from the list.
func (e *toplevelEvents) HandleZwlrForeignToplevelHandleV1Closed(wlr.ZwlrForeignToplevelHandleV1ClosedEvent) {
	if e.tl.req != nil {
		_ = e.tl.req.Destroy()
	}
	e.sess.removeToplevel(e.tl)
}

// decodeToplevelState maps a state batch onto the Toplevel flags.
func decodeToplevelState(states []int32) (maximized, minimized, activated, fullscreen bool) {
	for _, st := range states {
		switch st {
		case wlr.ZwlrForeignToplevelHandleV1StateMaximized:
			maximized = true
		case wlr.ZwlrForeignToplevelHandleV1StateMinimized:
			minimized = true
		case wlr.ZwlrForeignToplevelHandleV1StateActivated:
			activated = true
		case wlr.ZwlrForeignToplevelHandleV1StateFullscreen:
			fullscreen = true
		}
	}
	return maximized, minimized, activated, fullscreen
}
