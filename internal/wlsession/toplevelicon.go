// xdg-toplevel-icon-v1: optional toplevel window icons (#70). The
// manager creates icon objects and assigns them to toplevels; its
// icon_size events name the sizes the compositor prefers, which the
// client rasterizes into square shm buffers. Without the global every
// window-icon call is a silent no-op - the icon stays whatever the
// compositor derives from the desktop entry. Like all session state,
// the size list lives on the loop goroutine the events arrive from.
package wlsession

import (
	"slices"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxToplevelIconManagerVersion is the xdg_toplevel_icon_manager_v1
// version we bind: the protocol stopped at version 1.
const maxToplevelIconManagerVersion = 1

// iconSizeListener collects the manager's preferred sizes.
type iconSizeListener struct {
	session *Session
}

// HandleToplevelIconManagerV1IconSize records one preferred size.
func (l *iconSizeListener) HandleToplevelIconManagerV1IconSize(ev wlr.ToplevelIconManagerV1IconSizeEvent) {
	size := int(ev.Size)
	if size <= 0 || slices.Contains(l.session.iconSizes, size) {
		return
	}
	l.session.iconSizes = append(l.session.iconSizes, size)
	slices.Sort(l.session.iconSizes)
}

// HandleToplevelIconManagerV1Done marks the size list complete.
func (l *iconSizeListener) HandleToplevelIconManagerV1Done(wlr.ToplevelIconManagerV1DoneEvent) {
	l.session.iconSizesDone = true
	debug.Log("shell", "xdg-toplevel-icon-v1 bound; preferred sizes %v", l.session.iconSizes)
}

// bindToplevelIconManager binds the optional toplevel-icon manager and
// starts collecting its preferred sizes.
func (s *Session) bindToplevelIconManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewToplevelIconManagerV1(ctx)
	if !s.bindOptional(ev, maxToplevelIconManagerVersion, mgr) {
		return
	}
	l := &iconSizeListener{session: s}
	mgr.AddIconSizeHandler(l)
	mgr.AddDoneHandler(l)
	s.toplevelIconMgr = mgr
}

// ToplevelIconManager returns the bound xdg_toplevel_icon_manager_v1,
// or nil when the compositor does not advertise the protocol; a nil
// manager makes every window-icon request a no-op and the icon stays
// the compositor's default.
func (s *Session) ToplevelIconManager() *wlr.ToplevelIconManagerV1 {
	return s.toplevelIconMgr
}

// PreferredIconSizes returns the sizes the compositor asked window
// icons to be rasterized at (its icon_size events, sorted), complete
// once IconSizesReported is true. Without the protocol the list is
// empty and callers pick their own sizes.
func (s *Session) PreferredIconSizes() []int {
	return slices.Clone(s.iconSizes)
}

// IconSizesReported reports whether the manager's size list completed.
func (s *Session) IconSizesReported() bool { return s.iconSizesDone }
