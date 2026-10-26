// Idle inhibit: optional zwp_idle_inhibit_manager_v1 support — the
// way to keep the compositor from marking the user idle (and
// suspending the screen) while something visible is happening, like a
// server stream. Inhibitors are explicit handles: create one for the
// surface that plays the activity, destroy it when it ends. The
// manager global is feature-detected: without it creation fails with
// ErrIdleInhibitUnavailable.
package wlsession

import (
	"errors"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxIdleInhibitVersion is the zwp_idle_inhibit_manager_v1 version we
// bind: the protocol stopped at version 1.
const maxIdleInhibitVersion = 1

// ErrIdleInhibitUnavailable reports that the compositor advertised no
// idle-inhibit manager, so nothing can be inhibited.
var ErrIdleInhibitUnavailable = errors.New("wlsession: no idle-inhibit protocol")

// idleInhibitAPI is the request side of zwp_idle_inhibit_manager_v1,
// seen through the narrow interface below so tests can record the
// requests. The wire proxy satisfies it through wireIdleInhibit.
type idleInhibitAPI interface {
	CreateInhibitor(surface *wl.Surface) (idleInhibitorAPI, error)
}

// idleInhibitorAPI is the request side of a zwp_idle_inhibitor_v1.
// *wlr.ZwpInhibitorV1 satisfies it; tests substitute a recorder.
type idleInhibitorAPI interface {
	Destroy() error
}

// wireIdleInhibit adapts the generated manager proxy to the narrow
// interface above.
type wireIdleInhibit struct{ mgr *wlr.ZwpInhibitManagerV1 }

// CreateInhibitor implements idleInhibitAPI.
func (m wireIdleInhibit) CreateInhibitor(surface *wl.Surface) (idleInhibitorAPI, error) {
	i, err := m.mgr.CreateInhibitor(surface)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// IdleInhibitor keeps the compositor from idling while its surface is
// visible. Destroy it when the activity ends; Destroy is nil-safe and
// idempotent.
type IdleInhibitor struct {
	req  idleInhibitorAPI
	done bool
}

// Destroy releases the inhibitor, letting the compositor idle again.
func (i *IdleInhibitor) Destroy() {
	if i == nil || i.done {
		return
	}
	i.done = true
	if i.req != nil {
		_ = i.req.Destroy()
	}
}

// IdleInhibitAvailable reports whether the compositor advertised
// zwp_idle_inhibit_manager_v1.
func (s *Session) IdleInhibitAvailable() bool { return s.idleInhibitMgr != nil }

// bindIdleInhibitManager binds the optional idle-inhibit manager
// global.
func (s *Session) bindIdleInhibitManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwpInhibitManagerV1(ctx)
	if !s.bindOptional(ev, maxIdleInhibitVersion, mgr) {
		return
	}
	s.idleInhibitMgr = wireIdleInhibit{mgr: mgr}
	debug.Log("shell", "idle-inhibit-v1 bound")
}

// InhibitIdle creates an inhibitor for surface: while the surface is
// visible, the compositor will not idle or suspend. The handle must
// be destroyed when the activity ends. Fails with
// ErrIdleInhibitUnavailable when the compositor lacks the protocol.
func (s *Session) InhibitIdle(surface *wl.Surface) (*IdleInhibitor, error) {
	if s.idleInhibitMgr == nil {
		return nil, ErrIdleInhibitUnavailable
	}
	if surface == nil {
		return nil, errors.New("wlsession: idle inhibit needs a surface")
	}
	req, err := s.idleInhibitMgr.CreateInhibitor(surface)
	if err != nil {
		return nil, err
	}
	return &IdleInhibitor{req: req}, nil
}
